package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/floatpane/matcha/config"
)

func (m *Settings) updateEncryption(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	isEnabled := config.IsSecureModeEnabled()

	if isEnabled {
		if m.confirmingDisable {
			switch msg.String() {
			case "y", "Y":
				m.confirmingDisable = false
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
		if msg.String() == "enter" {
			m.confirmingDisable = true
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		// Clear inputs and return to menu
		m.encPasswordInput.SetValue("")
		m.encConfirmInput.SetValue("")
		m.encPasswordInput.Blur()
		m.encConfirmInput.Blur()
		m.encError = ""
		m.activePane = PaneMenu
		return m, nil
	case "tab", "shift+tab", "down", "up":
		if msg.String() == "shift+tab" || msg.String() == "up" {
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
	case "enter":
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
		// Forward input to focused textinput
		var cmd tea.Cmd
		if m.encFocusIndex == 0 {
			m.encPasswordInput, cmd = m.encPasswordInput.Update(msg)
		} else if m.encFocusIndex == 1 {
			m.encConfirmInput, cmd = m.encConfirmInput.Update(msg)
		}
		return m, cmd
	}
	return m, nil
}

func (m *Settings) viewEncryption() string {
	var b strings.Builder
	isEnabled := config.IsSecureModeEnabled()

	b.WriteString(titleStyle.Render(t("settings_encryption.title")) + "\n\n")

	if isEnabled {
		if m.confirmingDisable {
			dialog := DialogBoxStyle.Render(
				lipgloss.JoinVertical(lipgloss.Center,
					dangerStyle.Render(t("settings_encryption.disable_confirm")),
					accountEmailStyle.Render(t("settings_encryption.disable_warning")),
					HelpStyle.Render("\n(y/n)"),
				),
			)
			b.WriteString(dialog + "\n")
		} else {
			b.WriteString(settingsFocusedStyle.Render("  "+t("settings_encryption.enabled")) + "\n\n")
			b.WriteString(accountEmailStyle.Render("  "+t("settings_encryption.disable_button")) + "\n\n")
			b.WriteString(helpStyle.Render("enter: disable"))
		}
	} else {
		b.WriteString(accountEmailStyle.Render(t("settings_encryption.disabled")) + "\n\n")

		if m.encFocusIndex == 0 {
			b.WriteString(settingsFocusedStyle.Render(t("settings_encryption.password_label") + "\n"))
		} else {
			b.WriteString(settingsBlurredStyle.Render(t("settings_encryption.password_label") + "\n"))
		}
		b.WriteString(m.encPasswordInput.View() + "\n\n")

		if m.encFocusIndex == 1 {
			b.WriteString(settingsFocusedStyle.Render(t("settings_encryption.confirm_label") + "\n"))
		} else {
			b.WriteString(settingsBlurredStyle.Render(t("settings_encryption.confirm_label") + "\n"))
		}
		b.WriteString(m.encConfirmInput.View() + "\n\n")

		saveBtn := "[ " + t("settings_encryption.enable_button") + " ]"
		if m.encFocusIndex == 2 {
			b.WriteString(settingsFocusedStyle.Render(saveBtn) + "\n")
		} else {
			b.WriteString(settingsBlurredStyle.Render(saveBtn) + "\n")
		}

		if m.encEnabling {
			b.WriteString("\n" + accountEmailStyle.Render("  "+t("settings_encryption.encrypting")) + "\n")
		}

		b.WriteString("\n" + helpStyle.Render(t("settings_encryption.help")))
	}

	if m.encError != "" {
		b.WriteString("\n" + dangerStyle.Render("  "+m.encError) + "\n")
	}

	return b.String()
}
