package plugin

import (
	"log"

	lua "github.com/yuin/gopher-lua"
)

// registerAPI registers the "matcha" module into the Lua VM.
func (m *Manager) registerAPI() {
	L := m.state

	mod := L.RegisterModule("matcha", map[string]lua.LGFunction{
		"on":         m.luaOn,
		"log":        m.luaLog,
		"notify":     m.luaNotify,
		"set_status": m.luaSetStatus,
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
