package tui

import (
	"fmt"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
)

func (m *Settings) updateAccounts(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.isCryptoConfig {
		var m2 *Settings
		var cmd tea.Cmd
		m2, cmd = m.updateSMIMEConfig(msg)
		return m2, cmd
	}

	if m.accountsConfirmingDelete {
		switch msg.String() {
		case "y", "Y":
			m.accountsConfirmingDelete = false
			if m.accountsCursor < len(m.cfg.Accounts) {
				accountID := m.cfg.Accounts[m.accountsCursor].ID
				return m, func() tea.Msg {
					return DeleteAccountMsg{AccountID: accountID}
				}
			}
		case "n", "N", "esc":
			m.accountsConfirmingDelete = false
		}
		return m, nil
	}

	switch msg.String() {
	case "up", "k":
		itemCount := len(m.cfg.Accounts) + 1
		m.accountsCursor = (m.accountsCursor - 1 + itemCount) % itemCount
	case keyDown, "j":
		itemCount := len(m.cfg.Accounts) + 1
		m.accountsCursor = (m.accountsCursor + 1) % itemCount
	case "d":
		if m.accountsCursor < len(m.cfg.Accounts) {
			m.accountsConfirmingDelete = true
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
					CatchAll:     acc.CatchAll,
					IMAPServer:   acc.IMAPServer,
					IMAPPort:     acc.IMAPPort,
					SMTPServer:   acc.SMTPServer,
					SMTPPort:     acc.SMTPPort,
					Insecure:     acc.Insecure,
					Protocol:     acc.Protocol,
					JMAPEndpoint: acc.JMAPEndpoint,
					POP3Server:   acc.POP3Server,
					POP3Port:     acc.POP3Port,
					MaildirPath:  acc.MaildirPath,
					AuthMethod:   acc.AuthMethod,
				}
			}
		}
	case "s": // Edit account signature
		if m.accountsCursor < len(m.cfg.Accounts) {
			// Capture the ID now: the command runs on another goroutine.
			accountID := m.cfg.Accounts[m.accountsCursor].ID
			return m, func() tea.Msg { return GoToSignatureEditorMsg{AccountID: accountID} }
		}
	case "c": // Quick shortcut to crypto config
		if m.accountsCursor < len(m.cfg.Accounts) {
			m.enterCryptoConfig()
			return m, textinput.Blink
		}
	case keyEnter:
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

// accountsPage lists the configured accounts followed by "Add New Account".
func (m *Settings) accountsPage() settingsPage {
	if m.isCryptoConfig {
		return m.cryptoPage()
	}

	header := titleStyle.Render(t("settings_accounts.title")) + "\n"
	if len(m.cfg.Accounts) == 0 {
		header += "\n" + accountEmailStyle.Render("  "+t("settings_accounts.no_accounts")) + "\n"
	}

	body := make([]string, 0, len(m.cfg.Accounts)+1)
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
		if config.HasAccountSignature(&account) {
			providerInfo += " [Signature]"
		}
		if account.CatchAll {
			providerInfo += " [Catch-All]"
		}

		line := fmt.Sprintf("%s - %s", displayName, accountEmailStyle.Render(providerInfo))

		selected := m.accountsCursor == i
		body = append(body, m.contentItemStyle(selected).Render(m.contentCursor(selected)+line))
	}

	// Add Account option
	selected := m.accountsCursor == len(m.cfg.Accounts)
	body = append(body, m.contentItemStyle(selected).Render(m.contentCursor(selected)+t("settings_accounts.add_account")))

	return settingsPage{
		key:      "accounts",
		header:   header,
		body:     body,
		focusTop: m.accountsCursor,
		focusBot: m.accountsCursor,
		footer:   "\n" + helpStyle.Render(t("settings_accounts.help")),
		rows:     true,
	}
}
