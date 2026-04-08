package plugin

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// KeyBinding represents a plugin-registered keyboard shortcut.
type KeyBinding struct {
	Key         string
	Area        string // "inbox", "email_view", or "composer"
	Description string
	Fn          *lua.LFunction
}

// Manager manages the Lua VM and loaded plugins.
type Manager struct {
	state   *lua.LState
	hooks   map[string][]*lua.LFunction
	plugins []string
	// statuses holds persistent status strings per view area, shown in the UI.
	statuses map[string]string
	// pendingNotification is set by matcha.notify() and consumed by the orchestrator.
	pendingNotification string
	pendingDuration     float64 // seconds, 0 means default (2s)
	// pendingFields holds compose field updates set by matcha.set_compose_field().
	pendingFields map[string]string
	// bindings holds plugin-registered keyboard shortcuts.
	bindings []KeyBinding
	// pendingPrompt is set by matcha.prompt() and consumed by the orchestrator.
	pendingPrompt *PendingPrompt
}

// NewManager creates a new plugin manager with a Lua VM.
func NewManager() *Manager {
	m := &Manager{
		hooks:         make(map[string][]*lua.LFunction),
		statuses:      make(map[string]string),
		pendingFields: make(map[string]string),
	}

	L := lua.NewState(lua.Options{
		SkipOpenLibs: true,
	})

	// Open only safe standard libraries (no os, io, debug)
	for _, lib := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.LoadLibName, lua.OpenPackage},
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(lib.fn))
		L.Push(lua.LString(lib.name))
		L.Call(1, 0)
	}

	m.state = L
	m.registerAPI()

	return m
}

// LoadPlugins discovers and loads plugins from ~/.config/matcha/plugins/.
func (m *Manager) LoadPlugins() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	pluginsDir := filepath.Join(home, ".config", "matcha", "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		path := filepath.Join(pluginsDir, entry.Name())

		if entry.IsDir() {
			// Directory plugin: look for init.lua
			initPath := filepath.Join(path, "init.lua")
			if _, err := os.Stat(initPath); err == nil {
				m.loadPlugin(entry.Name(), initPath)
			}
		} else if strings.HasSuffix(entry.Name(), ".lua") {
			// Single-file plugin
			name := strings.TrimSuffix(entry.Name(), ".lua")
			m.loadPlugin(name, path)
		}
	}
}

func (m *Manager) loadPlugin(name, path string) {
	if err := m.state.DoFile(path); err != nil {
		log.Printf("plugin %q: load error: %v", name, err)
		return
	}
	m.plugins = append(m.plugins, name)
	log.Printf("plugin %q: loaded", name)
}

// Plugins returns the names of all loaded plugins.
func (m *Manager) Plugins() []string {
	return m.plugins
}

// PendingNotification holds a notification message and its display duration.
type PendingNotification struct {
	Message  string
	Duration float64 // seconds, 0 means default
}

// TakePendingNotification returns and clears any pending notification.
func (m *Manager) TakePendingNotification() (PendingNotification, bool) {
	if m.pendingNotification == "" {
		return PendingNotification{}, false
	}
	n := PendingNotification{
		Message:  m.pendingNotification,
		Duration: m.pendingDuration,
	}
	m.pendingNotification = ""
	m.pendingDuration = 0
	return n, true
}

// TakePendingFields returns and clears any pending compose field updates.
func (m *Manager) TakePendingFields() map[string]string {
	if len(m.pendingFields) == 0 {
		return nil
	}
	fields := m.pendingFields
	m.pendingFields = make(map[string]string)
	return fields
}

// Bindings returns all plugin-registered key bindings for the given view area.
func (m *Manager) Bindings(area string) []KeyBinding {
	var result []KeyBinding
	for _, b := range m.bindings {
		if b.Area == area {
			result = append(result, b)
		}
	}
	return result
}

// StatusText returns the plugin status string for the given view area.
func (m *Manager) StatusText(area string) string {
	return m.statuses[area]
}

// LuaState returns the Lua VM state for building tables.
func (m *Manager) LuaState() *lua.LState {
	return m.state
}

// Close shuts down the Lua VM.
func (m *Manager) Close() {
	if m.state != nil {
		m.state.Close()
	}
}
