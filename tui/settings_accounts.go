package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func (m *Settings) updateAccounts(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.isCryptoConfig {
		var m2 *Settings
		var cmd tea.Cmd
		m2, cmd = m.updateSMIMEConfig(msg)
		return m2, cmd
	}

	if m.confirmingDelete {
		switch msg.String() {
		case "y", "Y":
			if m.accountsCursor < len(m.cfg.Accounts) {
				accountID := m.cfg.Accounts[m.accountsCursor].ID
				m.confirmingDelete = false
				return m, func() tea.Msg {
					return DeleteAccountMsg{AccountID: accountID}
				}
			}
		case "n", "N", "esc":
			m.confirmingDelete = false
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "up", "k":
		if m.accountsCursor > 0 {
			m.accountsCursor--
		}
	case "down", "j":
		if m.accountsCursor < len(m.cfg.Accounts) {
			m.accountsCursor++
		}
	case "d":
		if m.accountsCursor < len(m.cfg.Accounts) && len(m.cfg.Accounts) > 0 {
			m.confirmingDelete = true
		}
	case "e":
		if m.accountsCursor < len(m.cfg.Accounts) {
			acc := m.cfg.Accounts[m.accountsCursor]
			return m, func() tea.Msg {
				return GoToEditAccountMsg{
					AccountID:    acc.ID,
					Provider:     acc.ServiceProvider,
					Name:         acc.Name,
					Email:        acc.Email,
					FetchEmail:   acc.FetchEmail,
					SendAsEmail:  acc.SendAsEmail,
					IMAPServer:   acc.IMAPServer,
					IMAPPort:     acc.IMAPPort,
					SMTPServer:   acc.SMTPServer,
					SMTPPort:     acc.SMTPPort,
					Insecure:     acc.Insecure,
					Protocol:     acc.Protocol,
					JMAPEndpoint: acc.JMAPEndpoint,
					POP3Server:   acc.POP3Server,
					POP3Port:     acc.POP3Port,
				}
			}
		}
	case "c": // Quick shortcut to crypto config
		if m.accountsCursor < len(m.cfg.Accounts) {
			m.enterCryptoConfig()
			return m, textinput.Blink
		}
	case "enter":
		if m.accountsCursor == len(m.cfg.Accounts) {
			return m, func() tea.Msg { return GoToAddAccountMsg{} }
		} else if m.accountsCursor < len(m.cfg.Accounts) {
			m.enterCryptoConfig()
			return m, textinput.Blink
		}
	}
	return m, nil
}

func (m *Settings) enterCryptoConfig() {
	m.isCryptoConfig = true
	m.editingAccountIdx = m.accountsCursor
	acc := m.cfg.Accounts[m.accountsCursor]

	m.smimeCertInput.SetValue(acc.SMIMECert)
	m.smimeKeyInput.SetValue(acc.SMIMEKey)
	m.pgpPublicKeyInput.SetValue(acc.PGPPublicKey)
	m.pgpPrivateKeyInput.SetValue(acc.PGPPrivateKey)
	if acc.PGPKeySource == "" {
		m.pgpKeySource = "file"
	} else {
		m.pgpKeySource = acc.PGPKeySource
	}
	m.pgpPINInput.SetValue(acc.PGPPIN)

	m.cryptoFocusIndex = 0
	m.smimeCertInput.Focus()
	m.smimeKeyInput.Blur()
	m.pgpPublicKeyInput.Blur()
	m.pgpPrivateKeyInput.Blur()
	m.pgpPINInput.Blur()
}

func (m *Settings) viewAccounts() string {
	if m.isCryptoConfig {
		return m.viewSMIMEConfig()
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("Account Settings") + "\n\n")

	if len(m.cfg.Accounts) == 0 {
		b.WriteString(accountEmailStyle.Render("  No accounts configured.\n\n"))
	}

	for i, account := range m.cfg.Accounts {
		displayName := account.Email
		if account.Name != "" {
			displayName = fmt.Sprintf("%s (%s)", account.Name, account.FetchEmail)
		}

		providerInfo := account.ServiceProvider
		if account.ServiceProvider == "custom" {
			providerInfo = fmt.Sprintf("custom: %s", account.IMAPServer)
		}

		if account.SMIMECert != "" && account.SMIMEKey != "" {
			providerInfo += " [S/MIME Configured]"
		}
		if account.PGPPublicKey != "" && account.PGPPrivateKey != "" {
			providerInfo += " [PGP Configured]"
		}

		line := fmt.Sprintf("%s - %s", displayName, accountEmailStyle.Render(providerInfo))

		cursor := "  "
		style := accountItemStyle
		if m.accountsCursor == i {
			cursor = "> "
			style = selectedAccountItemStyle
		}

		b.WriteString(style.Render(cursor+line) + "\n")
	}

	// Add Account option
	cursor := "  "
	style := accountItemStyle
	if m.accountsCursor == len(m.cfg.Accounts) {
		cursor = "> "
		style = selectedAccountItemStyle
	}
	b.WriteString(style.Render(cursor+"Add New Account") + "\n\n")

	b.WriteString(helpStyle.Render("↑/↓: navigate • enter: edit crypto config • e: edit server • d: delete"))

	if m.confirmingDelete {
		accountName := m.cfg.Accounts[m.accountsCursor].Email
		dialog := DialogBoxStyle.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				dangerStyle.Render("Delete account?"),
				accountEmailStyle.Render(accountName),
				HelpStyle.Render("\n(y/n)"),
			),
		)
		// Try to overlay dialog in a reasonable way, since we don't have full screen width access easily here.
		// Just append it.
		b.WriteString("\n\n" + dialog)
	}

	return b.String()
}
