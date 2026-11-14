package emailactions

import (
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/emailstore"
	"github.com/floatpane/matcha/tui"
)

type unreadDelta struct {
	account, folder string
	delta           int
}

// stubUnreadCache records per-account unread adjustments instead of writing
// the folder cache.
func stubUnreadCache(t *testing.T) *[]unreadDelta {
	t.Helper()
	var mu sync.Mutex
	var got []unreadDelta
	orig := adjustCachedUnread
	t.Cleanup(func() { adjustCachedUnread = orig })
	adjustCachedUnread = func(account, folder string, delta int) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, unreadDelta{account, folder, delta})
	}
	return &got
}

// Delete in INBOX, switch to Archive within the undo window, press undo: the
// Archive view must keep showing Archive's emails (later actions there use
// the Archive folder with whatever UIDs are displayed).
func TestUndoAfterFolderSwitchKeepsCurrentFolder(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	stubUnreadCache(t)
	cfg := &config.Config{Accounts: []config.Account{{ID: "a", Email: "a@example.com"}}}
	store := emailstore.NewStore()
	inbox := []fetcher.Email{{UID: 7, AccountID: "a", Subject: "inbox-7"}, {UID: 8, AccountID: "a", Subject: "inbox-8"}}
	store.SetFolder("INBOX", inbox)
	fake := &fakeFolderInbox{unread: map[string]int{}, folders: []string{"INBOX", "Archive"}, current: "INBOX"}
	m := NewManager(Dependencies{Config: cfg}, store, fake)

	pa, notice := m.HandleDeleteEmailMsg(tui.DeleteEmailMsg{UID: 7, AccountID: "a"}, "INBOX")
	_ = m.StartActionGracePeriod(pa, notice)

	archive := []fetcher.Email{{UID: 7, AccountID: "a", Subject: "archive-7"}}
	store.SetFolder("Archive", archive)
	fake.current = "Archive"
	fake.SetEmails(archive, cfg.Accounts)

	m.RestorePendingAction()

	if len(fake.emails) != 1 || fake.emails[0].Subject != "archive-7" {
		t.Fatalf("Archive view shows %+v after undo", fake.emails)
	}
	if len(store.Emails) != 1 || store.Emails[0].Subject != "archive-7" {
		t.Fatalf("store.Emails = %+v, want Archive's emails", store.Emails)
	}
	if got := store.FolderEmails["INBOX"]; len(got) != 2 {
		t.Fatalf("INBOX list not restored: %+v", got)
	}

	// Undo while still in the folder restores the visible list as before.
	pa, notice = m.HandleDeleteEmailMsg(tui.DeleteEmailMsg{UID: 7, AccountID: "a"}, "Archive")
	_ = m.StartActionGracePeriod(pa, notice)
	if len(fake.emails) != 1 {
		t.Fatalf("precondition: view = %+v", fake.emails)
	}
	m.RestorePendingAction()
	if len(store.Emails) != 1 || store.Emails[0].Subject != "archive-7" || len(fake.emails) != 1 {
		t.Fatalf("undo in the same folder: store=%+v view=%+v", store.Emails, fake.emails)
	}
}

// Deleting an unread email must persist only this account's delta, not the
// folder inbox's merged folder list and summed counts.
func TestDeletePersistsPerAccountUnreadDelta(t *testing.T) {
	deltas := stubUnreadCache(t)
	emails := []fetcher.Email{{UID: 1, AccountID: "acct-a"}, {UID: 2, AccountID: "acct-a"}}
	mgr, _ := newCounterManager(emails, 5)

	mgr.HandleBatchDeleteEmailsMsg(tui.BatchDeleteEmailsMsg{UIDs: []uint32{1, 2}, AccountID: "acct-a"}, emailstore.FolderInbox)
	if len(*deltas) != 1 || (*deltas)[0] != (unreadDelta{"acct-a", emailstore.FolderInbox, -2}) {
		t.Fatalf("deltas = %+v", *deltas)
	}
}

// The command runs on another goroutine while the UI goroutine may replace
// the dependencies (run with -race).
func TestBatchCmdsCaptureServiceAtCreation(t *testing.T) {
	cfg := &config.Config{Accounts: []config.Account{{ID: "a"}}}
	m := NewManager(Dependencies{Config: cfg}, emailstore.NewStore(), nil)
	// Built on this (the "UI") goroutine, run on another — like Bubble Tea.
	cmds := []tea.Cmd{
		m.BatchDeleteEmailsCmd([]uint32{1}, "a", "INBOX", tui.MailboxInbox, 1),
		m.BatchArchiveEmailsCmd([]uint32{1}, "a", "INBOX", tui.MailboxInbox, 1),
		m.BatchMoveEmailsCmd([]uint32{1}, "a", "INBOX", "Archive", 1),
	}
	for _, cmd := range cmds {
		done := make(chan any)
		go func() { done <- cmd() }()
		m.UpdateDependencies(Dependencies{Config: cfg, Service: daemonclient.Service(nil)})
		msg := <-done
		if res, ok := msg.(tui.BatchEmailActionDoneMsg); !ok || res.Err == nil {
			t.Fatalf("msg = %#v, want BatchEmailActionDoneMsg with error (no service)", msg)
		}
	}
}
