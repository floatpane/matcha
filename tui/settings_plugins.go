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
	m.pluginListCursor = max(0, min(m.pluginListCursor, len(schemas)-1))

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
	m.pluginSettingCursor = max(0, min(m.pluginSettingCursor, len(defs)-1))

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
			return m, m.persistPluginSettings()
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
		return m, m.persistPluginSettings()
	}

	// Forward all other keys (typing, backspace, arrows, etc.) to textinput.
	var cmd tea.Cmd
	m.pluginInput, cmd = m.pluginInput.Update(msg)
	return m, cmd
}

// persistPluginSettings saves the plugin settings and tells the app the
// config changed, reporting a failed save.
func (m *Settings) persistPluginSettings() tea.Cmd {
	if m.cfg == nil || m.plugins == nil {
		return nil
	}
	m.cfg.PluginSettings = m.plugins.AllSettingValues()
	return tea.Batch(m.saveConfig(), func() tea.Msg { return ConfigSavedMsg{} })
}

// pluginsPage shows the plugins with configurable settings, or the settings
// of the selected plugin.
func (m *Settings) pluginsPage() settingsPage {
	title := titleStyle.Render(t("settings.category_plugins"))
	page := settingsPage{key: "plugins", header: title + "\n", focusTop: -1, focusBot: -1}

	if m.plugins == nil {
		page.body = []string{accountEmailStyle.Render("  Plugin manager unavailable.")}
		return page
	}

	if m.pluginSelected == "" {
		schemas := m.plugins.Schemas()
		if len(schemas) == 0 {
			page.body = []string{accountEmailStyle.Render("  No plugins declare configurable settings.")}
			page.footer = "\n" + helpStyle.Render("Plugins use matcha.settings(...) to expose options.")
			return page
		}

		page.body = make([]string, len(schemas))
		for i, s := range schemas {
			selected := m.pluginListCursor == i
			line := fmt.Sprintf("%s (%d %s)", s.Plugin, len(s.Defs), pluralSettings(len(s.Defs)))
			page.body[i] = m.contentItemStyle(selected).Render(m.contentCursor(selected) + line)
		}
		page.focusTop, page.focusBot = m.pluginListCursor, m.pluginListCursor
		page.footer = "\n" + helpStyle.Render(navArrowsLabel()+" navigate • enter open • esc back")
		page.rows = true
		return page
	}

	defs := m.plugins.Schema(m.pluginSelected)
	page.key = "plugin:" + m.pluginSelected
	page.header = title + "\n\n" + accountEmailStyle.Render(m.pluginSelected) + "\n"
	page.body = make([]string, len(defs))
	for i, def := range defs {
		selected := m.pluginSettingCursor == i
		label := def.Label
		if label == "" {
			label = def.Key
		}
		val, _ := m.plugins.GetSettingValue(m.pluginSelected, def.Key)
		line := fmt.Sprintf("%s: %s", label, formatDisplayValue(def.Type, val))
		page.body[i] = m.contentItemStyle(selected).Render(m.contentCursor(selected) + line)
	}
	page.focusTop, page.focusBot = m.pluginSettingCursor, m.pluginSettingCursor

	if m.pluginEditing {
		page.footer = "\n" + m.contentFocusStyle().Render("Edit "+m.pluginEditingKey) + "\n" +
			m.pluginInput.View() + "\n\n" + helpStyle.Render("enter save • esc cancel")
		return page
	}

	page.rows = true
	if m.pluginSettingCursor >= 0 && m.pluginSettingCursor < len(defs) {
		if tip := defs[m.pluginSettingCursor].Description; tip != "" && !m.cfg.HideTips {
			page.extra = "\n" + TipStyle.Render("Tip: "+tip)
		}
	}
	page.footer = "\n" + helpStyle.Render(navArrowsLabel()+" navigate • enter toggle/edit • "+cancelKey()+" back")
	return page
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
