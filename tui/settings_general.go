package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/i18n"
)

type generalOption struct {
	labelKey string
	value    string
	tip      string
}

func (m *Settings) buildGeneralOptions() []generalOption {
	opts := []generalOption{
		{"settings_general.disable_images", onOff(m.cfg.DisableImages), "Prevent images from loading automatically in emails."},
		{"settings_general.hide_tips", onOff(m.cfg.HideTips), "Hide helpful hints displayed at the bottom of the screen."},
		{"settings_general.disable_notifications", onOff(m.cfg.DisableNotifications), "Turn off desktop notifications for new mail."},
		{"settings_general.disable_daemon", onOff(!m.cfg.DisableDaemon), "Run a background daemon for push notifications and sync. Takes effect on restart."},
		{"settings_general.enable_split_pane", onOff(m.cfg.EnableSplitPane), "View inbox and email side-by-side."},
		{"settings_general.split_pane_orientation", getSplitPaneOrientationLabel(m.cfg.GetSplitPaneOrientation()), "Lay the split pane out side-by-side or stacked."},
		{"settings_general.enable_threaded", onOff(m.cfg.EnableThreaded), "Group emails into conversations by reply chain. Per-folder overrides are kept."},
		{"settings_general.enable_detailed_dates", onOff(m.cfg.EnableDetailedDates), "Show detailed inbox dates."},
		{"settings_general.spellcheck", onOff(!m.cfg.DisableSpellcheck), "Underline misspelled words while composing."},
		{"settings_general.spell_suggestions", onOff(!m.cfg.DisableSpellSuggestions), "Show suggestion popup for misspelled words."},
		{"settings_general.date_format", getDateFormatLabel(m.cfg.DateFormat), "Change how dates and times are displayed."},
		{"settings_general.language", getLanguageLabel(m.cfg.GetLanguage()), "Change the interface language. Changes apply instantly."},
		{"settings_general.signature", getSignatureStatus(), "Configure the global signature appended to your outgoing emails."},
		{"settings_general.mouse_support", onOff(config.MouseEnabled != nil && *config.MouseEnabled), "Enable mouse clicks and scroll wheel in the TUI. Takes effect immediately."},
	}

	return opts
}

func (m *Settings) updateGeneral(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	opts := m.buildGeneralOptions()

	switch msg.String() {
	case "up", "k":
		m.generalCursor = (m.generalCursor - 1 + len(opts)) % len(opts)
	case keyDown, "j":
		m.generalCursor = (m.generalCursor + 1) % len(opts)
	case keyEnter, "space", keyRight, "l":
		if m.generalCursor < len(opts) {
			saved := false
			switch m.generalCursor {
			case 0: // Image Display
				m.cfg.DisableImages = !m.cfg.DisableImages
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 1: // Contextual Tips
				m.cfg.HideTips = !m.cfg.HideTips
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 2: // Desktop Notifications
				m.cfg.DisableNotifications = !m.cfg.DisableNotifications
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 3: // Background Daemon
				m.cfg.DisableDaemon = !m.cfg.DisableDaemon
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 4: // Split Pane View
				m.cfg.EnableSplitPane = !m.cfg.EnableSplitPane
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 5: // Split Pane Orientation
				if m.cfg.GetSplitPaneOrientation() == config.SplitPaneVertical {
					m.cfg.SplitPaneOrientation = config.SplitPaneHorizontal
				} else {
					m.cfg.SplitPaneOrientation = config.SplitPaneVertical
				}
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 6: // Threaded Conversation View
				m.cfg.EnableThreaded = !m.cfg.EnableThreaded
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 7: // Detailed Dates
				m.cfg.EnableDetailedDates = !m.cfg.EnableDetailedDates
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 8: // Spellcheck
				m.cfg.DisableSpellcheck = !m.cfg.DisableSpellcheck
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 9: // Spell Suggestions
				m.cfg.DisableSpellSuggestions = !m.cfg.DisableSpellSuggestions
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 10: // Date Format
				switch m.cfg.DateFormat {
				case config.DateFormatEU:
					m.cfg.DateFormat = config.DateFormatUS
				case config.DateFormatUS:
					m.cfg.DateFormat = config.DateFormatISO
				default: // or ISO
					m.cfg.DateFormat = config.DateFormatEU
				}
				_ = config.SaveConfig(m.cfg)
				saved = true
			case 11: // Language
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
				// Apply language change immediately
				i18n.GetManager().SetLanguage(m.cfg.Language) //nolint:errcheck,gosec
				// Trigger full UI rebuild
				return m, tea.Batch(
					func() tea.Msg { return ConfigSavedMsg{} },
					func() tea.Msg { return LanguageChangedMsg{} },
				)
			case 12: // Edit Signature
				if msg.String() == keyEnter || msg.String() == keyRight || msg.String() == "l" {
					return m, func() tea.Msg { return GoToSignatureEditorMsg{} }
				}
			case 13: // Mouse Support
				enabled := config.MouseEnabled == nil || !*config.MouseEnabled
				config.MouseEnabled = &enabled
				return m, func() tea.Msg { return MouseSupportChosenMsg{Enabled: enabled} }
			}
			if saved {
				return m, func() tea.Msg { return ConfigSavedMsg{} }
			}
		}
	}
	return m, nil
}

func (m *Settings) viewGeneral() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("General Settings") + "\n\n")

	options := m.buildGeneralOptions()

	for i, opt := range options {
		selected := m.generalCursor == i
		cursor := m.contentCursor(selected)
		style := m.contentItemStyle(selected)

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

func getSplitPaneOrientationLabel(orientation string) string {
	if orientation == config.SplitPaneVertical {
		return t("settings_general.split_pane_orientation_vertical")
	}
	return t("settings_general.split_pane_orientation_horizontal")
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
