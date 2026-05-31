package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
)

const cryptoConfigMaxFocus = 9

func (m *Settings) updateSMIMEConfig(msg tea.KeyPressMsg) (*Settings, tea.Cmd) {
	key := msg.Key()
	isEnter := key.Code == tea.KeyEnter || key.Code == tea.KeyReturn || key.Code == tea.KeyKpEnter
	isSpace := key.Code == tea.KeySpace

	switch msg.String() {
	case "esc":
		m.isCryptoConfig = false
		return m, nil
	case "tab", keyShiftTab, "up", keyDown:
		return m, m.cryptoNavigate(msg.String())
	}

	if isEnter {
		return m, m.cryptoHandleEnter()
	}
	if isSpace && m.cryptoToggle() {
		return m, nil
	}

	// Forward any remaining key press (typing, backspace, paste, cursor) to the
	// focused text input.
	return m, m.cryptoForwardInput(msg)
}

// cryptoSetFocus moves focus to index next, blurring every input and focusing
// the one (if any) bound to that row.
func (m *Settings) cryptoSetFocus(next int) tea.Cmd {
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

// cryptoNavigate handles tab/shift-tab/up/down, skipping the YubiKey PIN row
// (index 6) when the key source is not a YubiKey.
func (m *Settings) cryptoNavigate(s string) tea.Cmd {
	back := s == keyShiftTab || s == "up"
	if back {
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
	if m.cryptoFocusIndex == 6 && m.pgpKeySource != keyYubikey {
		if back {
			m.cryptoFocusIndex = 5
		} else {
			m.cryptoFocusIndex = 7
		}
	}
	return m.cryptoSetFocus(m.cryptoFocusIndex)
}

// cryptoHandleEnter saves (row 8), cancels (row 9), or advances focus.
func (m *Settings) cryptoHandleEnter() tea.Cmd {
	switch m.cryptoFocusIndex {
	case 8: // Save
		acct := &m.cfg.Accounts[m.editingAccountIdx]
		acct.SMIMECert = m.smimeCertInput.Value()
		acct.SMIMEKey = m.smimeKeyInput.Value()
		acct.PGPPublicKey = m.pgpPublicKeyInput.Value()
		acct.PGPPrivateKey = m.pgpPrivateKeyInput.Value()
		acct.PGPKeySource = m.pgpKeySource
		acct.PGPPIN = m.pgpPINInput.Value()
		_ = config.SaveConfig(m.cfg)
		m.isCryptoConfig = false
		return nil
	case 9: // Cancel
		m.isCryptoConfig = false
		return nil
	default:
		next := m.cryptoFocusIndex + 1
		if next == 6 && m.pgpKeySource != keyYubikey {
			next = 7
		}
		return m.cryptoSetFocus(next)
	}
}

// cryptoToggle flips the boolean/choice rows (Sign By Default, Key Source).
// It reports whether the current row was a toggle.
func (m *Settings) cryptoToggle() bool {
	acct := &m.cfg.Accounts[m.editingAccountIdx]
	switch m.cryptoFocusIndex {
	case 2:
		acct.SMIMESignByDefault = !acct.SMIMESignByDefault
	case 5:
		if m.pgpKeySource == "file" {
			m.pgpKeySource = keyYubikey
		} else {
			m.pgpKeySource = "file"
		}
	case 7:
		acct.PGPSignByDefault = !acct.PGPSignByDefault
	default:
		return false
	}
	return true
}

// cryptoForwardInput routes a key press to the focused text input. The
// toggle/button rows (2, 5, 7, 8, 9) have no input and are no-ops.
func (m *Settings) cryptoForwardInput(msg tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	switch m.cryptoFocusIndex {
	case 0:
		m.smimeCertInput, cmd = m.smimeCertInput.Update(msg)
	case 1:
		m.smimeKeyInput, cmd = m.smimeKeyInput.Update(msg)
	case 3:
		m.pgpPublicKeyInput, cmd = m.pgpPublicKeyInput.Update(msg)
	case 4:
		m.pgpPrivateKeyInput, cmd = m.pgpPrivateKeyInput.Update(msg)
	case 6:
		m.pgpPINInput, cmd = m.pgpPINInput.Update(msg)
	}
	return cmd
}

func (m *Settings) viewSMIMEConfig() string {
	var b strings.Builder
	account := m.cfg.Accounts[m.editingAccountIdx]
	b.WriteString(titleStyle.Render(fmt.Sprintf("Crypto Config: %s", account.FetchEmail)) + "\n\n")

	renderField := func(index int, label, content string) {
		if m.cryptoFocusIndex == index {
			b.WriteString(m.contentFocusStyle().Render(label) + "\n")
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
	if m.pgpKeySource == keyYubikey {
		keySource = "YubiKey"
	}
	renderField(5, "Key Source:", keySource)

	if m.pgpKeySource == keyYubikey {
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
		saveBtn = m.contentFocusStyle().Render(saveBtn)
	} else {
		saveBtn = settingsBlurredStyle.Render(saveBtn)
	}
	if m.cryptoFocusIndex == 9 {
		cancelBtn = m.contentFocusStyle().Render(cancelBtn)
	} else {
		cancelBtn = settingsBlurredStyle.Render(cancelBtn)
	}

	b.WriteString(saveBtn + "  " + cancelBtn + "\n\n")
	b.WriteString(helpStyle.Render("tab: next • enter: next/save • space: toggle • esc: cancel"))

	return b.String()
}
