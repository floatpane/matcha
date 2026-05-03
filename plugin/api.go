package plugin

import (
	"log"

	"charm.land/lipgloss/v2"
	lua "github.com/yuin/gopher-lua"
)

// registerAPI registers the "matcha" module into the Lua VM.
func (m *Manager) registerAPI() {
	L := m.state

	mod := L.RegisterModule("matcha", map[string]lua.LGFunction{
		"on":                m.luaOn,
		"log":               m.luaLog,
		"notify":            m.luaNotify,
		"set_status":        m.luaSetStatus,
		"set_compose_field": m.luaSetComposeField,
		"bind_key":          m.luaBindKey,
		"http":              m.luaHTTP,
		"prompt":            m.luaPrompt,
		"style":             m.luaStyle,
	})

	L.SetField(mod, "_VERSION", lua.LString("0.1.0"))
}

// matcha.on(event, callback) — register a hook callback.
func (m *Manager) luaOn(L *lua.LState) int {
	event := L.CheckString(1)
	fn := L.CheckFunction(2)
	m.registerHook(event, fn)
	return 0
}

// matcha.log(msg) — log a message to stderr.
func (m *Manager) luaLog(L *lua.LState) int {
	msg := L.CheckString(1)
	log.Printf("[plugin] %s", msg)
	return 0
}

// matcha.set_status(area, text) — set a persistent status string for a view area.
// Valid areas: "inbox", "composer", "email_view".
func (m *Manager) luaSetStatus(L *lua.LState) int {
	area := L.CheckString(1)
	text := L.CheckString(2)
	m.statuses[area] = text
	return 0
}

// matcha.notify(msg [, seconds]) — show a temporary notification in the TUI.
// The optional second argument sets the display duration in seconds (default 2).
func (m *Manager) luaNotify(L *lua.LState) int {
	m.pendingNotification = L.CheckString(1)
	m.pendingDuration = float64(L.OptNumber(2, 2))
	return 0
}

// matcha.bind_key(key, area, description, callback) — register a custom keyboard shortcut.
// Valid areas: "inbox", "email_view", "composer".
func (m *Manager) luaBindKey(L *lua.LState) int {
	key := L.CheckString(1)
	area := L.CheckString(2)
	description := L.CheckString(3)
	fn := L.CheckFunction(4)

	switch area {
	case "inbox", "email_view", "composer":
		m.bindings = append(m.bindings, KeyBinding{
			Key:         key,
			Area:        area,
			Description: description,
			Fn:          fn,
		})
	default:
		L.ArgError(2, "invalid area: must be \"inbox\", \"email_view\", or \"composer\"")
	}
	return 0
}

// matcha.style(text, opts) — wrap text in lipgloss styling and return the
// resulting ANSI-styled string. opts is a table with optional keys:
//   - color, bg: string (hex "#rrggbb", ANSI 256 number as string, or named like "red")
//   - bold, italic, underline, strikethrough, faint, blink, reverse: bool
//
// Plugins use this from email_body_render callbacks to style matched substrings:
//
//	matcha.on("email_body_render", function(email, body)
//	    return (body:gsub("TODO", function(m)
//	        return matcha.style(m, {color = "#ff0000", bold = true})
//	    end))
//	end)
func (m *Manager) luaStyle(L *lua.LState) int {
	text := L.CheckString(1)
	opts := L.OptTable(2, nil)

	style := lipgloss.NewStyle()
	if opts != nil {
		if v, ok := opts.RawGetString("color").(lua.LString); ok && v != "" {
			style = style.Foreground(lipgloss.Color(string(v)))
		}
		if v, ok := opts.RawGetString("bg").(lua.LString); ok && v != "" {
			style = style.Background(lipgloss.Color(string(v)))
		}
		if lua.LVAsBool(opts.RawGetString("bold")) {
			style = style.Bold(true)
		}
		if lua.LVAsBool(opts.RawGetString("italic")) {
			style = style.Italic(true)
		}
		if lua.LVAsBool(opts.RawGetString("underline")) {
			style = style.Underline(true)
		}
		if lua.LVAsBool(opts.RawGetString("strikethrough")) {
			style = style.Strikethrough(true)
		}
		if lua.LVAsBool(opts.RawGetString("faint")) {
			style = style.Faint(true)
		}
		if lua.LVAsBool(opts.RawGetString("blink")) {
			style = style.Blink(true)
		}
		if lua.LVAsBool(opts.RawGetString("reverse")) {
			style = style.Reverse(true)
		}
	}

	L.Push(lua.LString(style.Render(text)))
	return 1
}

// matcha.set_compose_field(field, value) — set a compose field value.
// Valid fields: "to", "cc", "bcc", "subject", "body".
func (m *Manager) luaSetComposeField(L *lua.LState) int {
	field := L.CheckString(1)
	value := L.CheckString(2)

	switch field {
	case "to", "cc", "bcc", "subject", "body":
		m.pendingFields[field] = value
	default:
		L.ArgError(1, "invalid field: must be \"to\", \"cc\", \"bcc\", \"subject\", or \"body\"")
	}
	return 0
}
