package main

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/tui"
)

func TestSanitizeFilenameTruncatesCJKOnUTF8Boundary(t *testing.T) {
	name := strings.Repeat("文", 100) + ".txt"

	got := sanitizeFilename(name)

	if !utf8.ValidString(got) {
		t.Fatalf("sanitizeFilename returned invalid UTF-8: %q", got)
	}
	if len(got) > 255 {
		t.Fatalf("sanitizeFilename returned %d bytes, want at most 255", len(got))
	}
	if filepath.Ext(got) != ".txt" {
		t.Fatalf("sanitizeFilename lost extension: got %q", got)
	}
}

func TestSanitizeFilenameTruncatesEmojiOnUTF8Boundary(t *testing.T) {
	name := strings.Repeat("🚀", 80) + ".log"

	got := sanitizeFilename(name)

	if !utf8.ValidString(got) {
		t.Fatalf("sanitizeFilename returned invalid UTF-8: %q", got)
	}
	if len(got) > 255 {
		t.Fatalf("sanitizeFilename returned %d bytes, want at most 255", len(got))
	}
	if filepath.Ext(got) != ".log" {
		t.Fatalf("sanitizeFilename lost extension: got %q", got)
	}
}

func TestParseGlobalFlagsEnablesLogPanel(t *testing.T) {
	args, _, show := parseGlobalFlags([]string{"matcha", "--debug", "--logs", "--version"})
	if !show {
		t.Fatal("expected log panel flag to be enabled")
	}
	if got := strings.Join(args, " "); got != "matcha --version" {
		t.Fatalf("args = %q, want %q", got, "matcha --version")
	}
}

func TestParseGlobalFlagsDoesNotConsumeSubcommandFlags(t *testing.T) {
	args, _, show := parseGlobalFlags([]string{"matcha", "send", "--logs"})
	if show {
		t.Fatal("did not expect log panel flag after subcommand to be consumed")
	}
	if got := strings.Join(args, " "); got != "matcha send --logs" {
		t.Fatalf("args = %q, want %q", got, "matcha send --logs")
	}
}

// newFolderCounterModel builds a minimal mainModel whose INBOX folder holds the
// given emails and reports the given unread count. It is used to verify that the
// per-folder unread counter shown in the sidebar is updated immediately after
// delete/archive operations, without waiting for the next fetch.
func newFolderCounterModel(emails []fetcher.Email, unread int) *mainModel {
	accountID := "acct-a"
	cfg := &config.Config{
		Accounts: []config.Account{{ID: accountID, Email: "a@example.com"}},
	}

	fi := tui.NewFolderInbox([]string{folderInbox}, cfg.Accounts)
	fi.SetUnreadCounts(map[string]int{folderInbox: unread})
	fi.SetEmails(emails, cfg.Accounts)

	byAcct := make(map[string][]fetcher.Email)
	for _, e := range emails {
		byAcct[e.AccountID] = append(byAcct[e.AccountID], e)
	}

	m := &mainModel{
		config:       cfg,
		folderInbox:  fi,
		current:      fi,
		emails:       emails,
		emailsByAcct: byAcct,
		folderEmails: map[string][]fetcher.Email{folderInbox: emails},
	}
	return m
}

// TestDeleteUnreadEmailUpdatesFolderCounter verifies that deleting an unread
// email immediately decrements the folder's unread counter in the sidebar,
// rather than waiting for the next fetch (issue #1404).
func TestDeleteUnreadEmailUpdatesFolderCounter(t *testing.T) {
	email := fetcher.Email{UID: 42, AccountID: "acct-a", IsRead: false}
	m := newFolderCounterModel([]fetcher.Email{email}, 1)

	m.Update(tui.DeleteEmailMsg{UID: 42, AccountID: "acct-a", Mailbox: folderInbox})

	if got := m.folderInbox.GetUnreadCountsCopy()[folderInbox]; got != 0 {
		t.Fatalf("after deleting an unread email, folder unread count = %d, want 0", got)
	}
}

// TestArchiveUnreadEmailUpdatesFolderCounter verifies the same immediate update
// for archive operations.
func TestArchiveUnreadEmailUpdatesFolderCounter(t *testing.T) {
	email := fetcher.Email{UID: 7, AccountID: "acct-a", IsRead: false}
	m := newFolderCounterModel([]fetcher.Email{email}, 1)

	m.Update(tui.ArchiveEmailMsg{UID: 7, AccountID: "acct-a", Mailbox: folderInbox})

	if got := m.folderInbox.GetUnreadCountsCopy()[folderInbox]; got != 0 {
		t.Fatalf("after archiving an unread email, folder unread count = %d, want 0", got)
	}
}

// TestBatchDeleteUnreadEmailsUpdatesFolderCounter verifies that batch delete
// decrements the folder counter once per unread email removed, while leaving
// already-read emails untouched.
func TestBatchDeleteUnreadEmailsUpdatesFolderCounter(t *testing.T) {
	emails := []fetcher.Email{
		{UID: 1, AccountID: "acct-a", IsRead: false},
		{UID: 2, AccountID: "acct-a", IsRead: false},
		{UID: 3, AccountID: "acct-a", IsRead: true},
	}
	m := newFolderCounterModel(emails, 2)

	m.Update(tui.BatchDeleteEmailsMsg{UIDs: []uint32{1, 2, 3}, AccountID: "acct-a", Mailbox: folderInbox})

	if got := m.folderInbox.GetUnreadCountsCopy()[folderInbox]; got != 0 {
		t.Fatalf("after batch-deleting two unread emails, folder unread count = %d, want 0", got)
	}
}

// TestDeleteReadEmailLeavesFolderCounter verifies that deleting an already-read
// email does not change the folder unread counter.
func TestDeleteReadEmailLeavesFolderCounter(t *testing.T) {
	emails := []fetcher.Email{
		{UID: 10, AccountID: "acct-a", IsRead: true},
		{UID: 11, AccountID: "acct-a", IsRead: false},
	}
	m := newFolderCounterModel(emails, 1)

	m.Update(tui.DeleteEmailMsg{UID: 10, AccountID: "acct-a", Mailbox: folderInbox})

	if got := m.folderInbox.GetUnreadCountsCopy()[folderInbox]; got != 1 {
		t.Fatalf("after deleting a read email, folder unread count = %d, want 1", got)
	}
}

// TestUndoDeleteRestoresFolderCounter verifies that undoing a delete restores
// the folder unread counter that was decremented when the email was deleted.
func TestUndoDeleteRestoresFolderCounter(t *testing.T) {
	email := fetcher.Email{UID: 99, AccountID: "acct-a", IsRead: false}
	m := newFolderCounterModel([]fetcher.Email{email}, 1)

	m.Update(tui.DeleteEmailMsg{UID: 99, AccountID: "acct-a", Mailbox: folderInbox})
	if got := m.folderInbox.GetUnreadCountsCopy()[folderInbox]; got != 0 {
		t.Fatalf("after delete, folder unread count = %d, want 0", got)
	}

	m.restorePendingAction()
	if got := m.folderInbox.GetUnreadCountsCopy()[folderInbox]; got != 1 {
		t.Fatalf("after undo, folder unread count = %d, want 1", got)
	}
}

func TestUnreadBadgeCountDeduplicatesOverlappingStores(t *testing.T) {
	email := fetcher.Email{UID: 42, AccountID: "acct-a"}
	got := unreadBadgeCount(
		map[string][]fetcher.Email{
			"acct-a": {email},
		},
		map[string][]fetcher.Email{
			folderInbox: {email},
		},
	)

	if got != 1 {
		t.Fatalf("unreadBadgeCount() = %d, want 1", got)
	}
}
