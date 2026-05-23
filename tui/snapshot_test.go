package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/internal/logging"
)

var updateGolden = flag.Bool("update", false, "update golden snapshot files")

// snapshotLogger is a deterministic in-memory logger for snapshot tests.
type snapshotLogger struct {
	entries []logging.Entry
}

func (l *snapshotLogger) Write(p []byte) (int, error) {
	l.entries = append(l.entries, logging.Entry{Text: strings.TrimRight(string(p), "\n")})
	return len(p), nil
}
func (l *snapshotLogger) MaxEntries() int { return logging.DefaultMaxEntries }
func (l *snapshotLogger) Tail(n int) []logging.Entry {
	if n <= 0 || len(l.entries) == 0 {
		return nil
	}
	if n >= len(l.entries) {
		out := make([]logging.Entry, len(l.entries))
		copy(out, l.entries)
		return out
	}
	out := make([]logging.Entry, n)
	copy(out, l.entries[len(l.entries)-n:])
	return out
}
func (l *snapshotLogger) Subscribe() <-chan logging.Entry { return nil }

// assertGolden compares rendered output to a golden file in testdata/golden.
// Re-run tests with `-update` to refresh the golden files.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()

	got = normalizeForGolden(got)

	path := filepath.Join("testdata", "golden", name+".txt")

	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir golden: %v", err)
		}
		if err := os.WriteFile(path, []byte(got+"\n"), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %q (run with -update to create): %v", path, err)
	}
	wantStr := normalizeForGolden(string(want))
	if got != wantStr {
		t.Fatalf("snapshot mismatch for %s\n--- got ---\n%s\n--- want ---\n%s\n--- diff ---\ngot bytes:  %q\nwant bytes: %q",
			name, got, wantStr, got, wantStr)
	}
}

func normalizeForGolden(s string) string {
	s = ansi.Strip(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = stripTrailingSpace(s)
	return strings.TrimRight(s, " \n\t")
}

func stripTrailingSpace(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	return strings.Join(lines, "\n")
}

func TestSnapshot_LogPanel_Empty(t *testing.T) {
	panel := NewLogPanel(&snapshotLogger{})
	panel.SetSize(60, 6)
	assertGolden(t, "log_panel_empty", panel.View())
}

func TestSnapshot_LogPanel_WithEntries(t *testing.T) {
	logger := &snapshotLogger{}
	logger.Write([]byte("started fetcher\n"))
	logger.Write([]byte("connected to imap.example.com\n"))
	logger.Write([]byte("fetched 12 new messages\n"))

	panel := NewLogPanel(logger)
	panel.SetSize(60, 6)
	assertGolden(t, "log_panel_with_entries", panel.View())
}

func TestSnapshot_LogPanel_TruncatesLongLines(t *testing.T) {
	logger := &snapshotLogger{}
	logger.Write([]byte(strings.Repeat("verylongline ", 20) + "\n"))

	panel := NewLogPanel(logger)
	panel.SetSize(30, 4)
	assertGolden(t, "log_panel_truncated", panel.View())
}

func TestSnapshot_SearchOverlay_Empty(t *testing.T) {
	overlay := NewSearchOverlay(80, 24)
	assertGolden(t, "search_overlay_empty", overlay.View())
}

func TestSnapshot_SearchOverlay_Loading(t *testing.T) {
	overlay := NewSearchOverlay(80, 24)
	overlay.loading = true
	assertGolden(t, "search_overlay_loading", overlay.View())
}

func TestSnapshot_SearchOverlay_Error(t *testing.T) {
	overlay := NewSearchOverlay(80, 24)
	overlay.err = "connection refused"
	assertGolden(t, "search_overlay_error", overlay.View())
}
