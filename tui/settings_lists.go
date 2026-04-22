package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
)

func (m *Settings) updateMailingLists(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.confirmingDelete {
		switch msg.String() {
		case "y", "Y":
			if m.listsCursor < len(m.cfg.MailingLists) {
				m.cfg.MailingLists = append(m.cfg.MailingLists[:m.listsCursor], m.cfg.MailingLists[m.listsCursor+1:]...)
				_ = config.SaveConfig(m.cfg)
				if m.listsCursor >= len(m.cfg.MailingLists) && m.listsCursor > 0 {
					m.listsCursor--
				}
				m.confirmingDelete = false
			}
		case "n", "N", "esc":
			m.confirmingDelete = false
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "up", "k":
		if m.listsCursor > 0 {
			m.listsCursor--
		}
	case "down", "j":
		if m.listsCursor < len(m.cfg.MailingLists) {
			m.listsCursor++
		}
	case "d":
		if m.listsCursor < len(m.cfg.MailingLists) && len(m.cfg.MailingLists) > 0 {
			m.confirmingDelete = true
		}
	case "e":
		if m.listsCursor < len(m.cfg.MailingLists) {
			list := m.cfg.MailingLists[m.listsCursor]
			idx := m.listsCursor
			return m, func() tea.Msg {
				return GoToEditMailingListMsg{
					Index:     idx,
					Name:      list.Name,
					Addresses: strings.Join(list.Addresses, ", "),
				}
			}
		}
	case "enter":
		if m.listsCursor == len(m.cfg.MailingLists) {
			return m, func() tea.Msg { return GoToAddMailingListMsg{} }
		}
	}
	return m, nil
}

func (m *Settings) viewMailingLists() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render(t("settings_mailing_lists.title")) + "\n\n")

	if len(m.cfg.MailingLists) == 0 {
		b.WriteString(accountEmailStyle.Render("  " + t("settings_mailing_lists.no_lists") + "\n\n"))
	}

	for i, list := range m.cfg.MailingLists {
		addrCount := tn("settings_mailing_lists.address_count", len(list.Addresses), map[string]interface{}{
			"count": len(list.Addresses),
		})
		line := fmt.Sprintf("%s - %s", list.Name, accountEmailStyle.Render(addrCount))

		cursor := "  "
		style := accountItemStyle
		if m.listsCursor == i {
			cursor = "> "
			style = selectedAccountItemStyle
		}
		b.WriteString(style.Render(cursor+line) + "\n")
	}

	cursor := "  "
	style := accountItemStyle
	if m.listsCursor == len(m.cfg.MailingLists) {
		cursor = "> "
		style = selectedAccountItemStyle
	}
	b.WriteString(style.Render(cursor+t("settings_mailing_lists.add_list")) + "\n\n")

	b.WriteString(helpStyle.Render(t("settings_mailing_lists.help")))

	if m.confirmingDelete {
		listName := m.cfg.MailingLists[m.listsCursor].Name
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				dangerStyle.Render(t("settings_mailing_lists.delete_confirm")),
				accountEmailStyle.Render(listName),
				HelpStyle.Render("\n(y/n)"),
			),
		)
		b.WriteString("\n\n" + dialog)
	}

	return b.String()
}
