package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
)

const cryptoConfigMaxFocus = 9

func (m *Settings) updateSMIMEConfig(msg tea.KeyPressMsg) (*Settings, tea.Cmd) {
	var cmds []tea.Cmd

	key := msg.Key()
	isEnter := key.Code == tea.KeyEnter || key.Code == tea.KeyReturn || key.Code == tea.KeyKpEnter
	isSpace := key.Code == tea.KeySpace

	setFocus := func(next int) tea.Cmd {
		m.cryptoFocusIndex = next
		m.smimeCertInput.Blur()
		m.smimeKeyInput.Blur()
		m.pgpPublicKeyInput.Blur()
		m.pgpPrivateKeyInput.Blur()
		m.pgpPINInput.Blur()

		switch m.cryptoFocusIndex {
		case 0:
			return m.smimeCertInput.Focus()
		case 1:
			return m.smimeKeyInput.Focus()
		case 3:
			return m.pgpPublicKeyInput.Focus()
		case 4:
			return m.pgpPrivateKeyInput.Focus()
		case 6:
			return m.pgpPINInput.Focus()
		}
		return nil
	}

	switch msg.String() {
	case "esc":
		m.isCryptoConfig = false
		return m, nil
	case "tab", "shift+tab", "up", "down":
		if msg.String() == "shift+tab" || msg.String() == "up" {
			m.cryptoFocusIndex--
			if m.cryptoFocusIndex < 0 {
				m.cryptoFocusIndex = cryptoConfigMaxFocus
			}
		} else {
			m.cryptoFocusIndex++
			if m.cryptoFocusIndex > cryptoConfigMaxFocus {
				m.cryptoFocusIndex = 0
			}
		}
		if m.cryptoFocusIndex == 6 && m.pgpKeySource != "yubikey" {
			if msg.String() == "shift+tab" || msg.String() == "up" {
				m.cryptoFocusIndex = 5
			} else {
				m.cryptoFocusIndex = 7
			}
		}
		cmds = append(cmds, setFocus(m.cryptoFocusIndex))
		return m, tea.Batch(cmds...)
	}

	if isEnter {
		switch m.cryptoFocusIndex {
		case 8: // Save
			m.cfg.Accounts[m.editingAccountIdx].SMIMECert = m.smimeCertInput.Value()
			m.cfg.Accounts[m.editingAccountIdx].SMIMEKey = m.smimeKeyInput.Value()
			m.cfg.Accounts[m.editingAccountIdx].PGPPublicKey = m.pgpPublicKeyInput.Value()
			m.cfg.Accounts[m.editingAccountIdx].PGPPrivateKey = m.pgpPrivateKeyInput.Value()
			m.cfg.Accounts[m.editingAccountIdx].PGPKeySource = m.pgpKeySource
			m.cfg.Accounts[m.editingAccountIdx].PGPPIN = m.pgpPINInput.Value()
			_ = config.SaveConfig(m.cfg)
			m.isCryptoConfig = false
			return m, nil
		case 9: // Cancel
			m.isCryptoConfig = false
			return m, nil
		default:
			// advance to next
			next := m.cryptoFocusIndex + 1
			if next == 6 && m.pgpKeySource != "yubikey" {
				next = 7
			}
			cmds = append(cmds, setFocus(next))
			return m, tea.Batch(cmds...)
		}
	}

	if isSpace {
		switch m.cryptoFocusIndex {
		case 2:
			m.cfg.Accounts[m.editingAccountIdx].SMIMESignByDefault = !m.cfg.Accounts[m.editingAccountIdx].SMIMESignByDefault
			return m, nil
		case 5:
			if m.pgpKeySource == "file" {
				m.pgpKeySource = "yubikey"
			} else {
				m.pgpKeySource = "file"
			}
			return m, nil
		case 7:
			m.cfg.Accounts[m.editingAccountIdx].PGPSignByDefault = !m.cfg.Accounts[m.editingAccountIdx].PGPSignByDefault
			return m, nil
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Settings) viewSMIMEConfig() string {
	var b strings.Builder
	account := m.cfg.Accounts[m.editingAccountIdx]
	b.WriteString(titleStyle.Render(fmt.Sprintf("Crypto Config: %s", account.FetchEmail)) + "\n\n")

	renderField := func(index int, label, content string) {
		if m.cryptoFocusIndex == index {
			b.WriteString(settingsFocusedStyle.Render(label) + "\n")
		} else {
			b.WriteString(settingsBlurredStyle.Render(label) + "\n")
		}
		b.WriteString(content + "\n\n")
	}

	// S/MIME
	b.WriteString(settingsFocusedStyle.Render("S/MIME") + "\n")
	renderField(0, "Certificate (PEM) Path:", m.smimeCertInput.View())
	renderField(1, "Private Key (PEM) Path:", m.smimeKeyInput.View())
	smimeSign := "OFF"
	if account.SMIMESignByDefault {
		smimeSign = "ON"
	}
	renderField(2, "Sign By Default:", smimeSign)

	// PGP
	b.WriteString(settingsFocusedStyle.Render("PGP") + "\n")
	renderField(3, "Public Key Path:", m.pgpPublicKeyInput.View())
	renderField(4, "Private Key Path:", m.pgpPrivateKeyInput.View())

	keySource := "File"
	if m.pgpKeySource == "yubikey" {
		keySource = "YubiKey"
	}
	renderField(5, "Key Source:", keySource)

	if m.pgpKeySource == "yubikey" {
		renderField(6, "YubiKey PIN:", m.pgpPINInput.View())
	}

	pgpSign := "OFF"
	if account.PGPSignByDefault {
		pgpSign = "ON"
	}
	renderField(7, "Sign By Default:", pgpSign)

	saveBtn := "[ Save ]"
	cancelBtn := "[ Cancel ]"
	if m.cryptoFocusIndex == 8 {
		saveBtn = settingsFocusedStyle.Render(saveBtn)
	} else {
		saveBtn = settingsBlurredStyle.Render(saveBtn)
	}
	if m.cryptoFocusIndex == 9 {
		cancelBtn = settingsFocusedStyle.Render(cancelBtn)
	} else {
		cancelBtn = settingsBlurredStyle.Render(cancelBtn)
	}

	b.WriteString(saveBtn + "  " + cancelBtn + "\n\n")
	b.WriteString(helpStyle.Render("tab: next • enter: next/save • space: toggle • esc: cancel"))

	return b.String()
}
