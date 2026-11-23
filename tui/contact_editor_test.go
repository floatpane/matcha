package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
)

// Regression: a terminal narrower than 4 columns gave the inputs a negative
// width and bubbles' textinput panicked rendering its placeholder.
func TestEditorsTinyWindowNoPanic(t *testing.T) {
	for w := 0; w <= 5; w++ {
		ce := NewContactEditor()
		ce.Update(tea.WindowSizeMsg{Width: w, Height: 3})
		_ = ce.View()

		ml := NewMailingListEditor()
		ml.Update(tea.WindowSizeMsg{Width: w, Height: 3})
		_ = ml.View()
	}
}

// Regression: with suggestions open, j/k were swallowed as navigation keys,
// so addresses containing those letters could not be typed.
func TestContactEditorSuggestionsDoNotEatLetters(t *testing.T) {
	ce := NewContactEditor()
	ce.focus = 1
	ce.nameInput.Blur()
	ce.emailInput.Focus()
	ce.emailInput.SetValue("ma")
	ce.lastEmailValue = "ma"
	ce.suggestions = []config.Contact{{Email: "mark@example.com"}, {Email: "mary@example.com"}}
	ce.showSuggestions = true

	for _, r := range "jk" {
		ce.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if got := ce.emailInput.Value(); got != "majk" {
		t.Fatalf("email = %q, want %q", got, "majk")
	}
}

func TestContactEditorDownSwitchesFields(t *testing.T) {
	ce := NewContactEditor()
	ce.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if ce.focus != 1 {
		t.Fatalf("focus after down = %d, want 1", ce.focus)
	}
}
