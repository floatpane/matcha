package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/theme"
)

const (
	// themePreviewMaxWidth caps the preview box width.
	themePreviewMaxWidth = 60
	// themePreviewMinWidth is the narrowest pane that still gets a preview.
	themePreviewMinWidth = 24
)

func (m *Settings) updateTheme(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	themes := theme.AllThemes()

	switch msg.String() {
	case "up", "k":
		if len(themes) > 0 {
			m.themeCursor = (m.themeCursor - 1 + len(themes)) % len(themes)
		}
	case keyDown, "j":
		if len(themes) > 0 {
			m.themeCursor = (m.themeCursor + 1) % len(themes)
		}
	case keyEnter, "space", keyRight, "l":
		if m.themeCursor < len(themes) {
			selected := themes[m.themeCursor]
			theme.SetTheme(selected.Name)
			RebuildStyles()
			m.cfg.Theme = selected.Name
			return m, m.saveConfig()
		}
	}
	return m, nil
}

// themePage lists the themes, previewing the highlighted one when there is
// room for it.
func (m *Settings) themePage() settingsPage {
	themes := theme.AllThemes()

	body := make([]string, len(themes))
	for i, thm := range themes {
		label := thm.Name
		if thm.Name == theme.ActiveTheme.Name {
			label += " (" + t("settings_theme.current") + ")"
		}
		selected := m.themeCursor == i
		body[i] = m.contentItemStyle(selected).Render(m.contentCursor(selected) + label)
	}

	page := settingsPage{
		key:      "theme",
		header:   titleStyle.Render(t("settings_theme.title")) + "\n",
		body:     body,
		focusTop: m.themeCursor,
		focusBot: m.themeCursor,
		rows:     true,
	}

	previewTheme := theme.ActiveTheme
	if m.themeCursor >= 0 && m.themeCursor < len(themes) {
		previewTheme = themes[m.themeCursor]
	}
	previewWidth := themePreviewMaxWidth
	if w := m.contentWidth(); w > 0 {
		previewWidth = min(w, themePreviewMaxWidth)
	}
	if previewWidth >= themePreviewMinWidth {
		page.extra = "\n" + renderThemePreview(previewTheme, previewWidth)
	}

	var footer strings.Builder
	if !m.cfg.HideTips {
		footer.WriteString("\n" + TipStyle.Render("Tip: Custom themes can be added as JSON files in ~/.config/matcha/themes/") + "\n")
	}
	footer.WriteString("\n" + helpStyle.Render(t("settings_theme.help")))
	page.footer = footer.String()
	return page
}

// renderThemePreview renders a small mockup showing how a theme looks. The
// box is previewWidth cells wide in total (border and padding included).
func renderThemePreview(t theme.Theme, previewWidth int) string {
	previewWidth = min(previewWidth, themePreviewMaxWidth)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.AccentDark).
		Padding(1, 2).
		Width(previewWidth)
	inner := max(1, previewWidth-box.GetHorizontalFrameSize())

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
	b.WriteString(secondary.Render(strings.Repeat("─", inner)) + "\n")

	b.WriteString(accentBold.Render("▌ ") + dim.Render("Alice  ") + accent.Render("Meeting tomorrow") + "  " + muted.Render("2m ago") + "\n")
	b.WriteString("  " + dim.Render("Bob    ") + secondary.Render("Re: Project update") + "  " + muted.Render("1h ago") + "\n")
	b.WriteString("  " + dim.Render("Carol  ") + secondary.Render("Quick question") + "    " + muted.Render("3h ago") + "\n\n")

	b.WriteString(accentBold.Render("Folders") + "\n")
	b.WriteString(activeFolder.Render(" INBOX ") + "  " + secondary.Render("Sent") + "  " + secondary.Render("Trash") + "\n\n")

	b.WriteString(accentBold.Render("Success: ") + accent.Render("Email sent!") + "\n")
	b.WriteString(danger.Render("Error: ") + danger.Render("Connection failed") + "\n")
	b.WriteString(warn.Render("Update available: v2.0") + "\n")
	b.WriteString(tip.Render("Tip: Press ? for help") + "\n")
	b.WriteString(link.Render("https://example.com"))

	// Truncate instead of wrapping so the mockup keeps its shape (and height)
	// in narrow panes.
	return box.Render(truncateLines(b.String(), inner))
}
