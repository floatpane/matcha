package tui

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/plugin"
)

// updatePlugins handles input for the plugins settings category. The view has
// three states:
//
//  1. Plugin list (m.pluginSelected == ""): pick a plugin to configure.
//  2. Plugin settings list (m.pluginSelected != "", m.pluginEditing == false):
//     navigate keys; enter/space toggles booleans, enter on number/string
//     opens an editor.
//  3. Editing input (m.pluginEditing == true): textinput for number/string;
//     enter commits, esc cancels.
func (m *Settings) updatePlugins(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.plugins == nil {
		return m, nil
	}

	if m.pluginEditing {
		return m.updatePluginEditor(msg)
	}

	if m.pluginSelected == "" {
		return m.updatePluginList(msg)
	}

	return m.updatePluginSettings(msg)
}

func (m *Settings) updatePluginList(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	schemas := m.plugins.Schemas()
	if len(schemas) == 0 {
		return m, nil
	}

	kb := config.Keybinds.Global
	key := msg.String()
	switch key {
	case "up", kb.NavUp:
		m.pluginListCursor = (m.pluginListCursor - 1 + len(schemas)) % len(schemas)
	case keyDown, kb.NavDown:
		m.pluginListCursor = (m.pluginListCursor + 1) % len(schemas)
	case keyEnter, keyRight, "l":
		m.pluginSelected = schemas[m.pluginListCursor].Plugin
		m.pluginSettingCursor = 0
	}
	return m, nil
}

func (m *Settings) updatePluginSettings(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	defs := m.plugins.Schema(m.pluginSelected)
	if len(defs) == 0 {
		m.pluginSelected = ""
		return m, nil
	}

	kb := config.Keybinds.Global
	key := msg.String()
	switch key {
	case "esc", "left", "h", kb.Cancel:
		m.pluginSelected = ""
		return m, nil
	case "up", kb.NavUp:
		m.pluginSettingCursor = (m.pluginSettingCursor - 1 + len(defs)) % len(defs)
	case keyDown, kb.NavDown:
		m.pluginSettingCursor = (m.pluginSettingCursor + 1) % len(defs)
	case keyEnter, "space", keyRight, "l":
		def := defs[m.pluginSettingCursor]
		switch def.Type {
		case plugin.SettingBool:
			cur, _ := m.plugins.GetSettingValue(m.pluginSelected, def.Key)
			b, _ := cur.(bool)
			m.plugins.SetSettingValue(m.pluginSelected, def.Key, !b)
			m.persistPluginSettings()
			return m, func() tea.Msg { return ConfigSavedMsg{} }
		case plugin.SettingNumber, plugin.SettingString:
			m.beginPluginEdit(def)
		}
	}
	return m, nil
}

func (m *Settings) beginPluginEdit(def plugin.SettingDef) {
	m.pluginEditing = true
	m.pluginEditingKey = def.Key
	m.pluginEditingType = def.Type
	cur, _ := m.plugins.GetSettingValue(m.pluginSelected, def.Key)
	m.pluginInput.SetValue(formatSettingValue(cur))
	m.pluginInput.Focus()
}

func (m *Settings) updatePluginEditor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.pluginEditing = false
		m.pluginInput.Blur()
		return m, nil
	case keyEnter:
		raw := m.pluginInput.Value()
		switch m.pluginEditingType {
		case plugin.SettingNumber:
			n, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil {
				return m, nil
			}
			m.plugins.SetSettingValue(m.pluginSelected, m.pluginEditingKey, n)
		case plugin.SettingString:
			m.plugins.SetSettingValue(m.pluginSelected, m.pluginEditingKey, raw)
		case plugin.SettingBool:
			// Bool settings are toggled directly, not via text input
		}
		m.pluginEditing = false
		m.pluginInput.Blur()
		m.persistPluginSettings()
		return m, func() tea.Msg { return ConfigSavedMsg{} }
	}

	// Forward all other keys (typing, backspace, arrows, etc.) to textinput.
	var cmd tea.Cmd
	m.pluginInput, cmd = m.pluginInput.Update(msg)
	return m, cmd
}

func (m *Settings) persistPluginSettings() {
	if m.cfg == nil || m.plugins == nil {
		return
	}
	m.cfg.PluginSettings = m.plugins.AllSettingValues()
	_ = config.SaveConfig(m.cfg)
}

func (m *Settings) viewPlugins() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(t("settings.category_plugins")) + "\n\n")

	if m.plugins == nil {
		b.WriteString(accountEmailStyle.Render("  Plugin manager unavailable.\n"))
		return b.String()
	}

	if m.pluginSelected == "" {
		schemas := m.plugins.Schemas()
		if len(schemas) == 0 {
			b.WriteString(accountEmailStyle.Render("  No plugins declare configurable settings.\n"))
			b.WriteString("\n")
			b.WriteString(helpStyle.Render("Plugins use matcha.settings(...) to expose options."))
			return b.String()
		}

		for i, s := range schemas {
			cursor := "  "
			style := accountItemStyle
			if m.pluginListCursor == i {
				cursor = "> "
				style = selectedAccountItemStyle
			}
			line := fmt.Sprintf("%s (%d %s)", s.Plugin, len(s.Defs), pluralSettings(len(s.Defs)))
			b.WriteString(style.Render(cursor+line) + "\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("↑/↓ navigate • enter open • esc back"))
		return b.String()
	}

	defs := m.plugins.Schema(m.pluginSelected)
	b.WriteString(accountEmailStyle.Render(m.pluginSelected) + "\n\n")

	for i, def := range defs {
		cursor := "  "
		style := accountItemStyle
		if m.pluginSettingCursor == i {
			cursor = "> "
			style = selectedAccountItemStyle
		}

		label := def.Label
		if label == "" {
			label = def.Key
		}
		val, _ := m.plugins.GetSettingValue(m.pluginSelected, def.Key)
		display := formatDisplayValue(def.Type, val)
		line := fmt.Sprintf("%s: %s", label, display)
		b.WriteString(style.Render(cursor+line) + "\n")
	}

	if m.pluginEditing {
		b.WriteString("\n")
		b.WriteString(settingsFocusedStyle.Render("Edit "+m.pluginEditingKey) + "\n")
		b.WriteString(m.pluginInput.View() + "\n")
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("enter save • esc cancel"))
	} else {
		if m.pluginSettingCursor < len(defs) {
			tip := defs[m.pluginSettingCursor].Description
			if tip != "" && !m.cfg.HideTips {
				b.WriteString("\n")
				b.WriteString(TipStyle.Render("Tip: " + tip))
			}
		}
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("↑/↓ navigate • enter toggle/edit • esc back"))
	}

	return b.String()
}

func formatSettingValue(v interface{}) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return x
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func formatDisplayValue(typ plugin.SettingType, v interface{}) string {
	if typ == plugin.SettingBool {
		b, _ := v.(bool)
		if b {
			return "[x] on"
		}
		return "[ ] off"
	}
	s := formatSettingValue(v)
	if s == "" && typ == plugin.SettingString {
		return "(empty)"
	}
	return s
}

func pluralSettings(n int) string {
	if n == 1 {
		return "setting"
	}
	return "settings"
}
