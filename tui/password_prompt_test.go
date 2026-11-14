package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// A second enter while the password is being verified must not start
// another verification.
func TestPasswordPromptIgnoresInputWhileVerifying(t *testing.T) {
	p := NewPasswordPrompt()
	for _, r := range "pw" {
		p.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if _, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil {
		t.Fatal("expected the first enter to start verification")
	}
	if _, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil {
		t.Fatal("second enter started another verification")
	}
}
