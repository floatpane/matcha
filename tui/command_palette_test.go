package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestCommandPaletteRenderKeepsBackgroundOnBoxRows verifies the palette is drawn
// as a floating layer: background content to the left and right of the box must
// remain visible on the same rows the box occupies (the whole point of using a
// canvas instead of blanking full rows).
func TestCommandPaletteRenderKeepsBackgroundOnBoxRows(t *testing.T) {
	const screenW, screenH = 100, 24

	// Background: every row is "L" + dots + "R" filling the full width, so a
	// surviving row shows both edge markers around the centered box.
	bgLine := "L" + strings.Repeat(".", screenW-2) + "R"
	bg := strings.Repeat(bgLine+"\n", screenH-1) + bgLine

	p := NewCommandPalette([]PaletteCommand{
		{Title: "Refresh", Hint: "r"},
		{Title: "Compose new email"},
	}, screenW, screenH)

	out := ansi.Strip(p.Render(bg, screenW, screenH))
	lines := strings.Split(out, "\n")

	var boxRows int
	for _, ln := range lines {
		if !strings.Contains(ln, "│") { // a row occupied by the box border
			continue
		}
		boxRows++
		if !strings.HasPrefix(ln, "L") {
			t.Errorf("box row lost left background marker: %q", ln)
		}
		if !strings.HasSuffix(strings.TrimRight(ln, " "), "R") {
			t.Errorf("box row lost right background marker: %q", ln)
		}
	}
	if boxRows == 0 {
		t.Fatal("no box rows found in composited output")
	}
	if !strings.Contains(out, "Refresh") {
		t.Error("composited output missing palette content")
	}
}

// TestCommandPaletteFilter narrows the list by query and keeps a runnable
// selection.
func TestCommandPaletteFilter(t *testing.T) {
	p := NewCommandPalette([]PaletteCommand{
		{Title: "Refresh", Action: func() tea.Msg { return "refresh" }},
		{Title: "Compose new email", Action: func() tea.Msg { return "compose" }},
		{Title: "Settings", Action: func() tea.Msg { return "settings" }},
	}, 80, 24)

	// Type "comp" → only "Compose new email" matches (subsequence).
	for _, r := range "comp" {
		p.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if len(p.filtered) != 1 {
		t.Fatalf("expected 1 match for 'comp', got %d", len(p.filtered))
	}
	cmd := p.SelectedCmd()
	if cmd == nil {
		t.Fatal("expected a selected command")
	}
	if msg := cmd(); msg != "compose" {
		t.Errorf("selected wrong command: got %v", msg)
	}
}
