package tui

import (
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
)

// SignatureEditor displays the signature editing screen.
type SignatureEditor struct {
	textarea textarea.Model
	width    int
	height   int
}

// NewSignatureEditor creates a new signature editor model.
func NewSignatureEditor() *SignatureEditor {
	ta := textarea.New()
	ta.Placeholder = "Enter your email signature...\n\nExample:\nBest regards,\nDrew"
	ta.SetHeight(10)
	ta.SetStyles(ThemedTextAreaStyles())
	ta.Focus()

	// Load existing signature
	if sig, err := config.LoadSignature(); err == nil && sig != "" {
		ta.SetValue(sig)
	}

	return &SignatureEditor{
		textarea: ta,
	}
}

// Init initializes the signature editor model.
func (m *SignatureEditor) Init() tea.Cmd {
	return textarea.Blink
}

// Update handles messages for the signature editor model.
func (m *SignatureEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(msg.Width - 4)
		m.textarea.SetHeight(msg.Height - 10)
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			// Save and go back to settings
			signature := m.textarea.Value()
			go config.SaveSignature(signature)
			return m, func() tea.Msg { return GoToSettingsMsg{} }
		}
	}

	m.textarea, cmd = m.textarea.Update(msg)
	return m, cmd
}

// View renders the signature editor screen.
func (m *SignatureEditor) View() tea.View {
	title := titleStyle.Render("Email Signature")
	hint := accountEmailStyle.Render("This signature will be appended to your emails.")

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left,
		title,
		hint,
		"",
		m.textarea.View(),
		"",
		helpStyle.Render("esc: save & back"),
	))
}
