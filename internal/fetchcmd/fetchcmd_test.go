package fetchcmd

import (
	"errors"
	"testing"
	"time"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/tui"
)

func testConfig(ids ...string) *config.Config {
	cfg := &config.Config{}
	for _, id := range ids {
		cfg.Accounts = append(cfg.Accounts, config.Account{ID: id, Email: id + "@example.com", SC: &config.SessionCache{}})
	}
	return cfg
}

// A panic while fetching/parsing a body (e.g. a malformed S/MIME payload)
// must come back as an error message, not crash the program.
func TestBodyCmdsRecoverPanics(t *testing.T) {
	orig := fetchFolderEmailBody
	t.Cleanup(func() { fetchFolderEmailBody = orig })
	fetchFolderEmailBody = func(*config.Account, string, uint32) (string, string, []fetcher.Attachment, error) {
		panic("index out of range [2] with length 2")
	}
	cfg := testConfig("a")

	msg := FetchFolderEmailBodyCmd(cfg, 7, "a", "Work", tui.MailboxInbox)()
	body, ok := msg.(tui.EmailBodyFetchedMsg)
	if !ok || body.Err == nil {
		t.Fatalf("FetchFolderEmailBodyCmd = %#v, want EmailBodyFetchedMsg with Err", msg)
	}
	if body.UID != 7 || body.AccountID != "a" || body.Folder != "Work" {
		t.Errorf("identity not preserved: %+v", body)
	}

	msg = FetchPreviewBodyCmd(cfg, 7, "a", "Work")()
	preview, ok := msg.(tui.PreviewBodyFetchedMsg)
	if !ok || preview.Err == nil || preview.Folder != "Work" {
		t.Fatalf("FetchPreviewBodyCmd = %#v, want PreviewBodyFetchedMsg with Err and Folder", msg)
	}
}

func TestBodyCmdsCarryFolder(t *testing.T) {
	orig := fetchFolderEmailBody
	t.Cleanup(func() { fetchFolderEmailBody = orig })
	fetchFolderEmailBody = func(_ *config.Account, folder string, _ uint32) (string, string, []fetcher.Attachment, error) {
		return "body from " + folder, "text/plain", nil, nil
	}
	cfg := testConfig("a")
	preview := FetchPreviewBodyCmd(cfg, 1, "a", "Archive")().(tui.PreviewBodyFetchedMsg)
	if preview.Folder != "Archive" || preview.Body != "body from Archive" {
		t.Errorf("preview = %+v", preview)
	}
	full := FetchFolderEmailBodyCmd(cfg, 1, "a", "Archive", tui.MailboxInbox)().(tui.EmailBodyFetchedMsg)
	if full.Folder != "Archive" {
		t.Errorf("full = %+v", full)
	}
}

// A failing account must be reported, not silently turned into "no emails".
func TestFetchFolderEmailsReportsErrors(t *testing.T) {
	orig := fetchFolderEmails
	t.Cleanup(func() { fetchFolderEmails = orig })
	now := time.Now()
	fetchFolderEmails = func(acc *config.Account, _ string, _, _ uint32) ([]fetcher.Email, error) {
		if acc.ID == "down" {
			return nil, errors.New("dial tcp: connection refused")
		}
		return []fetcher.Email{
			{UID: 1, AccountID: acc.ID, Date: now.Add(-time.Hour)},
			{UID: 2, AccountID: acc.ID, Date: now},
		}, nil
	}

	msg := FetchFolderEmailsCmd(testConfig("up", "down"), "INBOX")().(tui.FolderEmailsFetchedMsg)
	if msg.Errors["down"] == nil {
		t.Fatalf("expected error for account 'down', got %v", msg.Errors)
	}
	if _, ok := msg.Errors["up"]; ok {
		t.Fatalf("unexpected error for account 'up'")
	}
	if len(msg.Emails) != 2 || msg.Emails[0].UID != 2 {
		t.Fatalf("emails not sorted newest first: %+v", msg.Emails)
	}

	ok := FetchFolderEmailsCmd(testConfig("up"), "INBOX")().(tui.FolderEmailsFetchedMsg)
	if ok.Errors != nil {
		t.Fatalf("Errors should be nil when every account succeeds, got %v", ok.Errors)
	}
}

func TestMergeFoldersSumsUnread(t *testing.T) {
	accounts := testConfig("a", "b").Accounts
	merged := MergeFolders(accounts, map[string][]fetcher.Folder{
		"a": {{Name: "INBOX", Unread: 3}, {Name: "Work", Unread: 1}},
		"b": {{Name: "INBOX", Unread: 4}},
	})
	got := map[string]uint32{}
	for _, f := range merged {
		got[f.Name] = f.Unread
	}
	if len(merged) != 2 || got["INBOX"] != 7 || got["Work"] != 1 {
		t.Fatalf("MergeFolders = %+v", merged)
	}
}

// Commands must not alias cfg.Accounts: the UI goroutine can remove an
// account (shifting the slice in place) while the command runs.
func TestCommandsSnapshotAccounts(t *testing.T) {
	orig := fetchFolderEmails
	t.Cleanup(func() { fetchFolderEmails = orig })
	seen := make(chan string, 4)
	fetchFolderEmails = func(acc *config.Account, _ string, _, _ uint32) ([]fetcher.Email, error) {
		seen <- acc.ID
		return nil, nil
	}
	cfg := testConfig("a", "b", "c")
	cmd := FetchFolderEmailsCmd(cfg, "INBOX")
	cfg.Accounts = append(cfg.Accounts[:0], cfg.Accounts[1:]...) // delete "a" in place
	_ = cmd()
	close(seen)
	ids := map[string]bool{}
	for id := range seen {
		ids[id] = true
	}
	if !ids["a"] || !ids["b"] || !ids["c"] || len(ids) != 3 {
		t.Fatalf("command saw accounts %v, want the snapshot a,b,c", ids)
	}
}
