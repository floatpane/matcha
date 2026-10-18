package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

func TestFolderSidebarClickSelectsRenderedFolder(t *testing.T) {
	previousProvider := uiProvider
	t.Cleanup(func() { uiProvider = previousProvider })
	folders := []string{"INBOX", "AardvarkFolderLong", "BravoFolderLonger", "CharlieFolderLong"}
	for _, headerVisible := range []bool{true, false} {
		uiProvider = &stubUIProvider{visibility: map[string]bool{"header": headerVisible}}
		for _, folder := range folders {
			fi := NewFolderInbox(folders, []config.Account{{ID: "a", Name: "Account"}})
			fi.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
			fi.unread = map[string]int{
				"AardvarkFolderLong": 123456,
				"BravoFolderLonger":  123456,
				"CharlieFolderLong":  123456,
			}
			rows := strings.Split(ansi.Strip(fi.renderSidebar()), "\n")
			label := folder
			if len(label) > 8 {
				label = label[:8]
			}
			row := -1
			for y, line := range rows {
				if strings.Contains(line, label) {
					row = y
					break
				}
			}
			if row < 0 {
				t.Fatalf("folder %q not rendered in sidebar", folder)
			}
			fi.Update(tea.MouseClickMsg{X: 3, Y: row, Button: tea.MouseLeft})
			if got := fi.GetCurrentFolder(); got != folder {
				t.Errorf("header visible %t: clicking %q at row %d selected %q", headerVisible, folder, row, got)
			}
		}
	}
}

func TestFolderPaginationFailureClearsOnlyActiveFolder(t *testing.T) {
	const folder = "UnreadTestFolder"
	accounts := []config.Account{{ID: "a", FetchEmail: "a@example.com"}}
	fi := NewFolderInbox([]string{folder, "ZOtherTestFolder"}, accounts)
	fi.SetEmails([]fetcher.Email{{UID: 1, AccountID: "a", IsRead: true}}, accounts)
	fi.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	if !fi.inbox.isFetching {
		t.Fatal("unread pagination should be marked as loading")
	}
	fi.Update(FolderEmailsPageFailedMsg{FolderName: "ZOtherTestFolder", Err: errors.New("old request")})
	if !fi.inbox.isFetching {
		t.Fatal("stale folder error cleared the current loading state")
	}
	fi.Update(FolderEmailsPageFailedMsg{FolderName: folder, Err: errors.New("network down")})
	if fi.inbox.isFetching {
		t.Fatal("failed request left the unread list loading")
	}
	fi.Update(FetchingMoreEmailsMsg{FolderName: "ZOtherTestFolder"})
	if fi.inbox.isFetching {
		t.Fatal("stale loading event restarted loading in the current folder")
	}
	if cmd := fi.ContinueUnreadFetch(); cmd == nil {
		t.Fatal("unread pagination cannot retry after a failed request")
	}
}

func TestFolderInboxUnreadOnlyFetchesCurrentFolder(t *testing.T) {
	const folder = "UnreadTestFolder"
	accounts := []config.Account{{ID: "a", FetchEmail: "a@example.com"}, {ID: "b", FetchEmail: "b@example.com"}}
	fi := NewFolderInbox([]string{folder}, accounts)
	fi.SetEmails([]fetcher.Email{{UID: 1, AccountID: "a", IsRead: true}}, accounts)
	_, cmd := fi.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	if cmd == nil {
		t.Fatal("unread-only filter should request another page")
	}
	first, ok := cmd().(FetchFolderMoreEmailsMsg)
	if !ok || first.FolderName != folder || first.AccountID != "a" {
		t.Fatalf("first request = %#v, want current folder in account a", first)
	}
	_, cmd = fi.Update(FolderEmailsAppendedMsg{FolderName: folder, AccountID: "a"})
	if cmd == nil {
		t.Fatal("exhausting account a should fetch account b")
	}
	second, ok := cmd().(FetchFolderMoreEmailsMsg)
	if !ok || second.FolderName != folder || second.AccountID != "b" {
		t.Fatalf("second request = %#v, want current folder in account b", second)
	}
}

func TestFolderInboxReadingMessageRefillsUnreadView(t *testing.T) {
	const folder = "UnreadTestFolder"
	accounts := []config.Account{{ID: "a", FetchEmail: "a@example.com"}}
	fi := NewFolderInbox([]string{folder}, accounts)
	fi.SetEmails([]fetcher.Email{{UID: 7, AccountID: "a", Subject: "unread"}}, accounts)
	fi.inbox.unreadOnly = true
	fi.inbox.updateList()

	cmd := fi.MarkEmailAsRead(7, "a")
	if got := len(fi.inbox.list.Items()); got != 0 {
		t.Fatalf("read message remains in unread view: %d items", got)
	}
	if cmd == nil {
		t.Fatal("reading the last visible message should fetch the next page")
	}
	fetch, ok := cmd().(FetchFolderMoreEmailsMsg)
	if !ok || fetch.FolderName != folder || fetch.AccountID != "a" {
		t.Fatalf("read-triggered fetch = %#v, want current folder and account", fetch)
	}
}

func TestFolderInboxWrapsBatchedCommandsWithOriginalFolder(t *testing.T) {
	const folder = "UnreadTestFolder"
	fi := NewFolderInbox([]string{folder, "OtherTestFolder"}, nil)
	fi.currentFolder = folder
	cmd := fi.wrapInboxCmd(tea.Batch(
		func() tea.Msg { return FetchMoreEmailsMsg{AccountID: "a", Offset: 20, Limit: 20} },
		func() tea.Msg { return RequestRefreshMsg{} },
	))
	fi.currentFolder = "OtherTestFolder"
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("wrapped command = %#v, want two batched commands", cmd())
	}
	fetch, ok := batch[0]().(FetchFolderMoreEmailsMsg)
	if !ok || fetch.FolderName != folder || fetch.AccountID != "a" || fetch.Offset != 20 {
		t.Fatalf("batched fetch = %#v, want original folder request", fetch)
	}
	refresh, ok := batch[1]().(RequestRefreshMsg)
	if !ok || refresh.FolderName != folder {
		t.Fatalf("batched refresh = %#v, want original folder request", refresh)
	}
}

// TestFolderInboxSplitPreviewRendersSearchHit covers the case Lea reported on
// PR #1186: opening a search result in split-pane mode used to silently drop
// the keypress because the email was not in m.inbox.allEmails. After the fix
// OpenSplitPreview accepts the resolved email and findEmailByUID falls back
// to it, so PreviewBodyFetchedMsg can build the preview pane.
func TestFolderInboxSplitPreviewRendersSearchHit(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "host.example.com", FetchEmail: "first@example.com"},
	}
	fi := NewFolderInbox([]string{keyINBOX, "Archive"}, accounts)
	// Force a non-zero canvas so calculate*Width does not panic on Update.
	model, _ := fi.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	fi = model.(*FolderInbox)

	// Search hit lives in a different folder; allEmails is empty.
	hit := &fetcher.Email{
		UID:       4242,
		AccountID: "account-1",
		MessageID: "<search-hit@example.com>",
		From:      "sender@example.com",
		To:        []string{"first@example.com"},
		Subject:   "Search hit",
	}

	fi.OpenSplitPreview(hit.UID, hit.AccountID, hit)

	if fi.previewSearchEmail == nil {
		t.Fatal("OpenSplitPreview should retain the search hit email")
	}
	if got := fi.findEmailByUID(hit.UID, hit.AccountID); got == nil {
		t.Fatal("findEmailByUID should fall back to the search hit email")
	}

	// Simulate the body arriving and verify the preview pane is built.
	model, _ = fi.Update(PreviewBodyFetchedMsg{
		UID:       hit.UID,
		AccountID: hit.AccountID,
		Body:      "hello body",
	})
	fi = model.(*FolderInbox)

	if fi.previewPane == nil {
		t.Fatal("expected previewPane to be built from the search hit fallback")
	}

	// closeSplitPreview must clear the cached search hit so a later open with
	// no email cannot accidentally reuse the stale reference.
	fi.closeSplitPreview()
	if fi.previewSearchEmail != nil {
		t.Fatal("closeSplitPreview should clear previewSearchEmail")
	}
}

// TestFolderInboxSplitPreviewPrefersAllEmails verifies that when the email is
// already known in allEmails, findEmailByUID returns the live entry (so reads
// like IsRead stay current) instead of the snapshot passed via OpenSplitPreview.
func TestFolderInboxSplitPreviewPrefersAllEmails(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "host.example.com", FetchEmail: "first@example.com"},
	}
	fi := NewFolderInbox([]string{keyINBOX}, accounts)
	model, _ := fi.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	fi = model.(*FolderInbox)

	live := fetcher.Email{UID: 7, AccountID: "account-1", Subject: "live", IsRead: true}
	fi.SetEmails([]fetcher.Email{live}, accounts)

	stale := &fetcher.Email{UID: 7, AccountID: "account-1", Subject: "stale", IsRead: false}
	fi.OpenSplitPreview(live.UID, live.AccountID, stale)

	got := fi.findEmailByUID(live.UID, live.AccountID)
	if got == nil {
		t.Fatal("findEmailByUID should resolve the email")
	}
	if got.Subject != "live" || !got.IsRead {
		t.Fatalf("expected the live allEmails entry, got %+v", got)
	}
}

// TestSearchOverlayKeysNotIntercepted covers issue #1199: pressing keys that
// match folder-level bindings (e.g. "m" for move) while the search overlay is
// active used to trigger the move flow instead of entering text into the
// search input. FolderInbox.Update now passes through to the inner inbox
// while m.inbox.searchOverlay != nil so the overlay receives raw keystrokes.
func TestSearchOverlayKeysNotIntercepted(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "host.example.com", FetchEmail: "first@example.com"},
	}
	fi := NewFolderInbox([]string{keyINBOX, "Archive"}, accounts)
	model, _ := fi.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	fi = model.(*FolderInbox)

	// Selection must exist so the bug's "m" -> Move handler would actually fire.
	fi.SetEmails([]fetcher.Email{
		{UID: 1, AccountID: "account-1", Subject: "first"},
	}, accounts)

	// Open the search overlay (the same state pressing "/" produces in inbox.go).
	fi.inbox.searchOverlay = NewSearchOverlay(fi.width, fi.height)

	// Press "m" -- with the bug this would set movingEmail = true.
	model, _ = fi.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	fi = model.(*FolderInbox)

	if fi.movingEmail {
		t.Fatal("pressing 'm' while search overlay is active must not start the move flow")
	}
	if fi.inbox.searchOverlay == nil {
		t.Fatal("search overlay must remain open after typing into it")
	}
	if got := fi.inbox.searchOverlay.input.Value(); got != "m" {
		t.Fatalf("search input should contain typed character, got %q", got)
	}
}
