package tui

import (
	"strings"

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
		lines[i] = ansi.Truncate(line, innerWidth, "…")
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
