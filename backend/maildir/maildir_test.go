package maildir

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
)

// seenSuffix returns the on-disk suffix go-maildir appends for a message that
// carries only the Seen flag. Windows uses ';' instead of ':' because ':' is
// reserved in NTFS filenames.
func seenSuffix() string {
	if runtime.GOOS == "windows" {
		return ";2,S"
	}
	return ":2,S"
}

// makeMaildir creates a root + the named Maildir++ subfolders.
func makeMaildir(t *testing.T, subfolders ...string) string {
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

// dropMessage writes a fake delivered message into the new/ dir of a Maildir.
// The filename intentionally has no flag suffix (delivered state).
func dropMessage(t *testing.T, dir, key, subject, body string, deliveredAt time.Time) {
	t.Helper()
	contents := fmt.Sprintf(
		"From: alice@example.com\r\n"+
			"To: me@local\r\n"+
			"Subject: %s\r\n"+
			"Date: %s\r\n"+
			"Message-ID: <%s@local>\r\n"+
			"\r\n"+
			"%s\r\n",
		subject, deliveredAt.Format(time.RFC1123Z), key, body,
	)
	path := filepath.Join(dir, "new", key)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write message: %v", err)
	}
	// Match deliveredAt so sort-by-mtime is deterministic.
	if err := os.Chtimes(path, deliveredAt, deliveredAt); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func newProvider(t *testing.T, root string) *Provider {
	t.Helper()
	p, err := New(&config.Account{ID: "acct1", MaildirPath: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

func TestNewRejectsMissingPath(t *testing.T) {
	if _, err := New(&config.Account{ID: "x"}); err == nil {
		t.Fatal("expected error for empty MaildirPath")
	}
	if _, err := New(&config.Account{ID: "x", MaildirPath: "/this/does/not/exist"}); err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestFetchFoldersListsInboxAndSubfolders(t *testing.T) {
	root := makeMaildir(t, ".Sent", ".Archive")
	p := newProvider(t, root)

	folders, err := p.FetchFolders(context.Background())
	if err != nil {
		t.Fatalf("FetchFolders: %v", err)
	}

	names := make(map[string]bool, len(folders))
	for _, f := range folders {
		names[f.Name] = true
	}
	for _, want := range []string{"INBOX", "Sent", "Archive"} {
		if !names[want] {
			t.Errorf("expected folder %q in %v", want, names)
		}
	}
}

func TestFetchEmailsNewestFirst(t *testing.T) {
	root := makeMaildir(t)
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	dropMessage(t, root, "1700000000.older.host", "first", "old body", t0)
	dropMessage(t, root, "1700000100.newer.host", "second", "new body", t0.Add(time.Hour))

	p := newProvider(t, root)
	emails, err := p.FetchEmails(context.Background(), "INBOX", 50, 0)
	if err != nil {
		t.Fatalf("FetchEmails: %v", err)
	}
	if len(emails) != 2 {
		t.Fatalf("want 2 emails, got %d", len(emails))
	}
	if emails[0].Subject != "second" {
		t.Errorf("want newest first, got %q", emails[0].Subject)
	}
	if emails[1].Subject != "first" {
		t.Errorf("want oldest second, got %q", emails[1].Subject)
	}
	if emails[0].UID == 0 || emails[0].UID == emails[1].UID {
		t.Errorf("UIDs must be nonzero and distinct: %d vs %d", emails[0].UID, emails[1].UID)
	}
	if emails[0].IsRead {
		t.Error("freshly delivered message should not be read")
	}
}

func TestFetchEmailsRespectsLimitOffset(t *testing.T) {
	root := makeMaildir(t)
	base := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("1700000%03d.M%dP1.host", i, i)
		dropMessage(t, root, key, fmt.Sprintf("msg%d", i), "body", base.Add(time.Duration(i)*time.Minute))
	}

	p := newProvider(t, root)
	page, err := p.FetchEmails(context.Background(), "INBOX", 2, 1)
	if err != nil {
		t.Fatalf("FetchEmails: %v", err)
	}
	if len(page) != 2 {
		t.Fatalf("want 2, got %d", len(page))
	}
	if page[0].Subject != "msg3" || page[1].Subject != "msg2" {
		t.Errorf("want msg3,msg2 — got %q,%q", page[0].Subject, page[1].Subject)
	}
}

func TestMarkAsReadAddsSeenFlag(t *testing.T) {
	root := makeMaildir(t)
	dropMessage(t, root, "1700000000.x.host", "subj", "body", time.Now())

	p := newProvider(t, root)
	emails, err := p.FetchEmails(context.Background(), "INBOX", 10, 0)
	if err != nil || len(emails) != 1 {
		t.Fatalf("FetchEmails setup: %v / %d", err, len(emails))
	}

	if err := p.MarkAsRead(context.Background(), "INBOX", emails[0].UID); err != nil {
		t.Fatalf("MarkAsRead: %v", err)
	}

	curFiles, _ := os.ReadDir(filepath.Join(root, "cur"))
	if len(curFiles) != 1 {
		t.Fatalf("want 1 file in cur/, got %d", len(curFiles))
	}
	if !strings.HasSuffix(curFiles[0].Name(), seenSuffix()) {
		t.Errorf("want %s suffix, got %q", seenSuffix(), curFiles[0].Name())
	}

	emails, err = p.FetchEmails(context.Background(), "INBOX", 10, 0)
	if err != nil || len(emails) != 1 {
		t.Fatalf("FetchEmails post-flag: %v / %d", err, len(emails))
	}
	if !emails[0].IsRead {
		t.Error("email should report IsRead=true after MarkAsRead")
	}
}

func TestDeleteEmailRemovesFile(t *testing.T) {
	root := makeMaildir(t)
	dropMessage(t, root, "1700000000.del.host", "del", "body", time.Now())

	p := newProvider(t, root)
	emails, _ := p.FetchEmails(context.Background(), "INBOX", 10, 0)
	if len(emails) != 1 {
		t.Fatalf("setup: want 1 email, got %d", len(emails))
	}

	if err := p.DeleteEmail(context.Background(), "INBOX", emails[0].UID); err != nil {
		t.Fatalf("DeleteEmail: %v", err)
	}

	newFiles, _ := os.ReadDir(filepath.Join(root, "new"))
	curFiles, _ := os.ReadDir(filepath.Join(root, "cur"))
	if len(newFiles)+len(curFiles) != 0 {
		t.Errorf("expected no files left, got new=%d cur=%d", len(newFiles), len(curFiles))
	}
}

func TestMoveEmailRelocates(t *testing.T) {
	root := makeMaildir(t, ".Archive")
	dropMessage(t, root, "1700000000.mv.host", "mv", "body", time.Now())

	p := newProvider(t, root)
	emails, _ := p.FetchEmails(context.Background(), "INBOX", 10, 0)
	if len(emails) != 1 {
		t.Fatalf("setup: want 1 email, got %d", len(emails))
	}

	if err := p.MoveEmail(context.Background(), emails[0].UID, "INBOX", "Archive"); err != nil {
		t.Fatalf("MoveEmail: %v", err)
	}

	inboxFiles, _ := os.ReadDir(filepath.Join(root, "new"))
	if len(inboxFiles) != 0 {
		t.Errorf("expected INBOX empty, got %d files", len(inboxFiles))
	}
	archiveCur, _ := os.ReadDir(filepath.Join(root, ".Archive", "cur"))
	archiveNew, _ := os.ReadDir(filepath.Join(root, ".Archive", "new"))
	if len(archiveCur)+len(archiveNew) != 1 {
		t.Errorf("expected 1 file in .Archive, got cur=%d new=%d", len(archiveCur), len(archiveNew))
	}
}

func TestArchiveEmailRequiresArchiveFolder(t *testing.T) {
	root := makeMaildir(t) // no .Archive
	dropMessage(t, root, "1700000000.a.host", "a", "body", time.Now())

	p := newProvider(t, root)
	emails, _ := p.FetchEmails(context.Background(), "INBOX", 10, 0)
	err := p.ArchiveEmail(context.Background(), "INBOX", emails[0].UID)
	if !errors.Is(err, backend.ErrNotSupported) {
		t.Errorf("want ErrNotSupported, got %v", err)
	}
}

func TestSendEmailNotSupported(t *testing.T) {
	root := makeMaildir(t)
	p := newProvider(t, root)
	if err := p.SendEmail(context.Background(), &backend.OutgoingEmail{}); !errors.Is(err, backend.ErrNotSupported) {
		t.Errorf("want ErrNotSupported, got %v", err)
	}
}

func TestSearchFiltersBySubject(t *testing.T) {
	root := makeMaildir(t)
	t0 := time.Now()
	dropMessage(t, root, "k1.host", "alpha report", "x", t0)
	dropMessage(t, root, "k2.host", "beta notice", "y", t0)

	p := newProvider(t, root)
	results, err := p.Search(context.Background(), "INBOX", backend.SearchQuery{Subject: "alpha"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || !strings.Contains(results[0].Subject, "alpha") {
		t.Errorf("want one alpha result, got %+v", results)
	}
}

func TestCapabilitiesReflectsArchivePresence(t *testing.T) {
	root := makeMaildir(t)
	pNoArchive := newProvider(t, root)
	if pNoArchive.Capabilities().CanArchive {
		t.Error("CanArchive should be false without .Archive subfolder")
	}

	rootWithArchive := makeMaildir(t, ".Archive")
	pArchive := newProvider(t, rootWithArchive)
	caps := pArchive.Capabilities()
	if !caps.CanArchive {
		t.Error("CanArchive should be true when .Archive exists")
	}
	if caps.CanSend {
		t.Error("CanSend must be false for Maildir")
	}
	if !caps.CanFetchFolders {
		t.Error("CanFetchFolders must be true")
	}
}
