package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// MailingListEditor displays the screen to add or edit a mailing list.
type MailingListEditor struct {
	nameInput  textinput.Model
	addrInput  textinput.Model
	focus      int // 0 = name, 1 = addresses
	width      int
	height     int
	isEditMode bool
	editIndex  int // index of the mailing list being edited
}

// NewMailingListEditor creates a new mailing list editor model.
func NewMailingListEditor() *MailingListEditor {
	tiStyles := ThemedTextInputStyles()

	name := textinput.New()
	name.Placeholder = "e.g., Team"
	name.SetStyles(tiStyles)
	name.Focus()

	addr := textinput.New()
	addr.Placeholder = "e.g., alice@example.com, bob@example.com"
	addr.SetStyles(tiStyles)

	return &MailingListEditor{
		nameInput: name,
		addrInput: addr,
		focus:     0,
		editIndex: -1,
	}
}

// SetEditMode sets the editor to edit an existing mailing list.
func (m *MailingListEditor) SetEditMode(index int, name, addresses string) {
	m.isEditMode = true
	m.editIndex = index
	m.nameInput.SetValue(name)
	m.addrInput.SetValue(addresses)
}

// Init initializes the mailing list editor model.
func (m *MailingListEditor) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles messages for the mailing list editor model.
func (m *MailingListEditor) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.nameInput.SetWidth(msg.Width - 4)
		m.addrInput.SetWidth(msg.Width - 4)
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m, func() tea.Msg { return GoToSettingsMsg{} }
		case "tab", "shift+tab", "up", "down":
			if m.focus == 0 {
				m.focus = 1
				m.nameInput.Blur()
				m.addrInput.Focus()
			} else {
				m.focus = 0
				m.addrInput.Blur()
				m.nameInput.Focus()
			}
			return m, nil
		case "enter":
			if m.focus == 0 {
				m.focus = 1
				m.nameInput.Blur()
				m.addrInput.Focus()
				return m, nil
			} else {
				// Submit on second field
				name := strings.TrimSpace(m.nameInput.Value())
				addrs := strings.TrimSpace(m.addrInput.Value())
				if name != "" && addrs != "" {
					editIdx := m.editIndex
					return m, func() tea.Msg {
						return SaveMailingListMsg{
							Name:      name,
							Addresses: addrs,
							EditIndex: editIdx,
						}
					}
				}
			}
		}
	}

	if m.focus == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else {
		m.addrInput, cmd = m.addrInput.Update(msg)
	}

	return m, cmd
}

// View renders the mailing list editor screen.
func (m *MailingListEditor) View() tea.View {
	titleText := "Add Mailing List"
	if m.isEditMode {
		titleText = "Edit Mailing List"
	}
	title := titleStyle.Render(titleText)

	var nameView, addrView string
	if m.focus == 0 {
		nameView = focusedStyle.Render("Name:") + "\n" + m.nameInput.View()
		addrView = blurredStyle.Render("Addresses (comma-separated):") + "\n" + m.addrInput.View()
	} else {
		nameView = blurredStyle.Render("Name:") + "\n" + m.nameInput.View()
		addrView = focusedStyle.Render("Addresses (comma-separated):") + "\n" + m.addrInput.View()
	}

	return tea.NewView(lipgloss.JoinVertical(lipgloss.Left,
		title,
		"",
		nameView,
		"",
		addrView,
		"",
		helpStyle.Render("tab/↑/↓: switch fields • enter: submit • esc: back"),
	))
}
