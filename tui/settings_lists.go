package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Settings) updateMailingLists(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.listsConfirmingDelete {
		switch msg.String() {
		case "y", "Y":
			m.listsConfirmingDelete = false
			if m.listsCursor < len(m.cfg.MailingLists) {
				m.cfg.MailingLists = append(m.cfg.MailingLists[:m.listsCursor], m.cfg.MailingLists[m.listsCursor+1:]...)
				if m.listsCursor >= len(m.cfg.MailingLists) && m.listsCursor > 0 {
					m.listsCursor--
				}
				return m, m.saveConfig()
			}
		case "n", "N", "esc":
			m.listsConfirmingDelete = false
		}
		return m, nil
	}

	switch msg.String() {
	case "up", "k":
		itemCount := len(m.cfg.MailingLists) + 1
		m.listsCursor = (m.listsCursor - 1 + itemCount) % itemCount
	case keyDown, "j":
		itemCount := len(m.cfg.MailingLists) + 1
		m.listsCursor = (m.listsCursor + 1) % itemCount
	case "d":
		if m.listsCursor < len(m.cfg.MailingLists) {
			m.listsConfirmingDelete = true
		}
	case "e":
		return m, m.editMailingListCmd()
	case keyEnter:
		if m.listsCursor == len(m.cfg.MailingLists) {
			return m, func() tea.Msg { return GoToAddMailingListMsg{} }
		}
		// "enter: select" on an existing list opens it for editing.
		return m, m.editMailingListCmd()
	}
	return m, nil
}

// editMailingListCmd opens the editor for the list under the cursor (nil on
// the "Add New" row).
func (m *Settings) editMailingListCmd() tea.Cmd {
	if m.listsCursor < 0 || m.listsCursor >= len(m.cfg.MailingLists) {
		return nil
	}
	list := m.cfg.MailingLists[m.listsCursor]
	idx := m.listsCursor
	addresses := strings.Join(list.Addresses, ", ")
	return func() tea.Msg {
		return GoToEditMailingListMsg{
			Index:     idx,
			Name:      list.Name,
			Addresses: addresses,
		}
	}
}

// mailingListsPage lists the mailing lists followed by "Add New Mailing List".
func (m *Settings) mailingListsPage() settingsPage {
	header := titleStyle.Render(t("settings_mailing_lists.title")) + "\n"
	if len(m.cfg.MailingLists) == 0 {
		header += "\n" + accountEmailStyle.Render("  "+t("settings_mailing_lists.no_lists")) + "\n"
	}

	body := make([]string, 0, len(m.cfg.MailingLists)+1)
	for i, list := range m.cfg.MailingLists {
		addrCount := tn("settings_mailing_lists.address_count", len(list.Addresses), map[string]interface{}{
			keyCount: len(list.Addresses),
		})
		line := fmt.Sprintf("%s - %s", list.Name, accountEmailStyle.Render(addrCount))

		selected := m.listsCursor == i
		body = append(body, m.contentItemStyle(selected).Render(m.contentCursor(selected)+line))
	}

	selected := m.listsCursor == len(m.cfg.MailingLists)
	body = append(body, m.contentItemStyle(selected).Render(m.contentCursor(selected)+t("settings_mailing_lists.add_list")))

	return settingsPage{
		key:      "lists",
		header:   header,
		body:     body,
		focusTop: m.listsCursor,
		focusBot: m.listsCursor,
		footer:   "\n" + helpStyle.Render(t("settings_mailing_lists.help")),
		rows:     true,
	}
}
