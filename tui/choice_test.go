package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func newTestChoice(t *testing.T, width, height int) Choice {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	c := NewChoice()
	c.UpdateAvailable = true
	c.LatestVersion = "v9.9.9"
	c.CurrentVersion = "v1.0.0"
	model, _ := c.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return model.(Choice)
}

// TestChoiceFitsScreen wraps the update notice and help instead of letting
// them overflow the terminal.
func TestChoiceFitsScreen(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 20}} {
		c := newTestChoice(t, size[0], size[1])
		lines := strings.Split(ansi.Strip(c.View().Content), "\n")
		if len(lines) > size[1] {
			t.Errorf("%dx%d: %d rows", size[0], size[1], len(lines))
		}
		for i, line := range lines {
			if w := ansi.StringWidth(line); w > size[0] {
				t.Errorf("%dx%d: row %d is %d wide", size[0], size[1], i, w)
			}
		}
	}
}

// TestChoiceClickOnlyOnEntries maps clicks to entries only inside the menu
// column of their rows.
func TestChoiceClickOnlyOnEntries(t *testing.T) {
	c := newTestChoice(t, 80, 24)
	row := -1
	for i, line := range strings.Split(ansi.Strip(c.View().Content), "\n") {
		if strings.Contains(line, "Settings") {
			row = i
		}
	}
	if row < 0 {
		t.Fatal("Settings entry not rendered")
	}

	if _, cmd := c.Update(tea.MouseClickMsg{X: 75, Y: row, Button: tea.MouseLeft}); cmd != nil {
		t.Fatal("a click far right of an entry must not activate it")
	}
	if _, cmd := c.Update(tea.MouseClickMsg{X: 5, Y: row - 1 - len(c.choices), Button: tea.MouseLeft}); cmd != nil {
		t.Fatal("a click above the menu must not activate an entry")
	}
	_, cmd := c.Update(tea.MouseClickMsg{X: 5, Y: row, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("a click on the entry should activate it")
	}
	if _, ok := cmd().(GoToSettingsMsg); !ok {
		t.Fatal("clicking Settings should open settings")
	}
}
