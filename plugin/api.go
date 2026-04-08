package plugin

import (
	"log"

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
