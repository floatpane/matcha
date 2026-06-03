package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// TestKeyMsgFromBindingRoundTrips verifies that a synthetic key press built
// from a binding string reproduces that exact string via String(), which is
// how the views match keybindings. If this breaks, palette actions silently
// stop triggering their handlers.
func TestKeyMsgFromBindingRoundTrips(t *testing.T) {
	bindings := []string{
		"r", "f", "d", "a", "i", "v", "m", "/", "[", "]",
		"T", "1", "2", "3",
		"tab", "shift+tab", "enter", "esc",
		"ctrl+e", "ctrl+n", "ctrl+p", "ctrl+k",
	}
	for _, b := range bindings {
		if got := keyMsgFromBinding(b).String(); got != b {
			t.Errorf("keyMsgFromBinding(%q).String() = %q, want %q", b, got, b)
		}
	}
}

func TestKeyActionEmptyBindingIsNil(t *testing.T) {
	if keyAction("") != nil {
		t.Error("keyAction(\"\") should return nil")
	}
	if keyAction("r") == nil {
		t.Error("keyAction(\"r\") should return a non-nil action")
	}
}

// TestBuildPaletteCommandsAlwaysHasNav ensures global navigation commands are
// present regardless of the active view, and that each has a runnable action.
func TestBuildPaletteCommandsAlwaysHasNav(t *testing.T) {
	m := &mainModel{}
	cmds := m.buildPaletteCommands()

	want := map[string]bool{
		"Compose new email": false,
		"Settings":          false,
		"Quit Matcha":       false,
	}
	for _, c := range cmds {
		if _, ok := want[c.Title]; ok {
			want[c.Title] = true
		}
		if c.Action != nil {
			if msg := c.Action(); msg == nil {
				t.Errorf("command %q action returned nil msg", c.Title)
			}
		}
	}
	for title, found := range want {
		if !found {
			t.Errorf("expected global command %q to be present", title)
		}
	}
}

func TestQuitCommandEmitsQuit(t *testing.T) {
	m := &mainModel{}
	for _, c := range m.buildPaletteCommands() {
		if c.Title != "Quit Matcha" {
			continue
		}
		if _, ok := c.Action().(tea.QuitMsg); !ok {
			t.Errorf("Quit command should emit tea.QuitMsg, got %T", c.Action())
		}
		return
	}
	t.Fatal("Quit command not found")
}
