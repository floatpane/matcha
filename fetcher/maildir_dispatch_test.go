package fetcher

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/floatpane/matcha/config"
)

// End-to-end coverage for the maildir dispatch path in the fetcher package.
// These tests exercise the public entry points main.go calls (FetchFolders,
// FetchMailboxEmails, FetchEmailBodyFromMailbox, MarkEmailAsReadInMailbox)
// against an on-disk Maildir, mirroring the real TUI flow without an IMAP
// server.

func seenSuffix() string {
	if runtime.GOOS == "windows" {
		return ";2,S"
	}
	return ":2,S"
}

func makeMaildirRoot(t *testing.T, subfolders ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, sub := range []string{"cur", "new", "tmp"} {
		if err := os.MkdirAll(filepath.Join(root, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	for _, folder := range subfolders {
		for _, sub := range []string{"cur", "new", "tmp"} {
			if err := os.MkdirAll(filepath.Join(root, folder, sub), 0o755); err != nil {
				t.Fatalf("mkdir subfolder %s/%s: %v", folder, sub, err)
			}
		}
	}
	return root
}

func dropNewMessage(t *testing.T, folderDir, key, subject, body string, deliveredAt time.Time) {
	t.Helper()
	contents := fmt.Sprintf(
		"From: alice@example.com\r\n"+
			"To: me@local\r\n"+
			"Subject: %s\r\n"+
			"Date: %s\r\n"+
			"Message-ID: <%s@local>\r\n"+
			"Content-Type: text/plain; charset=utf-8\r\n"+
			"\r\n"+
			"%s\r\n",
		subject, deliveredAt.Format(time.RFC1123Z), key, body,
	)
	path := filepath.Join(folderDir, "new", key)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write message: %v", err)
	}
	if err := os.Chtimes(path, deliveredAt, deliveredAt); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func maildirAccount(root string) *config.Account {
	return &config.Account{
		ID:          "maildir-acct",
		Protocol:    "maildir",
		MaildirPath: root,
	}
}

func TestFetcherFetchFoldersMaildir(t *testing.T) {
	root := makeMaildirRoot(t, ".Sent", ".Archive")
	acct := maildirAccount(root)

	folders, err := FetchFolders(acct)
	if err != nil {
		t.Fatalf("FetchFolders: %v", err)
	}

	names := make(map[string]bool, len(folders))
	for _, f := range folders {
		names[f.Name] = true
	}

	for _, want := range []string{"INBOX", "Sent", "Archive"} {
		if !names[want] {
			t.Errorf("FetchFolders missing %q; got %v", want, names)
		}
	}
}

func TestFetcherFetchMailboxEmailsMaildir(t *testing.T) {
	root := makeMaildirRoot(t)
	acct := maildirAccount(root)

	dropNewMessage(t, root, "1700000000.M1.host", "Hello", "body one", time.Unix(1700000000, 0))
	dropNewMessage(t, root, "1700000100.M1.host", "Second", "body two", time.Unix(1700000100, 0))

	emails, err := FetchMailboxEmails(acct, "INBOX", 10, 0)
	if err != nil {
		t.Fatalf("FetchMailboxEmails: %v", err)
	}
	if len(emails) != 2 {
		t.Fatalf("expected 2 emails, got %d", len(emails))
	}

	// Newest-first ordering — Second was delivered later.
	if emails[0].Subject != "Second" {
		t.Errorf("expected first email subject %q, got %q", "Second", emails[0].Subject)
	}
	if emails[0].AccountID != acct.ID {
		t.Errorf("AccountID not propagated: got %q", emails[0].AccountID)
	}
	if emails[0].From == "" {
		t.Errorf("From not parsed")
	}
}

func TestFetcherFetchEmailBodyMaildir(t *testing.T) {
	root := makeMaildirRoot(t)
	acct := maildirAccount(root)

	dropNewMessage(t, root, "1700000200.M1.host", "Body Test", "the body contents", time.Unix(1700000200, 0))

	emails, err := FetchMailboxEmails(acct, "INBOX", 10, 0)
	if err != nil {
		t.Fatalf("FetchMailboxEmails: %v", err)
	}
	if len(emails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(emails))
	}

	body, _, _, err := FetchEmailBodyFromMailbox(acct, "INBOX", emails[0].UID)
	if err != nil {
		t.Fatalf("FetchEmailBodyFromMailbox: %v", err)
	}
	if !strings.Contains(body, "the body contents") {
		t.Errorf("body missing expected text; got %q", body)
	}
}

func TestFetcherMarkAsReadMaildir(t *testing.T) {
	root := makeMaildirRoot(t)
	acct := maildirAccount(root)

	dropNewMessage(t, root, "1700000300.M1.host", "Mark Me", "x", time.Unix(1700000300, 0))

	// First fetch promotes new/ → cur/ and returns an unread message.
	emails, err := FetchMailboxEmails(acct, "INBOX", 10, 0)
	if err != nil {
		t.Fatalf("first FetchMailboxEmails: %v", err)
	}
	if len(emails) != 1 || emails[0].IsRead {
		t.Fatalf("expected one unread email, got %+v", emails)
	}

	if err := MarkEmailAsReadInMailbox(acct, "INBOX", emails[0].UID); err != nil {
		t.Fatalf("MarkEmailAsReadInMailbox: %v", err)
	}

	// Verify the on-disk filename now carries the Seen flag suffix.
	curDir := filepath.Join(root, "cur")
	entries, err := os.ReadDir(curDir)
	if err != nil {
		t.Fatalf("read cur/: %v", err)
	}
	suffix := seenSuffix()
	found := false
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), suffix) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a cur/ entry with suffix %q; got %v", suffix, entries)
	}

	// Re-fetch and confirm IsRead is true now.
	emails2, err := FetchMailboxEmails(acct, "INBOX", 10, 0)
	if err != nil {
		t.Fatalf("second FetchMailboxEmails: %v", err)
	}
	if len(emails2) != 1 || !emails2[0].IsRead {
		t.Fatalf("expected one read email after mark, got %+v", emails2)
	}
}

func TestFetcherIMAPPathUnaffected(t *testing.T) {
	// An account with Protocol="" (IMAP default) and no IMAP server should
	// still fail with the original IMAP error, proving dispatch only fires
	// for maildir.
	acct := &config.Account{ID: "x", Protocol: ""}
	_, err := FetchFolders(acct)
	if err == nil {
		t.Fatal("expected error for empty IMAP server")
	}
	if !strings.Contains(err.Error(), "unsupported service_provider") {
		t.Errorf("expected IMAP-path error, got %v", err)
	}
}
