package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
)

func (m *Settings) updateContacts(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.contactsConfirming {
		switch msg.String() {
		case "y", "Y":
			m.contactsConfirming = false
			if m.contactsCursor < len(m.contactsList) {
				var cmd tea.Cmd
				if err := config.DeleteContact(m.contactsList[m.contactsCursor].Email); err != nil {
					cmd = settingsErrorCmd("Could not delete contact", err)
				}
				m.contactsList = config.GetNamedContacts()
				if m.contactsCursor >= len(m.contactsList) && m.contactsCursor > 0 {
					m.contactsCursor--
				}
				return m, cmd
			}
		case "n", "N", "esc":
			m.contactsConfirming = false
		}
		return m, nil
	}

	itemCount := len(m.contactsList) + 1 // +1 for "Add New Contact"

	switch msg.String() {
	case "up", "k":
		m.contactsCursor = (m.contactsCursor - 1 + itemCount) % itemCount
	case keyDown, "j":
		m.contactsCursor = (m.contactsCursor + 1) % itemCount
	case "d":
		if m.contactsCursor < len(m.contactsList) {
			m.contactsConfirming = true
		}
	case "e":
		if m.contactsCursor < len(m.contactsList) {
			c := m.contactsList[m.contactsCursor]
			return m, func() tea.Msg {
				return GoToEditContactMsg{
					OriginalEmail: c.Email,
					Name:          c.Name,
					Email:         c.Email,
				}
			}
		}
	case keyEnter:
		if m.contactsCursor == len(m.contactsList) {
			return m, func() tea.Msg { return GoToAddContactMsg{} }
		}
		if m.contactsCursor < len(m.contactsList) {
			c := m.contactsList[m.contactsCursor]
			return m, func() tea.Msg {
				return GoToEditContactMsg{
					OriginalEmail: c.Email,
					Name:          c.Name,
					Email:         c.Email,
				}
			}
		}
	}
	return m, nil
}

// contactsPage lists the named contacts followed by "Add New Contact".
func (m *Settings) contactsPage() settingsPage {
	header := titleStyle.Render(t("settings_contacts.title")) + "\n"
	if len(m.contactsList) == 0 {
		header += "\n" + accountEmailStyle.Render("  "+t("settings_contacts.no_contacts")) + "\n"
	}

	body := make([]string, 0, len(m.contactsList)+1)
	for i, c := range m.contactsList {
		line := fmt.Sprintf("%s  %s", c.Name, accountEmailStyle.Render("<"+c.Email+">"))
		selected := m.contactsCursor == i
		body = append(body, m.contentItemStyle(selected).Render(m.contentCursor(selected)+line))
	}

	selected := m.contactsCursor == len(m.contactsList)
	body = append(body, m.contentItemStyle(selected).Render(m.contentCursor(selected)+t("settings_contacts.add_contact")))

	return settingsPage{
		key:      "contacts",
		header:   header,
		body:     body,
		focusTop: m.contactsCursor,
		focusBot: m.contactsCursor,
		footer:   "\n" + helpStyle.Render(t("settings_contacts.help")),
		rows:     true,
	}
}
