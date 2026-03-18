package tui

import (
	"fmt"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/theme"
)

// ASCII logo for Matcha displayed during loading screens
const asciiLogo = `
                	__       __
   ____ ___  ____ _/ /______/ /_  ____ _
  / __ '__ \/ __ '/ __/ ___/ __ \/ __ '/
 / / / / / / /_/ / /_/ /__/ / / / /_/ /
/_/ /_/ /_/\__,_/\__/\___/_/ /_/\__,_/
`

var (
	DialogBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#25A065")).
			Padding(1, 2).
			BorderTop(true).
			BorderLeft(true).
			BorderRight(true).
			BorderBottom(true)

	HelpStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	TipStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Italic(true)
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	InfoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)

	H1Style = lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(true).
		Align(lipgloss.Center)

	H2Style = lipgloss.NewStyle().
		Foreground(lipgloss.Color("42")).
		Bold(false). // Less bold
		Align(lipgloss.Center)

	BodyStyle = lipgloss.NewStyle().
			Bold(true) // A bit bold
)

var DocStyle = lipgloss.NewStyle().Margin(1, 2)

// A simple model for showing a status message
type Status struct {
	spinner spinner.Model
	message string
}

func NewStatus(msg string) Status {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent)
	return Status{spinner: s, message: msg}
}

func (m Status) Init() tea.Cmd { return m.spinner.Tick }

func (m Status) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Status) View() tea.View {
	logoStyle := lipgloss.NewStyle().Foreground(theme.ActiveTheme.Accent)
	styledLogo := logoStyle.Render(asciiLogo)

	spinnerLine := fmt.Sprintf("   %s %s", m.spinner.View(), m.message)

	return tea.NewView(fmt.Sprintf("%s\n%s\n\n", styledLogo, spinnerLine))
}
