package exportcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/tui"
)

// Folder views always pass MailboxInbox; the download used to fetch the part
// from the INBOX message that happened to share the UID.
func TestDownloadAttachmentUsesFolderAndUID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	type call struct {
		folder string
		uid    uint32
		part   string
	}
	var got []call
	origFetch, origOpen := fetchFolderAttachment, openFile
	t.Cleanup(func() { fetchFolderAttachment, openFile = origFetch, origOpen })
	fetchFolderAttachment = func(_ *config.Account, folder string, uid uint32, partID, _ string) ([]byte, error) {
		got = append(got, call{folder, uid, partID})
		return []byte("pdf bytes"), nil
	}
	opened := make(chan string, 1)
	openFile = func(p string) { opened <- p }

	acct := &config.Account{ID: "a", Email: "a@example.com"}
	msg := tui.DownloadAttachmentMsg{
		Filename:  "invoice.pdf",
		PartID:    "2",
		AccountID: "a",
		Mailbox:   tui.MailboxInbox, // what folder views send
		UID:       17,
		Folder:    "Archive",
	}
	res := DownloadAttachmentCmd(acct, 999 /* stale index-derived UID */, msg)().(tui.AttachmentDownloadedMsg)
	if res.Err != nil {
		t.Fatal(res.Err)
	}
	if len(got) != 1 || got[0] != (call{"Archive", 17, "2"}) {
		t.Fatalf("fetch calls = %+v", got)
	}
	data, err := os.ReadFile(res.Path)
	if err != nil || string(data) != "pdf bytes" {
		t.Fatalf("saved %q (%v)", data, err)
	}
	if filepath.Dir(res.Path) != filepath.Join(home, "Downloads") {
		t.Fatalf("saved to %s", res.Path)
	}
	if p := <-opened; p != res.Path {
		t.Fatalf("opened %s", p)
	}
}

func TestDownloadAttachmentPrefersDecodedDataAndRejectsUnknownMailbox(t *testing.T) {
	acct := &config.Account{ID: "a"}
	data, err := fetchAttachmentData(acct, 1, tui.DownloadAttachmentMsg{Data: []byte("inline"), Folder: "Work"})
	if err != nil || string(data) != "inline" {
		t.Fatalf("data=%q err=%v", data, err)
	}
	if _, err := fetchAttachmentData(acct, 1, tui.DownloadAttachmentMsg{Mailbox: ""}); err == nil {
		t.Fatal("an unknown mailbox must be an error, not an empty file")
	}
}
