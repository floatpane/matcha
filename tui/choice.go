package tui

import (
	"fmt"
	"reflect"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/theme"
)

// Styles defined locally to avoid import issues.
var (
	docStyle          = lipgloss.NewStyle().Margin(1, 2)
	titleStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFDF5")).Background(lipgloss.Color("#25A065")).Padding(0, 1)
	listHeader        = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).PaddingBottom(1)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("42"))
)

// ASCII logo for the start screen
const choiceLogo = `
███╗   ███╗ █████╗ ████████╗ ██████╗ ██╗  ██╗ █████╗
████╗ ████║██╔══██╗╚══██╔══╝██╔════╝ ██║  ██║██╔══██╗
██╔████╔██║███████║   ██║   ██║      ███████║███████║
██║╚██╔╝██║██╔══██║   ██║   ██║      ██╔══██║██╔══██║
██║ ╚═╝ ██║██║  ██║   ██║   ╚██████╗ ██║  ██║██║  ██║
╚═╝     ╚═╝╚═╝  ╚═╝   ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝
           ╭──────────────────────╮
           │ v1 Release Candidate │
           ╰──────────────────────╯
`

type Choice struct {
	cursor          int
	choices         []string
	hasSavedDrafts  bool
	UpdateAvailable bool
	LatestVersion   string
	CurrentVersion  string
	width           int
	height          int
	keybindWarnings []string
}

func NewChoice() Choice {
	hasSavedDrafts := config.HasDrafts()
	choices := []string{
		"\ueb1c " + t("choice.inbox"),
		"\ueb1b " + t("choice.compose"),
	}
	if hasSavedDrafts {
		choices = append(choices, "\uec0e "+t("choice.drafts"))
	}
	choices = append(choices, "\uf487 "+t("choice.marketplace"))
	choices = append(choices, "\uf013 "+t("choice.settings"))
	return Choice{
		choices:         choices,
		hasSavedDrafts:  hasSavedDrafts,
		UpdateAvailable: false,
		LatestVersion:   "",
		CurrentVersion:  "",
		keybindWarnings: config.ValidateKeybinds(config.Keybinds),
	}
}

func (m Choice) Init() tea.Cmd {
	return nil
}

func (m Choice) computeMenuStartY() int {
	y := 1 // docStyle top margin (Margin(1, 2))
	y += 6 // choiceLogo: blank line + 5 ascii art lines (trailing \n doesn't add a visible row)
	if len(m.keybindWarnings) > 0 {
		y += len(m.keybindWarnings) + 1 // one row per warning + trailing \n
	}
	// listHeader with PaddingBottom(1) = 2 rows, then \n\n = 2 more rows
	y += 4
	if m.UpdateAvailable {
		y += 3 // update message (1 row) + \n\n (2 rows)
	}
	return y
}

func (m Choice) handleSelect() (tea.Model, tea.Cmd) {
	idx := m.cursor
	marketplaceIdx := 2
	settingsIdx := 3
	if m.hasSavedDrafts {
		marketplaceIdx = 3
		settingsIdx = 4
	}
	switch idx {
	case 0:
		return m, func() tea.Msg { return GoToInboxMsg{} }
	case 1:
		return m, func() tea.Msg { return GoToSendMsg{} }
	case marketplaceIdx - 1:
		if m.hasSavedDrafts {
			return m, func() tea.Msg { return GoToDraftsMsg{} }
		}
		return m, func() tea.Msg { return GoToMarketplaceMsg{} }
	case marketplaceIdx:
		return m, func() tea.Msg { return GoToMarketplaceMsg{} }
	case settingsIdx:
		return m, func() tea.Msg { return GoToSettingsMsg{} }
	}
	return m, nil
}

func (m Choice) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			m.cursor = (m.cursor + 1) % len(m.choices)
		case tea.MouseWheelUp:
			m.cursor = (m.cursor - 1 + len(m.choices)) % len(m.choices)
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return m, nil
		}
		idx := msg.Y - m.computeMenuStartY()
		if idx >= 0 && idx < len(m.choices) {
			m.cursor = idx
			return m.handleSelect()
		}
		return m, nil

	case tea.KeyPressMsg:
		kb := config.Keybinds
		switch msg.String() {
		case "up", kb.Global.NavUp:
			m.cursor = (m.cursor - 1 + len(m.choices)) % len(m.choices)
		case keyDown, kb.Global.NavDown:
			m.cursor = (m.cursor + 1) % len(m.choices)
		case keyEnter:
			return m.handleSelect()
		}
	}

	// Handle update notification from other package without importing its type directly.
	// We look for a struct named 'UpdateAvailableMsg' that contains 'Latest' and 'Current' string fields.
	rv := reflect.ValueOf(msg)
	if rv.IsValid() && rv.Kind() == reflect.Struct && rv.Type().Name() == "UpdateAvailableMsg" {
		f := rv.FieldByName("Latest")
		c := rv.FieldByName("Current")
		updated := false
		if f.IsValid() && f.Kind() == reflect.String {
			m.LatestVersion = f.String()
			updated = true
		}
		if c.IsValid() && c.Kind() == reflect.String {
			m.CurrentVersion = c.String()
			updated = true
		}
		if updated {
			m.UpdateAvailable = true
			return m, nil
		}
	}

	return m, nil
}

func (m Choice) View() tea.View {
	var b strings.Builder

	// renderLogo checks the plugin banner override (set via
	// matcha.ui.set_banner) and falls back to the default choiceLogo.
	b.WriteString(renderLogo(choiceLogo))
	b.WriteString("\n")

	if len(m.keybindWarnings) > 0 {
		warnStyle := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Warning).Padding(0, 1)
		for _, w := range m.keybindWarnings {
			b.WriteString(warnStyle.Render("⚠ keybind " + w))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(listHeader.Render(t("choice.what_to_do")))
	b.WriteString("\n\n")

	// If we detected an update, show a short message under the header.
	if m.UpdateAvailable {
		updateStyle := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Warning).Padding(0, 1)
		cur := m.CurrentVersion
		if cur == "" {
			cur = t("choice.unknown")
		}
		msg := tpl("choice.update_available", map[string]interface{}{
			"latest":  m.LatestVersion,
			"current": cur,
		})
		b.WriteString(updateStyle.Render(msg))
		b.WriteString("\n\n")
	}

	for i, choice := range m.choices {
		if m.cursor == i {
			b.WriteString(selectedItemStyle.Render(fmt.Sprintf("> %s", choice)))
		} else {
			b.WriteString(itemStyle.Render(fmt.Sprintf("  %s", choice)))
		}
		b.WriteString("\n")
	}

	mainContent := b.String()
	helpView := helpStyle.Render(t("choice.help"))

	if m.height > 0 {
		currentHeight := lipgloss.Height(docStyle.Render(mainContent + helpView))
		gap := m.height - currentHeight
		if gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

	v := tea.NewView(docStyle.Render(mainContent + helpView))
	if config.MouseEnabled != nil && *config.MouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}
