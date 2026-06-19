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

// newBadgeTestModel builds a minimal mainModel seeded with a single unread
// email for one account, ready to receive delete/archive messages.
func newBadgeTestModel(uid uint32, accountID string) *mainModel {
	email := fetcher.Email{UID: uid, AccountID: accountID, IsRead: false}
	return &mainModel{
		current: tui.NewChoice(),
		config: &config.Config{
			Accounts: []config.Account{{ID: accountID}},
		},
		emails:       []fetcher.Email{email},
		emailsByAcct: map[string][]fetcher.Email{accountID: {email}},
		folderEmails: map[string][]fetcher.Email{folderInbox: {email}},
	}
}

func TestDeleteEmailRefreshesUnreadBadge(t *testing.T) {
	const acct = "acct-a"
	m := newBadgeTestModel(7, acct)
	m.syncUnreadBadge()
	if m.unreadBadge != 1 {
		t.Fatalf("setup: unreadBadge = %d, want 1", m.unreadBadge)
	}

	m.Update(tui.DeleteEmailMsg{UID: 7, AccountID: acct})

	if m.unreadBadge != 0 {
		t.Fatalf("after delete: unreadBadge = %d, want 0 (badge not refreshed)", m.unreadBadge)
	}
}

func TestArchiveEmailRefreshesUnreadBadge(t *testing.T) {
	const acct = "acct-a"
	m := newBadgeTestModel(7, acct)
	m.syncUnreadBadge()
	if m.unreadBadge != 1 {
		t.Fatalf("setup: unreadBadge = %d, want 1", m.unreadBadge)
	}

	m.Update(tui.ArchiveEmailMsg{UID: 7, AccountID: acct})

	if m.unreadBadge != 0 {
		t.Fatalf("after archive: unreadBadge = %d, want 0 (badge not refreshed)", m.unreadBadge)
	}
}

func TestBatchDeleteEmailsRefreshesUnreadBadge(t *testing.T) {
	const acct = "acct-a"
	m := newBadgeTestModel(7, acct)
	m.syncUnreadBadge()
	if m.unreadBadge != 1 {
		t.Fatalf("setup: unreadBadge = %d, want 1", m.unreadBadge)
	}

	m.Update(tui.BatchDeleteEmailsMsg{UIDs: []uint32{7}, AccountID: acct})

	if m.unreadBadge != 0 {
		t.Fatalf("after batch delete: unreadBadge = %d, want 0 (badge not refreshed)", m.unreadBadge)
	}
}

func TestBatchArchiveEmailsRefreshesUnreadBadge(t *testing.T) {
	const acct = "acct-a"
	m := newBadgeTestModel(7, acct)
	m.syncUnreadBadge()
	if m.unreadBadge != 1 {
		t.Fatalf("setup: unreadBadge = %d, want 1", m.unreadBadge)
	}

	m.Update(tui.BatchArchiveEmailsMsg{UIDs: []uint32{7}, AccountID: acct})

	if m.unreadBadge != 0 {
		t.Fatalf("after batch archive: unreadBadge = %d, want 0 (badge not refreshed)", m.unreadBadge)
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
