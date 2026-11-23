package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestLogPanelTabsDoNotHideNewestLine: a tab measures zero cells when
// truncated but renders four wide, so an unexpanded tab made the line wrap
// and pushed the newest entry out of the panel.
func TestLogPanelTabsDoNotHideNewestLine(t *testing.T) {
	logger := &snapshotLogger{}
	logger.Write([]byte("first\n"))
	logger.Write([]byte("tabbed" + strings.Repeat("\t", 12) + "tail\x07\n"))
	logger.Write([]byte("newest\n"))

	panel := NewLogPanel(logger)
	panel.SetSize(40, 5)
	view := ansi.Strip(panel.View())
	lines := strings.Split(view, "\n")
	if len(lines) != 5 {
		t.Fatalf("panel is %d rows, want 5", len(lines))
	}
	if !strings.Contains(view, "newest") {
		t.Fatalf("newest log line hidden:\n%s", view)
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > 40 {
			t.Fatalf("row %d is %d wide", i, w)
		}
		if strings.ContainsAny(line, "\t\x07") {
			t.Fatalf("row %d still contains control characters: %q", i, line)
		}
	}
}
