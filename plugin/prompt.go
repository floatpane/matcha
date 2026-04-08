package plugin

import (
	"log"

	lua "github.com/yuin/gopher-lua"
)

// PendingPrompt holds the state for a plugin-requested user input prompt.
type PendingPrompt struct {
	Placeholder string
	callback    *lua.LFunction
}

// luaPrompt implements matcha.prompt(placeholder, callback).
// It requests a text input overlay in the TUI. When the user submits,
// the callback is called with their input string.
func (m *Manager) luaPrompt(L *lua.LState) int {
	placeholder := L.CheckString(1)
	fn := L.CheckFunction(2)

	m.pendingPrompt = &PendingPrompt{
		Placeholder: placeholder,
		callback:    fn,
	}
	return 0
}

// TakePendingPrompt returns and clears any pending prompt request.
func (m *Manager) TakePendingPrompt() (*PendingPrompt, bool) {
	if m.pendingPrompt == nil {
		return nil, false
	}
	p := m.pendingPrompt
	m.pendingPrompt = nil
	return p, true
}

// ResolvePrompt calls the stored prompt callback with the user's input.
func (m *Manager) ResolvePrompt(prompt *PendingPrompt, input string) {
	if prompt == nil || prompt.callback == nil {
		return
	}
	if err := m.state.CallByParam(lua.P{
		Fn:      prompt.callback,
		NRet:    0,
		Protect: true,
	}, lua.LString(input)); err != nil {
		log.Printf("plugin prompt callback error: %v", err)
	}
}
