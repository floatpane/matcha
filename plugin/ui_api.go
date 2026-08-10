package plugin

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// luaUISetText implements matcha.ui.set_text(key, value).
//
// It stores a plugin-provided override for a UI string identified by its i18n
// key (e.g. "folder_inbox.folders_title"). When the TUI renders a label, it
// checks the override registry before falling back to the locale bundle, so a
// plugin can rename "Inbox" to "Priority Queue" or translate "Drafts" to a
// custom term without shipping a full locale file.
//
// Passing an empty value clears the override for that key.
func (m *Manager) luaUISetText(L *lua.LState) int { //nolint:gocritic
	key := L.CheckString(1)
	value := L.CheckString(2)

	if key == "" {
		L.ArgError(1, "text key must not be empty")
		return 0
	}

	if value == "" {
		m.ClearTextOverride(key)
	} else {
		m.SetTextOverride(key, value)
	}
	return 0
}

// luaUISetVisible implements matcha.ui.set_visible(component_name, boolean).
//
// It toggles the visibility of a major UI pane. Valid component names are
// "sidebar", "status_bar", and "header". When a component is hidden, the main
// layout renderer skips drawing it and recalculates the flexbox/grid dimensions
// so the remaining panes fill the freed space.
//
// Returns true on success, false if the component name is not recognised.
func (m *Manager) luaUISetVisible(L *lua.LState) int { //nolint:gocritic
	name := L.CheckString(1)
	visible := L.ToBool(2)

	if name == "" {
		L.ArgError(1, "component name must not be empty")
		return 0
	}

	if ok := m.SetComponentVisible(name, visible); !ok {
		valid := []string{"sidebar", "status_bar", "header"}
		L.ArgError(1, "invalid component: must be one of "+strings.Join(valid, ", "))
		return 0
	}
	L.Push(lua.LBool(true))
	return 1
}

// luaUIAddComponent implements matcha.ui.add_component(id, content, position).
//
// It registers a custom component that the TUI overlays or inserts into the
// terminal grid. id is a unique identifier (re-adding with the same id replaces
// the previous content). content is a pre-rendered string (may include ANSI
// styling from matcha.style). position is one of "floating", "top_left",
// "top_right", "bottom_left", "bottom_right".
//
// Returns true on success, false if the position is invalid.
func (m *Manager) luaUIAddComponent(L *lua.LState) int { //nolint:gocritic
	id := L.CheckString(1)
	content := L.CheckString(2)
	position := L.OptString(3, "floating")

	if id == "" {
		L.ArgError(1, "component id must not be empty")
		return 0
	}

	if ok := m.AddComponent(id, content, position); !ok {
		valid := []string{"floating", "top_left", "top_right", "bottom_left", "bottom_right"}
		L.ArgError(3, "invalid position: must be one of "+strings.Join(valid, ", "))
		return 0
	}
	L.Push(lua.LBool(true))
	return 1
}

// luaUISetBanner implements matcha.ui.set_banner(multiline_string).
//
// It replaces the default ASCII logo shown on the startup / choice screen with
// a custom multiline string. The banner is rendered with the same logo styling
// (accent colour) as the default. Passing an empty string restores the default
// logo.
//
// This function is typically called inside a "startup" hook so the banner is
// in place before the first frame renders:
//
//	matcha.on("startup", function()
//	    matcha.ui.set_banner("  Welcome to My Custom Mail!")
//	end)
func (m *Manager) luaUISetBanner(L *lua.LState) int { //nolint:gocritic
	banner := L.CheckString(1)
	m.SetBanner(banner)
	return 0
}
