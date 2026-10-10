package jmap

import (
	"testing"

	"git.sr.ht/~rockorager/go-jmap/mail/mailbox"
)

func TestFolderName(t *testing.T) {
	if got := folderName(&mailbox.Mailbox{Name: "Inbox", Role: mailbox.RoleInbox}); got != "INBOX" {
		t.Errorf("inbox: got %q, want INBOX", got)
	}
	if got := folderName(&mailbox.Mailbox{Name: "Work"}); got != "Work" {
		t.Errorf("folder: got %q, want Work", got)
	}
}
