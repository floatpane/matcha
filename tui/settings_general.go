package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
)

func (m *Settings) updateGeneral(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.generalCursor > 0 {
			m.generalCursor--
		}
	case "down", "j":
		if m.generalCursor < 4 {
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
		case 4: // Edit Signature
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
		label string
		value string
		tip   string
	}{
		{"Disable Image Display", onOff(m.cfg.DisableImages), "Prevent images from loading automatically in emails."},
		{"Hide Contextual Tips", onOff(m.cfg.HideTips), "Hide helpful hints displayed at the bottom of the screen."},
		{"Disable Notifications", onOff(m.cfg.DisableNotifications), "Turn off desktop notifications for new mail."},
		{"Date Format", getDateFormatLabel(m.cfg.DateFormat), "Change how dates and times are displayed."},
		{"Signature", getSignatureStatus(), "Configure the signature appended to your outgoing emails."},
	}

	for i, opt := range options {
		cursor := "  "
		style := accountItemStyle
		if m.generalCursor == i {
			cursor = "> "
			style = selectedAccountItemStyle
		}

		text := fmt.Sprintf("%s: %s", opt.label, opt.value)
		if opt.label == "Signature" {
			text = fmt.Sprintf("Edit Signature (%s)", opt.value)
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
		return "ON"
	}
	return "OFF"
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
		return "configured"
	}
	return "not configured"
}
