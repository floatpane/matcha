package plugin

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/floatpane/matcha/internal/loglevel"
	lua "github.com/yuin/gopher-lua"
)

// KeyBinding represents a plugin-registered keyboard shortcut.
type KeyBinding struct {
	Key         string
	Area        string // "inbox", "email_view", or "composer"
	Description string
	Fn          *lua.LFunction
	Plugin      string
}

// FlagOp is a pending flag change queued by a plugin via matcha.mark_read / matcha.mark_unread.
type FlagOp struct {
	UID       uint32
	AccountID string
	Folder    string
	Read      bool // true = mark read, false = mark unread
}

// Manager manages the Lua VM and loaded plugins.
//
// Manager is not safe for concurrent use. The Lua VM itself is single-
// threaded, and all hook callbacks, key-binding invocations, and API calls
// must be dispatched from the same goroutine that owns the Manager (the
// orchestrator). Mutable Manager state (hooks, stores, bindings,
// currentPlugin, pending* fields) is therefore unprotected by design; callers
// that need to drive plugin events from multiple goroutines must serialize
// access externally.
type Manager struct {
	state         *lua.LState
	hooks         map[string][]registeredHook
	plugins       []string
	currentPlugin string
	stores        map[string]*pluginStore
	// statuses holds persistent status strings per view area, shown in the UI.
	statuses map[string]string
	// pendingNotification is set by matcha.notify() and consumed by the orchestrator.
	pendingNotification *PendingNotification
	// pendingLoadNotifications holds plugin load errors queued for display.
	// They are drained one-at-a-time into pendingNotification by the
	// orchestrator so that multiple failed plugins don't get lost when the
	// single-slot pendingNotification is already occupied.
	pendingLoadNotifications []*PendingNotification
	// pendingFields holds compose field updates set by matcha.set_compose_field().
	pendingFields map[string]string
	// bindings holds plugin-registered keyboard shortcuts.
	bindings []KeyBinding
	// pendingPrompt is set by matcha.prompt() and consumed by the orchestrator.
	pendingPrompt *PendingPrompt
	// pendingFlagOps queues flag changes (read/unread) requested by plugins.
	pendingFlagOps []FlagOp
	// suppressAutoRead is set by matcha.suppress_auto_read() inside email_viewed callbacks.
	suppressAutoRead bool

	// pluginSchemas holds settings declarations per plugin.
	pluginSchemas map[string][]SettingDef
	// pluginValues holds current setting values per plugin.
	pluginValues map[string]map[string]interface{}

	// ui holds all plugin-driven UI customization state: text overrides,
	// component visibility toggles, custom injected components, and the
	// startup banner override. See ui.go for details.
	ui uiState
}

// NewManager creates a new plugin manager with a Lua VM.
func NewManager() *Manager {
	m := &Manager{
		hooks:         make(map[string][]registeredHook),
		statuses:      make(map[string]string),
		pendingFields: make(map[string]string),
		pluginSchemas: make(map[string][]SettingDef),
		pluginValues:  make(map[string]map[string]interface{}),
	}
	m.initUI()

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
	previousPlugin := m.currentPlugin
	m.currentPlugin = name
	defer func() {
		m.currentPlugin = previousPlugin
	}()

	if err := m.state.DoFile(path); err != nil {
		log.Printf("plugin %q: load error: %v", name, err)
		m.queueLoadNotification(name, err)
		return
	}
	m.plugins = append(m.plugins, name)
	loglevel.Verbosef("plugin %q: loaded", name)
}

// Plugins returns the names of all loaded plugins.
func (m *Manager) Plugins() []string {
	return m.plugins
}

// NotifyKind classifies a plugin notification for visual styling.
type NotifyKind string

const (
	NotifyKindInfo    NotifyKind = "info"
	NotifyKindWarning NotifyKind = "warning"
	NotifyKindError   NotifyKind = "error"
)

// PendingNotification holds a notification requested by a plugin via
// matcha.notify(). It is consumed once by the orchestrator via
// TakePendingNotification().
type PendingNotification struct {
	Message  string
	Title    string
	Duration float64 // seconds, 0 means default (2s)
	Kind     NotifyKind
	Closable bool // true = dismissible with a key press; false = auto-close only
}

// TakePendingNotification returns and clears any pending notification.
func (m *Manager) TakePendingNotification() (*PendingNotification, bool) {
	if m.pendingNotification == nil && len(m.pendingLoadNotifications) > 0 {
		m.pendingNotification = m.pendingLoadNotifications[0]
		m.pendingLoadNotifications = m.pendingLoadNotifications[1:]
	}
	if m.pendingNotification == nil {
		return nil, false
	}
	n := m.pendingNotification
	m.pendingNotification = nil
	return n, true
}

// queueLoadNotification records a plugin load error as a non-blocking
// notification so the orchestrator can surface it in the UI via the
// bubble-overlay notification system. Multiple failures are queued and
// drained one-at-a-time by TakePendingNotification.
func (m *Manager) queueLoadNotification(name string, err error) {
	m.pendingLoadNotifications = append(m.pendingLoadNotifications, &PendingNotification{
		Title:    "Plugin load error",
		Message:  fmt.Sprintf("%s: %v", name, err),
		Duration: 6,
		Kind:     NotifyKindError,
		Closable: true,
	})
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

// TakePendingFlagOps returns and clears all pending flag operations.
func (m *Manager) TakePendingFlagOps() []FlagOp {
	if len(m.pendingFlagOps) == 0 {
		return nil
	}
	ops := m.pendingFlagOps
	m.pendingFlagOps = nil
	return ops
}

// TakeAutoReadSuppressed returns true (and resets the flag) if a plugin
// called matcha.suppress_auto_read() during the current email_viewed callback.
func (m *Manager) TakeAutoReadSuppressed() bool {
	v := m.suppressAutoRead
	m.suppressAutoRead = false
	return v
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
