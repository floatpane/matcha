package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/internal/palette"
	"github.com/floatpane/matcha/tui"
)

func TestIsQuitKeyUsesConfiguredBinding(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	config.Keybinds.Global.Quit = "q"
	if !isQuitKey("q") {
		t.Error("configured quit key q should quit")
	}
	if !isQuitKey("ctrl+c") {
		t.Error("ctrl+c should always quit")
	}
	if isQuitKey("x") {
		t.Error("unbound key should not quit")
	}

	config.Keybinds.Global.Quit = ""
	if isQuitKey("") {
		t.Error("empty binding must not match an empty key string")
	}
}

func TestIsCancelKeyUsesConfiguredBinding(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	config.Keybinds.Global.Cancel = "b"
	if !isCancelKey("b") {
		t.Error("configured cancel key b should cancel")
	}
	if isCancelKey("esc") {
		t.Error("esc should not cancel once rebound")
	}

	config.Keybinds.Global.Cancel = ""
	if isCancelKey("") {
		t.Error("empty binding must not match an empty key string")
	}
}

func TestIsPaletteQuitKey(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	tests := []struct {
		quit, key string
		want      bool
	}{
		{"ctrl+c", "ctrl+c", true},
		{"q", "ctrl+c", true},
		{"q", "q", false},
		{"space", "space", false},
		{"enter", "enter", false},
		{"esc", "esc", false},
		{"ctrl+q", "ctrl+q", true},
		{"f10", "f10", true},
		{"", "", false},
		{"ctrl+q", "x", false},
	}
	for _, tt := range tests {
		config.Keybinds.Global.Quit = tt.quit
		if got := isPaletteQuitKey(tt.key); got != tt.want {
			t.Errorf("quit=%q key=%q: got %v, want %v", tt.quit, tt.key, got, tt.want)
		}
	}
}

func TestPaletteQuitKeyHandling(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	open := func() *Model {
		m := NewModel(&config.Config{}, nil, nil, false, nil, nil)
		m.palette.Open([]tui.PaletteCommand{{Title: "Test", Action: func() tea.Msg { return nil }}}, 80, 24)
		return m
	}

	config.Keybinds.Global.Quit = "q"
	m := open()
	m.Update(palette.KeyMsgFromBinding("q"))
	if !m.palette.IsOpen() {
		t.Error("printable quit key must be typed into the palette filter, not close it")
	}

	config.Keybinds.Global.Quit = "ctrl+q"
	m = open()
	_, cmd := m.Update(palette.KeyMsgFromBinding("ctrl+q"))
	if cmd == nil {
		t.Fatal("ctrl+q should return a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("ctrl+q should quit")
	}
}
