package plugin

import (
	"strings"
	"sync"
)

// ComponentPosition describes where a plugin-injected component should appear
// relative to the main TUI grid. Floating components overlay existing content
// without affecting layout; the anchored variants insert the component into the
// grid flow and shrink the remaining panes accordingly.
type ComponentPosition string

const (
	// PosFloating renders the component on top of the existing layout as an
	// overlay. It does not change the flexbox/grid dimensions of the built-in
	// panes.
	PosFloating ComponentPosition = "floating"
	// PosTopLeft inserts the component at the top of the layout, pushing the
	// main content area down.
	PosTopLeft ComponentPosition = "top_left"
	// PosTopRight inserts the component at the top-right of the layout.
	PosTopRight ComponentPosition = "top_right"
	// PosBottomLeft inserts the component at the bottom of the layout, above
	// the help bar.
	PosBottomLeft ComponentPosition = "bottom_left"
	// PosBottomRight inserts the component at the bottom-right of the layout.
	PosBottomRight ComponentPosition = "bottom_right"
)

// CustomComponent is a plugin-injected UI element. Plugins register components
// through matcha.ui.add_component(id, content, position); the TUI renderer
// retrieves them via Manager.Components().
type CustomComponent struct {
	ID       string
	Content  string
	Position ComponentPosition
	Plugin   string
}

// componentName identifies a major UI pane that can be toggled on or off.
type componentName string

const (
	CompSidebar   componentName = "sidebar"
	CompStatusBar componentName = "status_bar"
	CompHeader    componentName = "header"
)

// validComponents is the set of component names accepted by set_visible.
var validComponents = map[componentName]bool{
	CompSidebar:   true,
	CompStatusBar: true,
	CompHeader:    true,
}

// validPositions is the set of position strings accepted by add_component.
var validPositions = map[ComponentPosition]bool{
	PosFloating:    true,
	PosTopLeft:     true,
	PosTopRight:    true,
	PosBottomLeft:  true,
	PosBottomRight: true,
}

// uiState holds all plugin-driven UI customization. It is embedded in Manager
// and accessed by the TUI layer through the getter methods below.
//
// All fields are guarded by mu because, unlike the rest of Manager's mutable
// state (which is single-goroutine by contract), the TUI render loop reads these
// fields from the Bubble Tea goroutine while a plugin hook callback executing on
// the same goroutine may write to them between frames. The mutex is kept in the
// uiState struct so that the zero-value Manager (used in tests that don't touch
// UI APIs) never dereferences a nil lock.
type uiState struct {
	mu sync.RWMutex

	// textOverrides maps i18n keys (e.g. "folder_inbox.folders_title") to
	// replacement strings. When the TUI calls t(key), the helper checks this
	// map first; if present, the override wins over the locale translation.
	textOverrides map[string]string

	// visibility tracks whether major UI panes are shown. A missing key
	// defaults to visible (true), so plugins only need to act when they want
	// to hide something.
	visibility map[componentName]bool

	// components is the registry of plugin-injected custom components,
	// keyed by ID. Re-adding with the same ID replaces the previous content.
	components map[string]CustomComponent

	// customBanner, when non-empty, replaces the default ASCII logo on the
	// startup / choice screen. Plugins set it via matcha.ui.set_banner().
	customBanner string
}

// initUI lazily initialises the UI state maps. Called from NewManager so the
// fields are always non-nil before any Lua code runs.
func (m *Manager) initUI() {
	m.ui.textOverrides = make(map[string]string)
	m.ui.visibility = make(map[componentName]bool)
	m.ui.components = make(map[string]CustomComponent)
}

// --- Text overrides -------------------------------------------------------

// SetTextOverride stores a plugin-provided string for the given i18n key. The
// TUI layer's t() helper calls TextOverride to check for a replacement before
// falling back to the locale bundle.
func (m *Manager) SetTextOverride(key, value string) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()
	m.ui.textOverrides[key] = value
}

// TextOverride returns the plugin override for key, if any. The ok result is
// false when no plugin has overridden the key (caller should use the locale
// translation).
func (m *Manager) TextOverride(key string) (string, bool) {
	m.ui.mu.RLock()
	defer m.ui.mu.RUnlock()
	v, ok := m.ui.textOverrides[key]
	return v, ok
}

// ClearTextOverride removes a single override. A no-op if the key was never set.
func (m *Manager) ClearTextOverride(key string) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()
	delete(m.ui.textOverrides, key)
}

// --- Component visibility -------------------------------------------------

// SetComponentVisible toggles the visibility of a major UI pane. name must be
// one of "sidebar", "status_bar", "header".
func (m *Manager) SetComponentVisible(name string, visible bool) bool {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()
	cn := componentName(strings.ToLower(name))
	if !validComponents[cn] {
		return false
	}
	m.ui.visibility[cn] = visible
	return true
}

// IsComponentVisible reports whether the named pane should be drawn. Returns
// true (visible) for any pane that has not been explicitly hidden, so the
// default behaviour is unchanged when no plugin touches visibility.
func (m *Manager) IsComponentVisible(name string) bool {
	m.ui.mu.RLock()
	defer m.ui.mu.RUnlock()
	cn := componentName(strings.ToLower(name))
	if !validComponents[cn] {
		return true // unknown components default to visible
	}
	v, ok := m.ui.visibility[cn]
	if !ok {
		return true
	}
	return v
}

// --- Custom components ----------------------------------------------------

// AddComponent registers or replaces a custom component. Returns false if the
// position string is invalid.
func (m *Manager) AddComponent(id, content, position string) bool {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()
	pos := ComponentPosition(strings.ToLower(position))
	if !validPositions[pos] {
		return false
	}
	m.ui.components[id] = CustomComponent{
		ID:       id,
		Content:  content,
		Position: pos,
		Plugin:   m.currentPlugin,
	}
	return true
}

// RemoveComponent deletes a custom component by ID.
func (m *Manager) RemoveComponent(id string) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()
	delete(m.ui.components, id)
}

// Components returns a snapshot of all registered custom components. The TUI
// renderer calls this once per frame; modifications after the snapshot is taken
// do not affect the returned slice.
func (m *Manager) Components() []CustomComponent {
	m.ui.mu.RLock()
	defer m.ui.mu.RUnlock()
	out := make([]CustomComponent, 0, len(m.ui.components))
	for _, c := range m.ui.components {
		out = append(out, c)
	}
	return out
}

// --- Banner override ------------------------------------------------------

// SetBanner replaces the default ASCII logo on the startup / choice screen.
// An empty string restores the default logo.
func (m *Manager) SetBanner(banner string) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()
	m.ui.customBanner = banner
}

// CustomBanner returns the plugin-provided banner string, or empty if no plugin
// has set one. The caller (Choice.View / Status.View) checks for a non-empty
// return to decide whether to bypass the default logo.
func (m *Manager) CustomBanner() string {
	m.ui.mu.RLock()
	defer m.ui.mu.RUnlock()
	return m.ui.customBanner
}

// --- Lua state safety note ------------------------------------------------
//
// The Lua bridge functions in api.go (luaUISetText, luaUISetVisible,
// luaUIAddComponent, luaUISetBanner) run on the same goroutine that owns the
// Manager — either during plugin load (DoFile) or inside a hook callback
// dispatched from the Bubble Tea Update loop. The uiState mutex nonetheless
// protects the maps because the TUI's View() method may execute concurrently
// with hook dispatch in edge cases (e.g. async command results), and a race
// on the map header would crash the program. The critical sections are tiny
// (a single map read/write), so contention is negligible.
