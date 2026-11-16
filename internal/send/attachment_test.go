package send

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
	"github.com/floatpane/matcha/tui"
)

type recordingService struct {
	daemonclient.Service
	queued bool
}

func (r *recordingService) QueueEmail(string, []string, []string, []string, string, string, string, map[string][]byte, map[string][]byte, string, []string, bool, bool, bool, bool, int, []byte) (string, error) {
	r.queued = true
	return "job", nil
}

// An unreadable attachment used to be logged and skipped, sending the email
// without it.
func TestSendEmailFailsOnUnreadableAttachment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	svc := &recordingService{}
	deps := &Dependencies{
		Service: svc,
		Config:  &config.Config{Accounts: []config.Account{{ID: "a", Email: "a@example.com"}}},
	}
	missing := filepath.Join(t.TempDir(), "report.pdf")
	msg := SendEmailCmd(deps, tui.SendEmailMsg{
		To:              "b@example.com",
		Subject:         "report",
		Body:            "attached",
		AccountID:       "a",
		AttachmentPaths: []string{missing},
	})()
	res, ok := msg.(tui.EmailResultMsg)
	if !ok || res.Err == nil || !strings.Contains(res.Err.Error(), "report.pdf") {
		t.Fatalf("got %#v, want EmailResultMsg error naming the attachment", msg)
	}
	if svc.queued {
		t.Fatal("email was queued without its attachment")
	}
}
