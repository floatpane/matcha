package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/theme"
)

func (m *Settings) updateTheme(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	themes := theme.AllThemes()

	switch msg.String() {
	case "up", "k":
		if m.themeCursor > 0 {
			m.themeCursor--
		}
	case "down", "j":
		if m.themeCursor < len(themes)-1 {
			m.themeCursor++
		}
	case "enter", "space", "right", "l":
		if m.themeCursor < len(themes) {
			selected := themes[m.themeCursor]
			theme.SetTheme(selected.Name)
			RebuildStyles()
			m.cfg.Theme = selected.Name
			_ = config.SaveConfig(m.cfg)
		}
	}
	return m, nil
}

func (m *Settings) viewTheme() string {
	themes := theme.AllThemes()
	var b strings.Builder

	b.WriteString(titleStyle.Render("Theme") + "\n\n")

	for i, t := range themes {
		isActive := t.Name == theme.ActiveTheme.Name
		label := t.Name
		if isActive {
			label += " (active)"
		}

		cursor := "  "
		style := accountItemStyle
		if m.themeCursor == i {
			cursor = "> "
			style = selectedAccountItemStyle
		}

		b.WriteString(style.Render(cursor+label) + "\n")
	}

	b.WriteString("\n")

	// Preview
	var previewTheme theme.Theme
	if m.themeCursor < len(themes) {
		previewTheme = themes[m.themeCursor]
	} else {
		previewTheme = theme.ActiveTheme
	}

	previewWidth := m.width - 34 - 4
	if previewWidth < 30 {
		previewWidth = 30
	}

	b.WriteString(renderThemePreview(previewTheme, previewWidth) + "\n\n")

	if !m.cfg.HideTips {
		b.WriteString(TipStyle.Render("Tip: Custom themes can be added as JSON files in ~/.config/matcha/themes/") + "\n\n")
	}

	b.WriteString(helpStyle.Render("↑/↓: navigate • enter/space: apply theme"))

	return b.String()
}

// renderThemePreview renders a small mockup showing how a theme looks.
func renderThemePreview(t theme.Theme, previewWidth int) string {
	if previewWidth > 60 {
		previewWidth = 60
	}

	accent := lipgloss.NewStyle().Foreground(t.Accent)
	accentBold := lipgloss.NewStyle().Foreground(t.Accent).Bold(true)
	secondary := lipgloss.NewStyle().Foreground(t.Secondary)
	muted := lipgloss.NewStyle().Foreground(t.MutedText)
	dim := lipgloss.NewStyle().Foreground(t.DimText)
	danger := lipgloss.NewStyle().Foreground(t.Danger)
	warn := lipgloss.NewStyle().Foreground(t.Warning)
	tip := lipgloss.NewStyle().Foreground(t.Tip).Italic(true)
	link := lipgloss.NewStyle().Foreground(t.Link)
	title := lipgloss.NewStyle().Foreground(t.AccentText).Background(t.AccentDark).Padding(0, 1)
	activeTab := lipgloss.NewStyle().Foreground(t.Accent).Bold(true).Underline(true)
	activeFolder := lipgloss.NewStyle().Background(t.Accent).Foreground(t.Contrast).Bold(true).Padding(0, 1)

	var b strings.Builder

	b.WriteString(title.Render("Preview: "+t.Name) + "\n\n")
	b.WriteString(activeTab.Render("Inbox") + "  " + secondary.Render("Sent") + "  " + secondary.Render("Drafts") + "\n")
	b.WriteString(secondary.Render(strings.Repeat("─", previewWidth)) + "\n")

	b.WriteString(accentBold.Render("> ") + dim.Render("Alice  ") + accent.Render("Meeting tomorrow") + "  " + muted.Render("2m ago") + "\n")
	b.WriteString("  " + dim.Render("Bob    ") + secondary.Render("Re: Project update") + "  " + muted.Render("1h ago") + "\n")
	b.WriteString("  " + dim.Render("Carol  ") + secondary.Render("Quick question") + "    " + muted.Render("3h ago") + "\n\n")

	b.WriteString(accentBold.Render("Folders") + "\n")
	b.WriteString(activeFolder.Render(" INBOX ") + "  " + secondary.Render("Sent") + "  " + secondary.Render("Trash") + "\n\n")

	b.WriteString(accentBold.Render("Success: ") + accent.Render("Email sent!") + "\n")
	b.WriteString(danger.Render("Error: ") + danger.Render("Connection failed") + "\n")
	b.WriteString(warn.Render("Update available: v2.0") + "\n")
	b.WriteString(tip.Render("Tip: Press ? for help") + "\n")
	b.WriteString(link.Render("https://example.com") + "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.AccentDark).
		Padding(1, 2).
		Width(previewWidth).
		Render(b.String())

	return box
}
