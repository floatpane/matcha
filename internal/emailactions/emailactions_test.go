package emailactions

import (
	"testing"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/emailstore"
	"github.com/floatpane/matcha/tui"
)

type fakeFolderInbox struct {
	emails      []fetcher.Email
	accounts    []config.Account
	unread      map[string]int
	folders     []string
	decremented []string
	incremented []string
}

func (f *fakeFolderInbox) SetEmails(emails []fetcher.Email, accounts []config.Account) {
	f.emails = emails
	f.accounts = accounts
}

func (f *fakeFolderInbox) IncrementUnreadCount(folder string) {
	f.incremented = append(f.incremented, folder)
	f.unread[folder]++
}

func (f *fakeFolderInbox) DecrementUnreadCount(folder string) {
	f.decremented = append(f.decremented, folder)
	f.unread[folder]--
}

func (f *fakeFolderInbox) GetFolders() []string {
	return f.folders
}

func (f *fakeFolderInbox) GetUnreadCountsCopy() map[string]int {
	cp := make(map[string]int, len(f.unread))
	for k, v := range f.unread {
		cp[k] = v
	}
	return cp
}

func newCounterManager(emails []fetcher.Email, unread int) (*Manager, *fakeFolderInbox) {
	accountID := "acct-a"
	cfg := &config.Config{
		Accounts: []config.Account{{ID: accountID, Email: "a@example.com"}},
	}
	store := emailstore.NewStore()
	store.FolderEmails[emailstore.FolderInbox] = emails
	store.Emails = emails
	store.EmailsByAcct[accountID] = emails

	fake := &fakeFolderInbox{
		unread:  map[string]int{emailstore.FolderInbox: unread},
		folders: []string{emailstore.FolderInbox},
	}
	mgr := NewManager(Dependencies{Config: cfg}, store, fake)
	return mgr, fake
}

func TestDeleteUnreadEmailUpdatesFolderCounter(t *testing.T) {
	emails := []fetcher.Email{{UID: 42, AccountID: "acct-a", IsRead: false}}
	mgr, fake := newCounterManager(emails, 1)

	mgr.HandleDeleteEmailMsg(tui.DeleteEmailMsg{UID: 42, AccountID: "acct-a", Mailbox: emailstore.FolderInbox}, emailstore.FolderInbox)

	if got := fake.GetUnreadCountsCopy()[emailstore.FolderInbox]; got != 0 {
		t.Fatalf("after deleting an unread email, folder unread count = %d, want 0", got)
	}
}

func TestArchiveUnreadEmailUpdatesFolderCounter(t *testing.T) {
	emails := []fetcher.Email{{UID: 7, AccountID: "acct-a", IsRead: false}}
	mgr, fake := newCounterManager(emails, 1)

	mgr.HandleArchiveEmailMsg(tui.ArchiveEmailMsg{UID: 7, AccountID: "acct-a", Mailbox: emailstore.FolderInbox}, emailstore.FolderInbox)

	if got := fake.GetUnreadCountsCopy()[emailstore.FolderInbox]; got != 0 {
		t.Fatalf("after archiving an unread email, folder unread count = %d, want 0", got)
	}
}

func TestBatchDeleteUnreadEmailsUpdatesFolderCounter(t *testing.T) {
	emails := []fetcher.Email{
		{UID: 1, AccountID: "acct-a", IsRead: false},
		{UID: 2, AccountID: "acct-a", IsRead: false},
		{UID: 3, AccountID: "acct-a", IsRead: true},
	}
	mgr, fake := newCounterManager(emails, 2)

	mgr.HandleBatchDeleteEmailsMsg(tui.BatchDeleteEmailsMsg{UIDs: []uint32{1, 2, 3}, AccountID: "acct-a", Mailbox: emailstore.FolderInbox}, emailstore.FolderInbox)

	if got := fake.GetUnreadCountsCopy()[emailstore.FolderInbox]; got != 0 {
		t.Fatalf("after batch-deleting two unread emails, folder unread count = %d, want 0", got)
	}
}

func TestDeleteReadEmailLeavesFolderCounter(t *testing.T) {
	emails := []fetcher.Email{
		{UID: 10, AccountID: "acct-a", IsRead: true},
		{UID: 11, AccountID: "acct-a", IsRead: false},
	}
	mgr, fake := newCounterManager(emails, 1)

	mgr.HandleDeleteEmailMsg(tui.DeleteEmailMsg{UID: 10, AccountID: "acct-a", Mailbox: emailstore.FolderInbox}, emailstore.FolderInbox)

	if got := fake.GetUnreadCountsCopy()[emailstore.FolderInbox]; got != 1 {
		t.Fatalf("after deleting a read email, folder unread count = %d, want 1", got)
	}
}

func TestUndoDeleteRestoresFolderCounter(t *testing.T) {
	emails := []fetcher.Email{{UID: 99, AccountID: "acct-a", IsRead: false}}
	mgr, fake := newCounterManager(emails, 1)

	pa, _ := mgr.HandleDeleteEmailMsg(tui.DeleteEmailMsg{UID: 99, AccountID: "acct-a", Mailbox: emailstore.FolderInbox}, emailstore.FolderInbox)
	if got := fake.GetUnreadCountsCopy()[emailstore.FolderInbox]; got != 0 {
		t.Fatalf("after delete, folder unread count = %d, want 0", got)
	}

	mgr.StartActionGracePeriod(pa, "")
	mgr.RestorePendingAction()
	if got := fake.GetUnreadCountsCopy()[emailstore.FolderInbox]; got != 1 {
		t.Fatalf("after undo, folder unread count = %d, want 1", got)
	}
}
