package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
)

func TestIsTypeableKey(t *testing.T) {
	for k, want := range map[string]bool{
		"q": true, "space": true, "/": true,
		"ctrl+q": false, "f10": false, "esc": false, "enter": false, "": false,
	} {
		if got := IsTypeableKey(k); got != want {
			t.Errorf("IsTypeableKey(%q) = %v, want %v", k, got, want)
		}
	}
}

func TestDraftsIsFilterActive(t *testing.T) {
	d := NewDrafts([]config.Draft{{ID: "d1", Subject: "hello"}})
	if d.IsFilterActive() {
		t.Error("filter should not be active initially")
	}
	d.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	if !d.IsFilterActive() {
		t.Error("filter should be active after /")
	}
}

func quits(cmd tea.Cmd) bool {
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
				if quits(c) {
					return true
				}
			}
		}
	case <-time.After(200 * time.Millisecond):
	}
	return false
}

func TestEditorsTypeableQuitKey(t *testing.T) {
	orig := config.Keybinds
	t.Cleanup(func() { config.Keybinds = orig })

	newScreens := func() map[string]tea.Model {
		return map[string]tea.Model{
			"composer":     NewComposer("", "", "", "", true),
			"contact":      NewContactEditor(),
			"mailing list": NewMailingListEditor(),
		}
	}

	config.Keybinds.Global.Quit = "q"
	for name, m := range newScreens() {
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
		if quits(cmd) {
			t.Errorf("%s: typeable quit key q must not quit", name)
		}
		if got := m.View().Content; !contains(got, "q") {
			t.Errorf("%s: q should reach the input", name)
		}
	}

	config.Keybinds.Global.Quit = "ctrl+q"
	for name, m := range newScreens() {
		_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
		if !quits(cmd) {
			t.Errorf("%s: ctrl+q should still quit", name)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
