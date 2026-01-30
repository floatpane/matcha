package tui

import (
	"fmt"
	"reflect"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/floatpane/matcha/config"
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
}

func NewChoice() Choice {
	hasSavedDrafts := config.HasDrafts()
	choices := []string{"\ueb1c Inbox", "\ueb1b Compose Email", "\uf1d8 Sent"}
	if hasSavedDrafts {
		choices = append(choices, "\uec0e Drafts")
	}
	choices = append(choices, "\uf013 Settings", "\uea81 Trash & Archive")
	return Choice{
		choices:         choices,
		hasSavedDrafts:  hasSavedDrafts,
		UpdateAvailable: false,
		LatestVersion:   "",
		CurrentVersion:  "",
	}

}

func (m Choice) Init() tea.Cmd {
	return nil
}

func (m Choice) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			selectedChoice := m.choices[m.cursor]
			switch selectedChoice {
			case "\ueb1c Inbox":
				return m, func() tea.Msg { return GoToInboxMsg{} }
			case "\ueb1b Compose Email":
				return m, func() tea.Msg { return GoToSendMsg{} }
			case "\uf1d8 Sent":
				return m, func() tea.Msg { return GoToSentInboxMsg{} }
			case "\uec0e Drafts":
				return m, func() tea.Msg { return GoToDraftsMsg{} }
			case "\uf013 Settings":
				return m, func() tea.Msg { return GoToSettingsMsg{} }
			case "\uea81 Trash & Archive":
				return m, func() tea.Msg { return GoToTrashArchiveMsg{} }
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

	return m, nil
}

func (m Choice) View() string {
	var b strings.Builder

	b.WriteString(logoStyle.Render(choiceLogo))
	b.WriteString("\n")
	b.WriteString(listHeader.Render("What would you like to do?"))
	b.WriteString("\n\n")

	// If we detected an update, show a short message under the header.
	if m.UpdateAvailable {
		updateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Padding(0, 1)
		cur := m.CurrentVersion
		if cur == "" {
			cur = "unknown"
		}
		msg := fmt.Sprintf("Update available: %s (installed: %s) — run `matcha update` to upgrade", m.LatestVersion, cur)
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

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("Use ↑/↓ to navigate, enter to select, and ctrl+c to quit."))

	return docStyle.Render(b.String())
}
