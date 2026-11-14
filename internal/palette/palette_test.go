package palette

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/tui"
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

// TestQuitCommandReplaysCtrlC guards against data loss: quitting from the
// palette must take the same path as pressing ctrl+c (which saves an open
// draft and shuts the service down) instead of emitting tea.QuitMsg directly.
func TestQuitCommandReplaysCtrlC(t *testing.T) {
	for _, c := range BuildCommands(nil, nil, nil) {
		if c.Title != "Quit Matcha" {
			continue
		}
		msg := c.Action()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatal("Quit command must not emit tea.QuitMsg directly (skips draft saving)")
		}
		key, ok := msg.(tea.KeyPressMsg)
		if !ok {
			t.Fatalf("Quit command should replay a key press, got %T", msg)
		}
		if key.String() != "ctrl+c" {
			t.Fatalf("Quit command should replay ctrl+c, got %q", key.String())
		}
		return
	}
	t.Fatal("Quit command not found")
}

// TestHandleMouseClickRunsCommand clicks a command row of the centered
// palette and expects its action to run and the palette to close.
func TestHandleMouseClickRunsCommand(t *testing.T) {
	const screenW, screenH = 100, 30
	p := New()
	p.Open([]tui.PaletteCommand{
		{Title: "First", Action: func() tea.Msg { return "first" }},
		{Title: "Second", Action: func() tea.Msg { return "second" }},
	}, screenW, screenH)

	box := strings.Split(ansi.Strip(p.Render("", screenW, screenH)), "\n")
	clickX, clickY := -1, -1
	for y, line := range box {
		if x := strings.Index(line, "Second"); x >= 0 {
			clickX, clickY = len([]rune(line[:x])), y
		}
	}
	if clickY < 0 {
		t.Fatal("rendered palette has no \"Second\" row")
	}

	cmd := p.HandleMouse(tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("clicking a command row should return its action")
	}
	if got := cmd(); got != "second" {
		t.Fatalf("clicked row ran %v, want \"second\"", got)
	}
	if p.IsOpen() {
		t.Fatal("palette should close after running a clicked command")
	}
}

// TestHandleMouseClickOutsideCloses dismisses the palette on a click outside
// its box without running anything.
func TestHandleMouseClickOutsideCloses(t *testing.T) {
	p := New()
	p.Open([]tui.PaletteCommand{{Title: "Only", Action: func() tea.Msg { return "only" }}}, 100, 30)
	if cmd := p.HandleMouse(tea.MouseClickMsg{X: 0, Y: 0, Button: tea.MouseLeft}); cmd != nil {
		t.Fatal("a click outside the palette must not run a command")
	}
	if p.IsOpen() {
		t.Fatal("a click outside the palette should close it")
	}
}
