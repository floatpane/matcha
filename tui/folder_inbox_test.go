package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

// TestFolderInboxSplitPreviewRendersSearchHit covers the case Lea reported on
// PR #1186: opening a search result in split-pane mode used to silently drop
// the keypress because the email was not in m.inbox.allEmails. After the fix
// OpenSplitPreview accepts the resolved email and findEmailByUID falls back
// to it, so PreviewBodyFetchedMsg can build the preview pane.
func TestFolderInboxSplitPreviewRendersSearchHit(t *testing.T) {
	accounts := []config.Account{
		{ID: "account-1", Email: "host.example.com", FetchEmail: "first@example.com"},
	}
	fi := NewFolderInbox([]string{"INBOX", "Archive"}, accounts)
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
	fi := NewFolderInbox([]string{"INBOX"}, accounts)
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
