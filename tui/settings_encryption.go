package tui

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/internal/passwordstrength"
)

func (m *Settings) updateEncryption(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	isEnabled := config.IsSecureModeEnabled()

	if isEnabled {
		if m.confirmingDisable {
			switch msg.String() {
			case "y", "Y":
				m.confirmingDisable = false
				m.encDisabling = true
				m.encError = ""
				cfg := m.cfg
				return m, func() tea.Msg {
					err := config.DisableSecureMode(cfg)
					return SecureModeDisabledMsg{Err: err}
				}
			case "n", "N", "esc":
				m.confirmingDisable = false
				return m, nil
			}
			return m, nil
		}
		// esc/left are handled by the settings view (back to the menu).
		if msg.String() == keyEnter && !m.encDisabling {
			m.confirmingDisable = true
		}
		return m, nil
	}

	if m.encEnabling {
		// Encryption is running: the form is locked so the password cannot be
		// submitted twice; leaving is still allowed.
		if s := msg.String(); s == "esc" || s == keyLeft {
			m.leaveEncryptionSettings()
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.leaveEncryptionSettings()
		return m, nil
	case keyLeft:
		if m.encryptionInputCursorAtStart() {
			m.leaveEncryptionSettings()
			return m, nil
		}
		return m.updateFocusedEncryptionInput(msg)
	case "tab", keyShiftTab, keyDown, "up":
		if msg.String() == keyShiftTab || msg.String() == "up" {
			m.encFocusIndex--
			if m.encFocusIndex < 0 {
				m.encFocusIndex = 2
			}
		} else {
			m.encFocusIndex++
			if m.encFocusIndex > 2 {
				m.encFocusIndex = 0
			}
		}
		m.encPasswordInput.Blur()
		m.encConfirmInput.Blur()
		var cmds []tea.Cmd
		if m.encFocusIndex == 0 {
			cmds = append(cmds, m.encPasswordInput.Focus())
		}
		if m.encFocusIndex == 1 {
			cmds = append(cmds, m.encConfirmInput.Focus())
		}
		return m, tea.Batch(cmds...)
	case keyEnter:
		switch m.encFocusIndex {
		case 0:
			m.encFocusIndex = 1
			m.encPasswordInput.Blur()
			return m, m.encConfirmInput.Focus()
		case 1:
			m.encFocusIndex = 2
			m.encConfirmInput.Blur()
			return m, nil
		case 2:
			password := m.encPasswordInput.Value()
			confirm := m.encConfirmInput.Value()
			if password == "" {
				m.encError = t("settings_encryption.error_empty")
				return m, nil
			}
			if password != confirm {
				m.encError = t("settings_encryption.error_mismatch")
				return m, nil
			}
			m.encEnabling = true
			m.encError = ""
			cfg := m.cfg
			return m, func() tea.Msg {
				err := config.EnableSecureMode(password, cfg)
				return SecureModeEnabledMsg{Err: err}
			}
		}
	default:
		return m.updateFocusedEncryptionInput(msg)
	}
	return m, nil
}

func (m *Settings) encryptionInputCursorAtStart() bool {
	switch m.encFocusIndex {
	case 0:
		return m.encPasswordInput.Position() == 0
	case 1:
		return m.encConfirmInput.Position() == 0
	default:
		return false
	}
}

func (m *Settings) leaveEncryptionSettings() {
	m.encPasswordInput.SetValue("")
	m.encConfirmInput.SetValue("")
	m.encPasswordStrength = ""
	m.encPasswordInput.Blur()
	m.encConfirmInput.Blur()
	m.encError = ""
	m.leaveContent()
}

func (m *Settings) updateFocusedEncryptionInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.encFocusIndex {
	case 0:
		before := m.encPasswordInput.Value()
		m.encPasswordInput, cmd = m.encPasswordInput.Update(msg)
		if m.encPasswordInput.Value() != before {
			m.handlePasswordChanged()
		}
	case 1:
		m.encConfirmInput, cmd = m.encConfirmInput.Update(msg)
	}
	return m, cmd
}

// encryptionPage shows the encryption status, or the password form that
// enables it with the focused field kept in view.
func (m *Settings) encryptionPage() settingsPage {
	title := titleStyle.Render(t("settings_encryption.title"))
	errLine := ""
	if m.encError != "" {
		errLine = "\n\n" + dangerStyle.Render("  "+m.encError)
	}

	if config.IsSecureModeEnabled() {
		// The disable prompt is drawn as a dialog over the whole view.
		return settingsPage{
			key:    "encryption",
			header: title + "\n",
			body: []string{
				m.contentFocusStyle().Render("  " + t("settings_encryption.enabled")),
				"",
				accountEmailStyle.Render("  " + t("settings_encryption.disable_button")),
			},
			focusTop: -1,
			focusBot: -1,
			footer:   "\n" + helpStyle.Render("enter: disable") + errLine,
		}
	}

	labelStyle := func(idx int) lipgloss.Style {
		if m.encFocusIndex == idx {
			return m.contentFocusStyle()
		}
		return settingsBlurredStyle
	}

	body := make([]string, 0, 12)
	focusTop, focusBot := -1, -1
	mark := func(idx, top int) {
		if m.encFocusIndex == idx {
			focusTop, focusBot = top, len(body)-1
		}
	}

	top := len(body)
	body = append(body,
		labelStyle(0).Render(t("settings_encryption.password_label")),
		m.encPasswordInput.View())
	mark(0, top)
	body = append(body, "")
	if m.encPasswordStrength != "" {
		body = append(body, "  "+m.renderPasswordStrength(), "")
	}

	top = len(body)
	body = append(body,
		labelStyle(1).Render(t("settings_encryption.confirm_label")),
		m.encConfirmInput.View())
	mark(1, top)
	if status := m.renderPasswordMatch(); status != "" {
		body = append(body, "  "+status)
	}
	body = append(body, "")

	top = len(body)
	body = append(body, labelStyle(2).Render("[ "+t("settings_encryption.enable_button")+" ]"))
	mark(2, top)
	if m.encEnabling {
		body = append(body, "", accountEmailStyle.Render("  "+t("settings_encryption.encrypting")))
	}

	return settingsPage{
		key:      "encryption",
		header:   title + "\n\n" + accountEmailStyle.Render(t("settings_encryption.disabled")) + "\n",
		body:     body,
		focusTop: focusTop,
		focusBot: focusBot,
		footer:   "\n" + helpStyle.Render(t("settings_encryption.help")) + errLine,
	}
}

func (m *Settings) renderPasswordMatch() string {
	password := m.encPasswordInput.Value()
	confirm := m.encConfirmInput.Value()
	if confirm == "" {
		return ""
	}
	if password == confirm {
		return successStyle.Render(t("settings_encryption.passwords_match"))
	}
	return dangerStyle.Render(t("settings_encryption.passwords_do_not_match"))
}

func (m *Settings) handlePasswordChanged() {
	password := m.encPasswordInput.Value()
	if password == "" {
		m.encPasswordStrength = ""
		return
	}
	m.encPasswordStrength = m.passwordMeter.Strength(password)
}

func (m *Settings) renderPasswordStrength() string {
	switch m.encPasswordStrength {
	case passwordstrength.Strong:
		return successStyle.Render(t("settings_encryption.strength_label") + " " + t("settings_encryption.strength_strong"))
	case passwordstrength.Medium:
		return settingsFocusedStyle.Render(t("settings_encryption.strength_label") + " " + t("settings_encryption.strength_medium"))
	case passwordstrength.Weak:
		return dangerStyle.Render(t("settings_encryption.strength_label") + " " + t("settings_encryption.strength_weak"))
	}
	return dangerStyle.Render(t("settings_encryption.strength_label") + " " + t("settings_encryption.strength_weak"))
}
