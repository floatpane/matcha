package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
)

// ContactEditor is the add/edit contact screen.
type ContactEditor struct {
	nameInput     textinput.Model
	emailInput    textinput.Model
	focus         int // 0 = name, 1 = email
	width         int
	height        int
	isEditMode    bool
	originalEmail string

	suggestions        []config.Contact
	selectedSuggestion int
	showSuggestions    bool
	lastEmailValue     string
}

// NewContactEditor creates a new contact editor model.
func NewContactEditor() *ContactEditor {
	tiStyles := ThemedTextInputStyles()

	name := textinput.New()
	name.Placeholder = "e.g., Jane Smith"
	name.SetStyles(tiStyles)
	name.Focus()

	email := textinput.New()
	email.Placeholder = "e.g., jane@example.com"
	email.SetStyles(tiStyles)

	return &ContactEditor{
		nameInput:  name,
		emailInput: email,
		focus:      0,
	}
}

// SetEditMode pre-populates the editor with an existing contact's data.
func (m *ContactEditor) SetEditMode(originalEmail, name, email string) {
	m.isEditMode = true
	m.originalEmail = originalEmail
	m.nameInput.SetValue(name)
	m.emailInput.SetValue(email)
}

// Init initializes the contact editor model.
func (m *ContactEditor) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages for the contact editor model.
func (m *ContactEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.nameInput.SetWidth(msg.Width - 4)
		m.emailInput.SetWidth(msg.Width - 4)
		return m, nil

	case tea.KeyPressMsg:
		kb := config.Keybinds

		// Dismiss suggestions on esc; navigate to settings if no suggestions.
		if msg.String() == kb.Global.Cancel {
			if m.showSuggestions {
				m.showSuggestions = false
				m.suggestions = nil
				return m, nil
			}
			return m, func() tea.Msg { return GoToSettingsMsg{} }
		}

		if msg.String() == kb.Global.Quit {
			return m, tea.Quit
		}

		// Navigate suggestions with arrow keys when visible.
		if m.showSuggestions && m.focus == 1 {
			switch msg.String() {
			case "up", "k":
				if m.selectedSuggestion > 0 {
					m.selectedSuggestion--
				}
				return m, nil
			case keyDown, "j":
				if m.selectedSuggestion < len(m.suggestions)-1 {
					m.selectedSuggestion++
				}
				return m, nil
			case keyEnter, "tab":
				if len(m.suggestions) > 0 {
					m.emailInput.SetValue(m.suggestions[m.selectedSuggestion].Email)
					m.showSuggestions = false
					m.suggestions = nil
					m.lastEmailValue = m.emailInput.Value()
					return m, nil
				}
			}
		}

		switch msg.String() {
		case "tab", keyShiftTab, "up":
			if m.focus == 0 {
				m.focus = 1
				m.nameInput.Blur()
				m.emailInput.Focus()
			} else {
				m.focus = 0
				m.emailInput.Blur()
				m.nameInput.Focus()
				m.showSuggestions = false
				m.suggestions = nil
			}
			return m, nil

		case keyEnter:
			if m.focus == 0 {
				m.focus = 1
				m.nameInput.Blur()
				m.emailInput.Focus()
				return m, nil
			}
			// Submit from email field
			name := strings.TrimSpace(m.nameInput.Value())
			email := strings.TrimSpace(m.emailInput.Value())
			if name != "" && email != "" {
				isEdit := m.isEditMode
				orig := m.originalEmail
				return m, func() tea.Msg {
					return SaveContactMsg{
						Name:          name,
						Email:         email,
						OriginalEmail: orig,
						IsEdit:        isEdit,
					}
				}
			}
		}
	}

	// Update the focused input.
	if m.focus == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else {
		m.emailInput, cmd = m.emailInput.Update(msg)
		// Refresh unnamed-contact suggestions based on current email value.
		if current := m.emailInput.Value(); current != m.lastEmailValue {
			m.lastEmailValue = current
			query := strings.TrimSpace(current)
			if len(query) >= 2 {
				m.suggestions = config.SearchUnnamedContacts(query)
				m.showSuggestions = len(m.suggestions) > 0
				m.selectedSuggestion = 0
			} else {
				m.showSuggestions = false
				m.suggestions = nil
			}
		}
	}

	return m, cmd
}

// View renders the contact editor screen.
func (m *ContactEditor) View() tea.View {
	titleText := "Add Contact"
	if m.isEditMode {
		titleText = "Edit Contact"
	}
	title := titleStyle.Render(titleText)

	var nameView, emailView string
	if m.focus == 0 {
		nameView = focusedStyle.Render("Name:") + "\n" + m.nameInput.View()
		emailView = blurredStyle.Render("Email:") + "\n" + m.emailInput.View()
	} else {
		nameView = blurredStyle.Render("Name:") + "\n" + m.nameInput.View()
		emailView = focusedStyle.Render("Email:") + "\n" + m.emailInput.View()
	}

	// Append suggestion box below email field when visible.
	if m.showSuggestions && len(m.suggestions) > 0 {
		var sb strings.Builder
		for i, s := range m.suggestions {
			display := s.Email
			if i == m.selectedSuggestion {
				sb.WriteString(selectedSuggestionStyle.Render("> "+display) + "\n")
			} else {
				sb.WriteString(suggestionStyle.Render("  "+display) + "\n")
			}
		}
		emailView = emailView + "\n" + suggestionBoxStyle.Render(strings.TrimSuffix(sb.String(), "\n"))
	}

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		nameView,
		"",
		emailView,
		"",
		helpStyle.Render("tab/↑/↓: switch fields • enter: submit • "+cancelKey()+": back"),
	))
}
