package palette

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
		if got := KeyMsgFromBinding(b).String(); got != b {
			t.Errorf("KeyMsgFromBinding(%q).String() = %q, want %q", b, got, b)
		}
	}
}

func TestKeyActionEmptyBindingIsNil(t *testing.T) {
	if KeyAction("") != nil {
		t.Error("KeyAction(\"\") should return nil")
	}
	if KeyAction("r") == nil {
		t.Error("KeyAction(\"r\") should return a non-nil action")
	}
}

// TestBuildPaletteCommandsAlwaysHasNav ensures global navigation commands are
// present regardless of the active view, and that each has a runnable action.
func TestBuildPaletteCommandsAlwaysHasNav(t *testing.T) {
	cmds := BuildCommands(nil, nil, nil)

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
	for _, c := range BuildCommands(nil, nil, nil) {
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
