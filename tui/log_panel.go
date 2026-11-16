package tui

import (
	"strings"
	"unicode"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/floatpane/matcha/internal/logging"
	"github.com/floatpane/matcha/theme"
)

type LogPanel struct {
	logger logging.Logger
	width  int
	height int
}

func NewLogPanel(logger logging.Logger) *LogPanel {
	return &LogPanel{logger: logger}
}

func (p *LogPanel) SetSize(width, height int) {
	p.width = width
	p.height = height
}

func (p *LogPanel) View() string {
	innerHeight := max(p.height-1, 2)
	visibleLogLines := max(innerHeight-1, 1)

	lines := p.tailLines(visibleLogLines)
	if len(lines) == 0 {
		lines = []string{t("common.no_logs_yet")}
	}

	innerWidth := max(p.width, 1)
	for i, line := range lines {
		lines[i] = ansi.Truncate(singleRowText(line), innerWidth, "…")
	}

	header := lipgloss.NewStyle().
		Foreground(theme.ActiveTheme.Accent).
		Bold(true).
		Render("[" + t("common.logs") + "]")
	separator := lipgloss.NewStyle().
		BorderForeground(theme.ActiveTheme.Secondary).
		Render(strings.Repeat("─", p.width))
	body := header + "\n" + strings.Join(lines, "\n")
	content := lipgloss.NewStyle().
		Width(p.width).
		Height(innerHeight).
		MaxHeight(innerHeight).
		Foreground(theme.ActiveTheme.SubtleText).
		Render(body)

	return lipgloss.JoinVertical(lipgloss.Left, separator, content)
}

// rowTabWidth matches lipgloss' tab expansion when text is rendered.
const rowTabWidth = 4

// singleRowText makes text safe to measure and draw on a single row. Escape
// sequences are stripped, tabs are expanded (ansi.Truncate counts a tab as
// zero cells but lipgloss draws it four wide, which pushed the newest log
// line out of the panel), newlines become spaces and other control
// characters are dropped.
func singleRowText(line string) string {
	line = ansi.Strip(line)
	var b strings.Builder
	b.Grow(len(line))
	for _, r := range line {
		switch {
		case r == '\t':
			b.WriteString(strings.Repeat(" ", rowTabWidth))
		case r == '\n':
			b.WriteByte(' ')
		case unicode.IsControl(r):
			// dropped
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (p *LogPanel) tailLines(n int) []string {
	if p.logger == nil {
		return nil
	}

	entries := p.logger.Tail(n)
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, entry.Text)
	}
	return lines
}
