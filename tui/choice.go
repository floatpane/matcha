package tui

import (
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
	menuItemStyle     = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("245"))
	menuSelectedStyle = lipgloss.NewStyle().PaddingLeft(2).Background(lipgloss.Color("42")).Foreground(lipgloss.Color("#000000")).Bold(true)
	menuSectionStyle  = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("241"))
	menuHintStyle     = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("245"))
	menuNoticeStyle   = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("214"))
)

// menuWidth is the width of the highlight bar behind the selected home entry.
const menuWidth = 36

// ASCII logo for the start screen
const choiceLogo = `
███╗   ███╗ █████╗ ████████╗ ██████╗ ██╗  ██╗ █████╗
████╗ ████║██╔══██╗╚══██╔══╝██╔════╝ ██║  ██║██╔══██╗
██╔████╔██║███████║   ██║   ██║      ███████║███████║
██║╚██╔╝██║██╔══██║   ██║   ██║      ██╔══██║██╔══██║
██║ ╚═╝ ██║██║  ██║   ██║   ╚██████╗ ██║  ██║██║  ██║
╚═╝     ╚═╝╚═╝  ╚═╝   ╚═╝    ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝
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
		// The body starts one row below the top margin of docStyle.
		_, rows := m.menuContent()
		for i, row := range rows {
			if row == msg.Y-1 {
				m.cursor = i
				return m.handleSelect()
			}
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

// menuContent renders the home screen body and returns the row (relative to
// the top of the body) of every menu entry, so mouse clicks map to items.
func (m Choice) menuContent() (string, []int) {
	var lines []string
	lines = append(lines, strings.Split(strings.TrimRight(renderLogo(choiceLogo), "\n"), "\n")...)
	lines = append(lines, "")

	if len(m.keybindWarnings) > 0 {
		warnStyle := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Warning).PaddingLeft(2)
		for _, w := range m.keybindWarnings {
			lines = append(lines, warnStyle.Render("⚠ keybind "+w))
		}
		lines = append(lines, "")
	}

	if m.UpdateAvailable {
		cur := m.CurrentVersion
		if cur == "" {
			cur = t("choice.unknown")
		}
		msg := tpl("choice.update_available", map[string]interface{}{
			"latest":  m.LatestVersion,
			"current": cur,
		})
		lines = append(lines, menuNoticeStyle.Render(msg), "")
	}

	lines = append(lines, menuHintStyle.Render(t("choice.what_to_do")), "")

	rows := make([]int, len(m.choices))
	for i, choice := range m.choices {
		rows[i] = len(lines)
		if i == m.cursor {
			lines = append(lines, menuSelectedStyle.Width(menuWidth).Render(choice))
		} else {
			lines = append(lines, menuItemStyle.Render(choice))
		}
	}
	return strings.Join(lines, "\n"), rows
}

func (m Choice) View() tea.View {
	content, _ := m.menuContent()
	helpView := helpStyle.Render(keybindHelp("choice.help"))

	mainContent := content
	if m.height > 0 {
		currentHeight := lipgloss.Height(docStyle.Render(mainContent + "\n" + helpView))
		if gap := m.height - currentHeight; gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

	v := tea.NewView(docStyle.Render(mainContent + "\n" + helpView))
	if config.MouseEnabled != nil && *config.MouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}
