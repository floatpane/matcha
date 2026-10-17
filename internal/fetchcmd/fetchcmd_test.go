package fetchcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/tui"
)

func TestFetchFolderEmailsCmdUsesPageLimit(t *testing.T) {
	root := t.TempDir()
	for _, subdir := range []string{"cur", "new", "tmp"} {
		if err := os.Mkdir(filepath.Join(root, subdir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		message := fmt.Sprintf("From: sender@example.com\r\nTo: me@example.com\r\nSubject: message %d\r\nMessage-ID: <message-%d@example.com>\r\n\r\nbody\r\n", i, i)
		if err := os.WriteFile(filepath.Join(root, "new", fmt.Sprintf("message-%d", i)), []byte(message), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{Accounts: []config.Account{{ID: "maildir", Protocol: "maildir", MaildirPath: root}}}
	msg := FetchFolderEmailsCmd(cfg, "INBOX", 2)()
	fetched, ok := msg.(tui.FolderEmailsFetchedMsg)
	if !ok || len(fetched.Emails) != 2 {
		t.Fatalf("first page = %#v, want two messages", msg)
	}
}
