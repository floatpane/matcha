package view

import (
	"os"
	"runtime"
	"testing"
)

func TestMaxImageCellHeight(t *testing.T) {
	old := terminalSize
	defer func() { terminalSize = old }()

	terminalSize = struct {
		cols, rows int
		ok         bool
	}{cols: 100, rows: 40, ok: true}

	if got := maxImageCellHeight(); got != 32 {
		t.Fatalf("expected maxImageCellHeight=32 for 40-row terminal, got %d", got)
	}
}

func TestMaxImageCellWidth(t *testing.T) {
	old := terminalSize
	defer func() { terminalSize = old }()

	terminalSize = struct {
		cols, rows int
		ok         bool
	}{cols: 100, rows: 40, ok: true}

	if got := maxImageCellWidth(); got != 96 {
		t.Fatalf("expected maxImageCellWidth=96 for 100-col terminal, got %d", got)
	}
}

func TestMaxImageCellDefaultsWhenNoTerminal(t *testing.T) {
	old := terminalSize
	defer func() { terminalSize = old }()

	terminalSize = struct {
		cols, rows int
		ok         bool
	}{}

	if got := maxImageCellHeight(); got != 25 {
		t.Fatalf("expected default height 25, got %d", got)
	}
	if got := maxImageCellWidth(); got != 80 {
		t.Fatalf("expected default width 80, got %d", got)
	}
}

func TestTerminalSizeFrom(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("TIOCGWINSZ is unavailable on Windows")
	}
	// The current process stdin/stdout may or may not be a terminal. We just
	// verify the helper doesn't panic and returns sensible values when given a
	// valid file.
	ts, ok := terminalSizeFrom(os.Stdout)
	if ok {
		if ts.cols < 1 || ts.rows < 1 {
			t.Fatalf("unexpected terminal size: %+v", ts)
		}
	}
}
