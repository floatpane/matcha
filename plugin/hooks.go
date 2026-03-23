package plugin

import (
	"log"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// Hook event names.
const (
	HookStartup         = "startup"
	HookShutdown        = "shutdown"
	HookEmailReceived   = "email_received"
	HookEmailSendBefore = "email_send_before"
	HookEmailSendAfter  = "email_send_after"
	HookEmailViewed     = "email_viewed"
	HookFolderChanged   = "folder_changed"
	HookComposerUpdated = "composer_updated"
)

// Status area names.
const (
	StatusInbox     = "inbox"
	StatusComposer  = "composer"
	StatusEmailView = "email_view"
)

// registerHook adds a callback for the given event.
func (m *Manager) registerHook(event string, fn *lua.LFunction) {
	m.hooks[event] = append(m.hooks[event], fn)
}

// CallHook invokes all callbacks registered for the given event.
func (m *Manager) CallHook(event string, args ...lua.LValue) {
	callbacks, ok := m.hooks[event]
	if !ok {
		return
	}

	for _, fn := range callbacks {
		if err := m.state.CallByParam(lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		}, args...); err != nil {
			log.Printf("plugin hook %q error: %v", event, err)
		}
	}
}

// CallSendHook calls a hook with email send metadata.
func (m *Manager) CallSendHook(event string, to, cc, subject, accountID string) {
	callbacks, ok := m.hooks[event]
	if !ok {
		return
	}

	L := m.state
	t := L.NewTable()
	t.RawSetString("to", lua.LString(to))
	t.RawSetString("cc", lua.LString(cc))
	t.RawSetString("subject", lua.LString(subject))
	t.RawSetString("account_id", lua.LString(accountID))

	for _, fn := range callbacks {
		if err := L.CallByParam(lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		}, t); err != nil {
			log.Printf("plugin hook %q error: %v", event, err)
		}
	}
}

// CallFolderHook calls a hook with a folder name.
func (m *Manager) CallFolderHook(event string, folderName string) {
	callbacks, ok := m.hooks[event]
	if !ok {
		return
	}

	for _, fn := range callbacks {
		if err := m.state.CallByParam(lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		}, lua.LString(folderName)); err != nil {
			log.Printf("plugin hook %q error: %v", event, err)
		}
	}
}

// CallComposerHook calls a hook with composer state info.
func (m *Manager) CallComposerHook(event string, body, subject, to string) {
	callbacks, ok := m.hooks[event]
	if !ok {
		return
	}

	L := m.state
	t := L.NewTable()
	t.RawSetString("body_len", lua.LNumber(len(body)))
	t.RawSetString("body", lua.LString(body))
	t.RawSetString("subject", lua.LString(subject))
	t.RawSetString("to", lua.LString(to))

	for _, fn := range callbacks {
		if err := L.CallByParam(lua.P{
			Fn:      fn,
			NRet:    0,
			Protect: true,
		}, t); err != nil {
			log.Printf("plugin hook %q error: %v", event, err)
		}
	}
}

// EmailToTable converts email fields into a Lua table.
func (m *Manager) EmailToTable(uid uint32, from string, to []string, subject string, date time.Time, isRead bool, accountID string, folder string) *lua.LTable {
	L := m.state

	t := L.NewTable()
	t.RawSetString("uid", lua.LNumber(uid))
	t.RawSetString("from", lua.LString(from))
	t.RawSetString("subject", lua.LString(subject))
	t.RawSetString("date", lua.LString(date.Format(time.RFC3339)))
	t.RawSetString("is_read", lua.LBool(isRead))
	t.RawSetString("account_id", lua.LString(accountID))
	t.RawSetString("folder", lua.LString(folder))

	toTable := L.NewTable()
	for i, addr := range to {
		toTable.RawSetInt(i+1, lua.LString(addr))
	}
	t.RawSetString("to", toTable)

	return t
}
