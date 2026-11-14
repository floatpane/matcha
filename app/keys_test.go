package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/internal/palette"
	"github.com/floatpane/matcha/tui"
)

func TestIsQuitKeyUsesConfiguredBinding(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	config.Keybinds.Global.Quit = "q"
	if !isQuitKey("q", true) {
		t.Error("configured quit key q should quit")
	}
	if !isQuitKey("ctrl+c", true) {
		t.Error("ctrl+c should always quit")
	}
	if isQuitKey("x", true) {
		t.Error("unbound key should not quit")
	}

	config.Keybinds.Global.Quit = ""
	if isQuitKey("", true) {
		t.Error("empty binding must not match an empty key string")
	}
}

func TestIsCancelKeyUsesConfiguredBinding(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	config.Keybinds.Global.Cancel = "b"
	if !isCancelKey("b", true) {
		t.Error("configured cancel key b should cancel")
	}
	if isCancelKey("esc", true) {
		t.Error("esc should not cancel once rebound")
	}

	config.Keybinds.Global.Cancel = ""
	if isCancelKey("", true) {
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

func TestIsTypeableKey(t *testing.T) {
	for k, want := range map[string]bool{
		"q": true, "space": true, "é": true, "+": true,
		"ctrl+q": false, "f10": false, "esc": false, "enter": false, "": false,
	} {
		if got := isTypeableKey(k); got != want {
			t.Errorf("isTypeableKey(%q) = %v, want %v", k, got, want)
		}
	}
}

func TestIsQuitKeyTypeable(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	config.Keybinds.Global.Quit = "q"
	if isQuitKey("q", false) {
		t.Error("typeable quit key must not quit on a text-input screen")
	}
	if !isQuitKey("q", true) {
		t.Error("typeable quit key should quit on an allowed screen")
	}
	if !isQuitKey("ctrl+c", false) {
		t.Error("ctrl+c should always quit")
	}
	config.Keybinds.Global.Quit = "space"
	if isQuitKey("space", false) || !isQuitKey("space", true) {
		t.Error("space should be treated as typeable")
	}
	for _, k := range []string{"ctrl+q", "f10", "esc"} {
		config.Keybinds.Global.Quit = k
		if !isQuitKey(k, false) {
			t.Errorf("non-typeable quit key %q should quit everywhere", k)
		}
	}
}

func TestIsCancelKeyTypeable(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	config.Keybinds.Global.Cancel = "esc"
	if !isCancelKey("esc", false) {
		t.Error("esc must still cancel on login and file picker")
	}
	config.Keybinds.Global.Cancel = "b"
	if isCancelKey("b", false) {
		t.Error("typeable cancel key must not act on a text-input screen")
	}
	if !isCancelKey("b", true) {
		t.Error("typeable cancel key should act on an allowed screen")
	}
}

func TestQuitKeyAllowed(t *testing.T) {
	tests := []struct {
		name string
		m    tea.Model
		want bool
	}{
		{"choice", tui.NewChoice(), true},
		{"status", tui.NewStatus("x"), true},
		{"marketplace", tui.NewMarketplace(false), true},
		{"drafts", tui.NewDrafts(nil), true},
		{"setup guide", newSetupGuide(), true},
		{"inbox", tui.NewInbox(nil, nil), true},
		{"folder inbox", tui.NewFolderInbox([]string{"INBOX"}, nil), true},
		{"login", tui.NewLogin(false), false},
		{"password prompt", tui.NewPasswordPrompt(), false},
		{"signature editor", tui.NewSignatureEditor(""), false},
		{"file picker", tui.NewFilePicker(t.TempDir()), false},
	}
	for _, tt := range tests {
		if got := quitKeyAllowed(tt.m); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

// producesQuit runs cmd (bounded to 200ms; a timeout means not quit, e.g. a
// blink tick) and reports whether it yields tea.QuitMsg, looking inside batches.
func producesQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		switch v := msg.(type) {
		case tea.QuitMsg:
			return true
		case tea.BatchMsg:
			for _, c := range v {
				if producesQuit(c) {
					return true
				}
			}
		}
	case <-time.After(200 * time.Millisecond):
	}
	return false
}

func TestQuitKeyScreens(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	config.Keybinds.Global.Quit = "q"
	m := NewModel(&config.Config{}, nil, nil, false, nil, nil)

	m.current = tui.NewLogin(false)
	_, cmd := m.Update(palette.KeyMsgFromBinding("q"))
	if producesQuit(cmd) {
		t.Error("q typed into login must not quit")
	}
	if _, ok := m.current.(*tui.Login); !ok {
		t.Errorf("login should remain current, got %T", m.current)
	}

	m.current = tui.NewChoice()
	_, cmd = m.Update(palette.KeyMsgFromBinding("q"))
	if !producesQuit(cmd) {
		t.Error("q on the choice menu should quit")
	}

	in := tui.NewInbox(nil, nil)
	m.current = in
	m.Update(palette.KeyMsgFromBinding("/"))
	if !in.IsSearchActive() {
		t.Fatal("/ should open inbox search")
	}
	_, cmd = m.Update(palette.KeyMsgFromBinding("q"))
	if producesQuit(cmd) {
		t.Error("q typed into inbox search must not quit")
	}

	fi := tui.NewFolderInbox([]string{"INBOX"}, nil)
	m.current = fi
	m.Update(palette.KeyMsgFromBinding("/"))
	if !fi.GetInbox().IsSearchActive() {
		t.Fatal("/ should open folder inbox search")
	}
	_, cmd = m.Update(palette.KeyMsgFromBinding("q"))
	if producesQuit(cmd) {
		t.Error("q typed into folder inbox search must not quit")
	}

	config.Keybinds.Global.Quit = "ctrl+q"
	m.current = tui.NewLogin(false)
	_, cmd = m.Update(palette.KeyMsgFromBinding("ctrl+q"))
	if !producesQuit(cmd) {
		t.Error("ctrl+q should quit from login")
	}
}

func TestCancelKeyAllowed(t *testing.T) {
	if cancelKeyAllowed(tui.NewLogin(false)) {
		t.Error("typeable cancel key must not act on login")
	}
	if cancelKeyAllowed(tui.NewFilePicker(t.TempDir())) {
		t.Error("typeable cancel key must not act on the file picker")
	}
	if !cancelKeyAllowed(tui.NewChoice()) {
		t.Error("typeable cancel key should act on the choice menu")
	}
}

func TestQuitKeyJumpFilter(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	config.Keybinds.Global.Quit = "q"
	m := NewModel(&config.Config{}, nil, nil, false, nil, nil)
	fi := tui.NewFolderInbox([]string{"INBOX", "Sent"}, nil)
	m.current = fi
	m.Update(tui.JumpToFolderMsg{})
	if fi.IsJumpFilterActive() {
		t.Fatal("jump filter should not be focused before the filter key")
	}
	m.Update(palette.KeyMsgFromBinding(config.Keybinds.Inbox.Filter))
	if !fi.IsJumpFilterActive() {
		t.Fatal("filter key should focus the jump filter")
	}
	_, cmd := m.Update(palette.KeyMsgFromBinding("q"))
	if producesQuit(cmd) {
		t.Error("q typed into the folder jump filter must not quit")
	}
}
