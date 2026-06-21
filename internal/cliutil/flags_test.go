package cliutil

import (
	"strings"
	"testing"

	"github.com/floatpane/matcha/internal/loglevel"
)

func TestParseGlobalFlagsEnablesLogPanel(t *testing.T) {
	args, level, show := ParseGlobalFlags([]string{"matcha", "--debug", "--logs", "--version"})
	if !show {
		t.Fatal("expected log panel flag to be enabled")
	}
	if got := strings.Join(args, " "); got != "matcha --version" {
		t.Fatalf("args = %q, want %q", got, "matcha --version")
	}
	if level != loglevel.LevelDebug {
		t.Fatalf("log level = %v, want debug", level)
	}
}

func TestParseGlobalFlagsDoesNotConsumeSubcommandFlags(t *testing.T) {
	args, _, show := ParseGlobalFlags([]string{"matcha", "send", "--logs"})
	if show {
		t.Fatal("did not expect log panel flag after subcommand to be consumed")
	}
	if got := strings.Join(args, " "); got != "matcha send --logs" {
		t.Fatalf("args = %q, want %q", got, "matcha send --logs")
	}
}
