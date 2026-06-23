package tui

import (
	"fmt"
	"reflect"
	"regexp"
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
	logoStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	listHeader        = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).PaddingBottom(1)
	itemStyle         = lipgloss.NewStyle().PaddingLeft(2)
	selectedItemStyle = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("42"))
)

// ASCII logo for the start screen
const choiceLogo = `
                    __       __
   ____ ___  ____ _/ /______/ /_  ____ _
  / __ '__ \/ __ '/ __/ ___/ __ \/ __ '/
 / / / / / / /_/ / /_/ /__/ / / / /_/ /
/_/ /_/ /_/\__,_/\__/\___/_/ /_/\__,_/
`

type Choice struct {
	cursor          int
	choices         []string
	hasSavedDrafts  bool
	UpdateAvailable bool
	LatestVersion   string
	CurrentVersion  string
	V1RCAvailable   bool
	V1RCVersion     string
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
		V1RCAvailable:   false,
		V1RCVersion:     "",
		keybindWarnings: config.ValidateKeybinds(config.Keybinds),
	}
}

func (m Choice) Init() tea.Cmd {
	return nil
}

func (m Choice) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyPressMsg:
		kb := config.Keybinds
		switch msg.String() {
		case "up", kb.Global.NavUp:
			m.cursor = (m.cursor - 1 + len(m.choices)) % len(m.choices)
		case keyDown, kb.Global.NavDown:
			m.cursor = (m.cursor + 1) % len(m.choices)
		case keyEnter:
			// Use cursor index instead of string comparison
			idx := m.cursor
			if idx == 0 { //nolint:gocritic
				// Inbox
				return m, func() tea.Msg { return GoToInboxMsg{} }
			} else if idx == 1 {
				// Compose
				return m, func() tea.Msg { return GoToSendMsg{} }
			} else if m.hasSavedDrafts && idx == 2 {
				// Drafts
				return m, func() tea.Msg { return GoToDraftsMsg{} }
			} else if (m.hasSavedDrafts && idx == 3) || (!m.hasSavedDrafts && idx == 2) {
				// Marketplace
				return m, func() tea.Msg { return GoToMarketplaceMsg{} }
			} else if (m.hasSavedDrafts && idx == 4) || (!m.hasSavedDrafts && idx == 3) {
				// Settings
				return m, func() tea.Msg { return GoToSettingsMsg{} }
			}
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

	// Handle v1 release-candidate notification from other package without importing its type directly.
	// We look for a struct named 'V1RCAvailableMsg' that contains 'Latest' and 'Current' string fields.
	rv = reflect.ValueOf(msg)
	if rv.IsValid() && rv.Kind() == reflect.Struct && rv.Type().Name() == "V1RCAvailableMsg" {
		f := rv.FieldByName("Latest")
		if f.IsValid() && f.Kind() == reflect.String && v1RCRegex.MatchString(f.String()) {
			m.V1RCVersion = f.String()
			m.V1RCAvailable = true
			if c := rv.FieldByName("Current"); c.IsValid() && c.Kind() == reflect.String {
				m.CurrentVersion = c.String()
			}
			return m, nil
		}
	}

	return m, nil
}

var (
	v0Regex   = regexp.MustCompile(`^v?0\.\d+\.\d+$`)
	v1RCRegex = regexp.MustCompile(`^v?1\.0\.0-rc\d+$`)
)

func (m Choice) isV0() bool {
	return v0Regex.MatchString(m.CurrentVersion)
}

func (m Choice) isV1RCAvailable() bool {
	return m.V1RCAvailable && m.isV0() && v1RCRegex.MatchString(m.V1RCVersion)
}

func (m Choice) View() tea.View {
	var b strings.Builder

	b.WriteString(logoStyle.Render(choiceLogo))
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

	content := mainContent + helpView
	if m.isV1RCAvailable() {
		noteStyle := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Warning).Padding(0, 1)
		content += "\n" + noteStyle.Render(t("choice.upgrade_v1_note"))
	}

	return tea.NewView(docStyle.Render(content))
}
