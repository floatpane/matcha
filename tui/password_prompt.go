package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
)

// PasswordPrompt asks the user for their encryption password to unlock the app.
type PasswordPrompt struct {
	input     textinput.Model
	err       string
	width     int
	height    int
	verifying bool
}

// NewPasswordPrompt creates a new password prompt screen.
func NewPasswordPrompt() *PasswordPrompt {
	ti := textinput.New()
	ti.Placeholder = t("password_prompt.enter_password")
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'
	ti.Prompt = "> "
	ti.CharLimit = 256
	ti.Focus()
	ti.SetStyles(ThemedTextInputStyles())

	return &PasswordPrompt{
		input: ti,
	}
}

func (m *PasswordPrompt) Init() tea.Cmd {
	return textinput.Blink
}

func (m *PasswordPrompt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			password := m.input.Value()
			if password == "" {
				m.err = t("password_prompt.error_empty")
				return m, nil
			}
			m.verifying = true
			return m, verifyPasswordCmd(password)
		case "ctrl+c":
			return m, tea.Quit
		}
		// Clear error on new input
		if m.err != "" {
			m.err = ""
		}

	case PasswordVerifiedMsg:
		if msg.Err != nil {
			m.err = msg.Err.Error()
			m.verifying = false
			m.input.SetValue("")
			return m, nil
		}
		// Password correct — key is in msg.Key
		return m, nil

	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *PasswordPrompt) View() tea.View {
	var b strings.Builder

	b.WriteString(logoStyle.Render(choiceLogo))
	b.WriteString("\n")

	lockTitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFDF5")).
		Background(lipgloss.Color("#25A065")).
		Padding(0, 1).
		Render(t("password_prompt.title"))

	b.WriteString(lockTitle)
	b.WriteString("\n\n")

	if m.verifying {
		b.WriteString("  Verifying password...\n")
	} else {
		b.WriteString("  " + m.input.View() + "\n")
	}

	if m.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		b.WriteString("\n" + errStyle.Render("  "+m.err) + "\n")
	}

	mainContent := b.String()
	helpView := helpStyle.Render(t("password_prompt.help"))

	if m.height > 0 {
		currentHeight := lipgloss.Height(docStyle.Render(mainContent + helpView))
		gap := m.height - currentHeight
		if gap > 0 {
			mainContent += strings.Repeat("\n", gap)
		}
	} else {
		mainContent += "\n\n"
	}

	return tea.NewView(docStyle.Render(mainContent + helpView))
}

// verifyPasswordCmd runs password verification in a goroutine.
func verifyPasswordCmd(password string) tea.Cmd {
	return func() tea.Msg {
		key, err := config.VerifyPassword(password)
		return PasswordVerifiedMsg{Key: key, Err: err}
	}
}
