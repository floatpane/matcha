package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/i18n"
)

func (m *Settings) updateGeneral(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.generalCursor > 0 {
			m.generalCursor--
		}
	case "down", "j":
		if m.generalCursor < 5 {
			m.generalCursor++
		}
	case "enter", "space", "right", "l":
		switch m.generalCursor {
		case 0: // Image Display
			m.cfg.DisableImages = !m.cfg.DisableImages
			_ = config.SaveConfig(m.cfg)
		case 1: // Contextual Tips
			m.cfg.HideTips = !m.cfg.HideTips
			_ = config.SaveConfig(m.cfg)
		case 2: // Desktop Notifications
			m.cfg.DisableNotifications = !m.cfg.DisableNotifications
			_ = config.SaveConfig(m.cfg)
		case 3: // Date Format
			switch m.cfg.DateFormat {
			case config.DateFormatEU:
				m.cfg.DateFormat = config.DateFormatUS
			case config.DateFormatUS:
				m.cfg.DateFormat = config.DateFormatISO
			default: // or ISO
				m.cfg.DateFormat = config.DateFormatEU
			}
			_ = config.SaveConfig(m.cfg)
		case 4: // Language
			// Cycle through available languages
			langs := i18n.LanguageCodes()
			currentLang := m.cfg.GetLanguage()
			currentIdx := -1
			for i, lang := range langs {
				if lang == currentLang {
					currentIdx = i
					break
				}
			}
			nextIdx := (currentIdx + 1) % len(langs)
			m.cfg.Language = langs[nextIdx]
			_ = config.SaveConfig(m.cfg)
		case 5: // Edit Signature
			if msg.String() == "enter" || msg.String() == "right" || msg.String() == "l" {
				return m, func() tea.Msg { return GoToSignatureEditorMsg{} }
			}
		}
	}
	return m, nil
}

func (m *Settings) viewGeneral() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("General Settings") + "\n\n")

	options := []struct {
		labelKey string
		value    string
		tip      string
	}{
		{"settings_general.disable_images", onOff(m.cfg.DisableImages), "Prevent images from loading automatically in emails."},
		{"settings_general.hide_tips", onOff(m.cfg.HideTips), "Hide helpful hints displayed at the bottom of the screen."},
		{"settings_general.disable_notifications", onOff(m.cfg.DisableNotifications), "Turn off desktop notifications for new mail."},
		{"settings_general.date_format", getDateFormatLabel(m.cfg.DateFormat), "Change how dates and times are displayed."},
		{"settings_general.language", getLanguageLabel(m.cfg.GetLanguage()), "Change the interface language. Restart required."},
		{"settings_general.signature", getSignatureStatus(), "Configure the signature appended to your outgoing emails."},
	}

	for i, opt := range options {
		cursor := "  "
		style := accountItemStyle
		if m.generalCursor == i {
			cursor = "> "
			style = selectedAccountItemStyle
		}

		label := t(opt.labelKey)
		text := fmt.Sprintf("%s: %s", label, opt.value)
		if opt.labelKey == "settings_general.signature" {
			text = fmt.Sprintf("%s (%s)", label, opt.value)
		}

		b.WriteString(style.Render(cursor+text) + "\n")
	}

	b.WriteString("\n\n")

	if !m.cfg.HideTips && m.generalCursor < len(options) {
		b.WriteString(TipStyle.Render("Tip: " + options[m.generalCursor].tip))
	}

	return b.String()
}

func onOff(b bool) string {
	if b {
		return t("settings_general.on")
	}
	return t("settings_general.off")
}

func getDateFormatLabel(f string) string {
	if f == "" {
		f = config.DateFormatEU
	}
	switch f {
	case config.DateFormatUS:
		return "US (MM/DD/YYYY hh:MM AM)"
	case config.DateFormatISO:
		return "ISO (YYYY-MM-DD HH:MM)"
	default:
		return "EU (DD/MM/YYYY HH:MM)"
	}
}

func getSignatureStatus() string {
	if config.HasSignature() {
		return t("settings_general.signature_configured")
	}
	return t("settings_general.signature_not_configured")
}

func getLanguageLabel(langCode string) string {
	if locale, ok := i18n.GetLanguage(langCode); ok {
		return fmt.Sprintf("%s (%s)", locale.NativeName, locale.Code)
	}
	return langCode
}
