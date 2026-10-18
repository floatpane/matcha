package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/emailstore"
	"github.com/floatpane/matcha/internal/palette"
	"github.com/floatpane/matcha/internal/pluginbridge"
	"github.com/floatpane/matcha/tui"
)

func TestOpeningUnreadEmailUpdatesVisibleFilter(t *testing.T) {
	accounts := []config.Account{{ID: "a", FetchEmail: "a@example.com"}}
	email := fetcher.Email{UID: 7, AccountID: "a", Subject: "unread"}
	store := emailstore.NewStore()
	store.SetFolder("INBOX", []fetcher.Email{email})
	folders := tui.NewFolderInbox([]string{"INBOX"}, accounts)
	folders.SetEmails([]fetcher.Email{email}, accounts)
	folders.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m := &Model{config: &config.Config{Accounts: accounts}, store: store, folderInbox: folders}
	m.markEmailReadAndQueue("a", 7, false)

	if selected := folders.GetInbox().GetSelectedEmail(); selected != nil {
		t.Fatalf("opened message remains visible in unread view: %+v", selected)
	}
	if got := store.GetEmailByUIDAndAccount(7, "a"); got == nil || !got.IsRead {
		t.Fatalf("opened message was not marked read in the store: %+v", got)
	}
}

func TestUnreadPaginationContinuesThroughApp(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	accounts := []config.Account{{ID: "a", FetchEmail: "a@example.com"}}
	folders := tui.NewFolderInbox([]string{"INBOX"}, accounts)
	folders.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m := &Model{
		current: folders, config: &config.Config{Accounts: accounts},
		store: emailstore.NewStore(), folderInbox: folders, palette: palette.New(),
	}
	_, cmd := m.Update(tui.FolderEmailsAppendedMsg{
		FolderName: "INBOX", AccountID: "a",
		Emails: []fetcher.Email{{UID: 7, AccountID: "a", IsRead: true}},
	})
	if cmd == nil {
		t.Fatal("app dropped unread pagination request")
	}
	fetch, ok := cmd().(tui.FetchFolderMoreEmailsMsg)
	if !ok || fetch.FolderName != "INBOX" || fetch.AccountID != "a" || fetch.Offset != 1 {
		t.Fatalf("app follow-up fetch = %#v, want next INBOX page", fetch)
	}
}

func TestUnreadInitialFolderFetchRequestsOlderPage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	accounts := []config.Account{{ID: "a", FetchEmail: "a@example.com"}}
	cfg := &config.Config{Accounts: accounts}
	store := emailstore.NewStore()
	folders := tui.NewFolderInbox([]string{"INBOX"}, accounts)
	folders.Update(tea.KeyPressMsg{Code: 'U', Text: "U"})
	m := &Model{
		current: folders, config: cfg, store: store, folderInbox: folders,
		palette: palette.New(), pluginBridge: pluginbridge.NewManager(nil, store, cfg, folders),
	}
	_, cmd := m.Update(tui.FolderEmailsFetchedMsg{
		FolderName: "INBOX",
		Emails:     []fetcher.Email{{UID: 7, AccountID: "a", IsRead: true}},
	})
	if cmd == nil {
		t.Fatal("initial read-only page should request older messages")
	}
	var fetch tui.FetchFolderMoreEmailsMsg
	var found bool
	for _, nested := range []tea.Cmd{cmd} {
		switch msg := nested().(type) {
		case tui.FetchFolderMoreEmailsMsg:
			fetch, found = msg, true
		case tea.BatchMsg:
			for _, inner := range msg {
				if request, ok := inner().(tui.FetchFolderMoreEmailsMsg); ok {
					fetch, found = request, true
				}
			}
		}
	}
	if !found || fetch.FolderName != "INBOX" || fetch.AccountID != "a" || fetch.Offset != 1 {
		t.Fatalf("initial folder fetch follow-up = %#v (found %t), want older INBOX page", fetch, found)
	}
}
