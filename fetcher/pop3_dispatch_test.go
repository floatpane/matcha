package fetcher

import (
	"strings"
	"testing"

	"github.com/floatpane/matcha/config"
)

// POP3 accounts used to fall through to the IMAP code path and fail with
// "unsupported service_provider", leaving the inbox empty.
func TestPOP3AccountsUseBackendProvider(t *testing.T) {
	acct := &config.Account{
		ID:         "pop",
		Email:      "u@example.com",
		Password:   "x",
		Protocol:   "pop3",
		POP3Server: "127.0.0.1",
		POP3Port:   1, // nothing listens here: connection refused
		SC:         &config.SessionCache{},
	}
	if !hasBackendProvider(acct) {
		t.Fatal("pop3 accounts must be served by the backend provider")
	}
	if _, err := FetchFolderEmails(acct, "INBOX", 0, 0); err == nil || strings.Contains(err.Error(), "unsupported service_provider") {
		t.Fatalf("FetchFolderEmails err = %v, want a POP3 connection error", err)
	}
	folders, err := FetchFolders(acct)
	if err != nil || len(folders) != 1 || folders[0].Name != "INBOX" {
		t.Fatalf("FetchFolders = %v, %v", folders, err)
	}
	if err := AppendToSentMailbox(acct, []byte("raw")); err != nil {
		t.Fatalf("AppendToSentMailbox should be a no-op for POP3, got %v", err)
	}
}
