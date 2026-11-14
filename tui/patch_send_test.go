package tui

import (
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/config"
)

// Regression: navigation was bound to the global j/k keys, so those letters
// could not be typed into the repo, range or address fields.
func TestPatchSendTypingJK(t *testing.T) {
	p := NewPatchSend(nil)
	p.focusIndex = psFocusTo
	p.updateFocus()
	for _, r := range "jake" {
		p.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := p.toInput.Value(); got != "jake" {
		t.Fatalf("To = %q, want %q", got, "jake")
	}
	if p.focusIndex != psFocusTo {
		t.Fatalf("typing moved focus to %d", p.focusIndex)
	}

	p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if p.focusIndex != psFocusCc {
		t.Fatalf("down moved focus to %d, want %d", p.focusIndex, psFocusCc)
	}
}

func TestPatchSendFromUsesAccountAddress(t *testing.T) {
	p := NewPatchSend([]config.Account{
		{ID: "a", Name: "Drew", Email: "drew@example.com"},
		{ID: "b", Name: "Work", Email: "work@example.com"},
	})
	if view := ansi.Strip(p.View().Content); !strings.Contains(view, "From: Drew <drew@example.com>") {
		t.Fatalf("From line missing the account address:\n%s", view)
	}

	// The account selector comes first in the tab order and cycles with →.
	p.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if p.focusIndex != psFocusFrom {
		t.Fatalf("focus = %d, want the From selector", p.focusIndex)
	}
	p.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if p.selectedAccount != 1 {
		t.Fatalf("selected account = %d, want 1", p.selectedAccount)
	}
}

// Regression: previewing a bad repository let git print "fatal: not a git
// repository" straight onto the TUI; the reason now lands in the form.
func TestPatchSendPreviewCapturesGitStderr(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	p := NewPatchSend(nil)
	p.repoInput.SetValue(t.TempDir()) // not a git repository
	p.commitInput.SetValue("HEAD~1..HEAD")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr := os.Stderr
	os.Stderr = w
	_, cmd := p.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if cmd != nil {
		p.Update(cmd())
	}
	os.Stderr = stderr
	_ = w.Close()
	leaked, _ := io.ReadAll(r)

	if len(leaked) > 0 {
		t.Fatalf("git wrote to the terminal: %q", leaked)
	}
	if !strings.Contains(p.errMsg, "not a git repository") {
		t.Fatalf("errMsg = %q, want git's reason", p.errMsg)
	}
}
