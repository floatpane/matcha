package app

import (
	"testing"

	"github.com/floatpane/matcha/config"
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
