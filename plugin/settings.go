package plugin

import (
	"sort"

	lua "github.com/yuin/gopher-lua"
)

// SettingType identifies the kind of value a plugin setting holds.
type SettingType string

const (
	SettingBool   SettingType = "boolean"
	SettingNumber SettingType = "number"
	SettingString SettingType = "string"
)

// SettingDef describes a single configurable plugin setting.
type SettingDef struct {
	Key         string
	Type        SettingType
	Default     interface{}
	Label       string
	Description string
}

// PluginSettings holds the schema for one plugin's settings, ordered by
// declaration order so the TUI lists them predictably.
type PluginSettings struct {
	Plugin string
	Defs   []SettingDef
}

// declareSettings registers the schema for the currently-loading plugin and
// returns a Lua table whose __index reads the live current value for a key.
// Plugins typically capture the returned table in a local and read fields
// from it at any later time, including inside hook callbacks:
//
//	local cfg = matcha.settings({
//	    threshold = {type = "number", default = 5},
//	    enabled   = {type = "boolean", default = true},
//	})
//	matcha.on("email_received", function(email)
//	    if cfg.enabled and #email.subject > cfg.threshold then ... end
//	end)
func (m *Manager) declareSettings(L *lua.LState, spec *lua.LTable) int { //nolint:gocritic
	if m.currentPlugin == "" {
		L.RaiseError("matcha.settings() must be called from a plugin file")
		return 0
	}
	plugin := m.currentPlugin

	defs := []SettingDef{}
	keys := []string{}
	specs := map[string]SettingDef{}

	spec.ForEach(func(k, v lua.LValue) {
		key, ok := k.(lua.LString)
		if !ok {
			return
		}
		entry, ok := v.(*lua.LTable)
		if !ok {
			return
		}

		def := SettingDef{Key: string(key)}

		if t, ok := entry.RawGetString("type").(lua.LString); ok {
			def.Type = SettingType(string(t))
		}
		if l, ok := entry.RawGetString("label").(lua.LString); ok {
			def.Label = string(l)
		}
		if d, ok := entry.RawGetString("description").(lua.LString); ok {
			def.Description = string(d)
		}

		raw := entry.RawGetString("default")
		switch def.Type {
		case SettingBool:
			def.Default = lua.LVAsBool(raw)
		case SettingNumber:
			if n, ok := raw.(lua.LNumber); ok {
				def.Default = float64(n)
			} else {
				def.Default = float64(0)
			}
		case SettingString:
			if s, ok := raw.(lua.LString); ok {
				def.Default = string(s)
			} else {
				def.Default = ""
			}
		default:
			// Unknown type — skip.
			return
		}

		keys = append(keys, def.Key)
		specs[def.Key] = def
	})

	sort.Strings(keys)
	for _, k := range keys {
		defs = append(defs, specs[k])
	}

	m.pluginSchemas[plugin] = defs
	if _, ok := m.pluginValues[plugin]; !ok {
		m.pluginValues[plugin] = map[string]interface{}{}
	}

	proxy := L.NewTable()
	mt := L.NewTable()
	L.SetField(mt, "__index", L.NewFunction(func(L *lua.LState) int {
		key := L.CheckString(2)
		def, ok := m.findDef(plugin, key)
		if !ok {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(toLuaValue(L, m.lookupValue(plugin, key, def)))
		return 1
	}))
	L.SetField(mt, "__newindex", L.NewFunction(func(L *lua.LState) int {
		L.RaiseError("plugin settings table is read-only; edit values in the TUI settings")
		return 0
	}))
	L.SetMetatable(proxy, mt)
	L.Push(proxy)
	return 1
}

// getSetting returns the value of a setting for the currently-running plugin.
// During plugin load, "currently running" means the loading plugin; outside
// load (e.g. inside a hook callback), it falls back to the plugin that owns
// the running closure — for now we use currentPlugin and only allow lookups
// to the plugin that declared the schema by name.
func (m *Manager) getSetting(L *lua.LState) int { //nolint:gocritic
	plugin := m.currentPlugin
	key := L.CheckString(1)

	if plugin == "" {
		// Allow optional second argument: explicit plugin name.
		if L.GetTop() >= 2 {
			plugin = L.CheckString(2)
		}
	}

	def, ok := m.findDef(plugin, key)
	if !ok {
		L.Push(lua.LNil)
		return 1
	}

	val := m.lookupValue(plugin, key, def)
	L.Push(toLuaValue(L, val))
	return 1
}

func (m *Manager) findDef(plugin, key string) (SettingDef, bool) {
	for _, d := range m.pluginSchemas[plugin] {
		if d.Key == key {
			return d, true
		}
	}
	return SettingDef{}, false
}

func (m *Manager) lookupValue(plugin, key string, def SettingDef) interface{} {
	if vals, ok := m.pluginValues[plugin]; ok {
		if v, ok := vals[key]; ok {
			return v
		}
	}
	return def.Default
}

func toLuaValue(_ *lua.LState, v interface{}) lua.LValue {
	switch x := v.(type) {
	case bool:
		return lua.LBool(x)
	case float64:
		return lua.LNumber(x)
	case int:
		return lua.LNumber(x)
	case string:
		return lua.LString(x)
	default:
		return lua.LNil
	}
}

// Schemas returns all plugin setting schemas, sorted by plugin name.
func (m *Manager) Schemas() []PluginSettings {
	names := make([]string, 0, len(m.pluginSchemas))
	for name := range m.pluginSchemas {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]PluginSettings, 0, len(names))
	for _, n := range names {
		out = append(out, PluginSettings{Plugin: n, Defs: m.pluginSchemas[n]})
	}
	return out
}

// Schema returns the schema for a single plugin.
func (m *Manager) Schema(plugin string) []SettingDef {
	return m.pluginSchemas[plugin]
}

// GetSettingValue returns the current value (or default) for a plugin setting.
func (m *Manager) GetSettingValue(plugin, key string) (interface{}, bool) {
	def, ok := m.findDef(plugin, key)
	if !ok {
		return nil, false
	}
	return m.lookupValue(plugin, key, def), true
}

// SetSettingValue updates a plugin setting in-memory. Coerces value to the
// declared type. Returns false if the plugin/key is unknown.
func (m *Manager) SetSettingValue(plugin, key string, val interface{}) bool {
	def, ok := m.findDef(plugin, key)
	if !ok {
		return false
	}

	if _, ok := m.pluginValues[plugin]; !ok {
		m.pluginValues[plugin] = map[string]interface{}{}
	}
	m.pluginValues[plugin][key] = coerceValue(def.Type, val)
	return true
}

// LoadSettingValues replaces in-memory values with the given snapshot. Values
// for unknown plugins/keys are kept as-is so freshly-disabled plugins don't
// lose their saved settings on next launch.
func (m *Manager) LoadSettingValues(values map[string]map[string]interface{}) {
	if values == nil {
		return
	}
	for plugin, vals := range values {
		if _, ok := m.pluginValues[plugin]; !ok {
			m.pluginValues[plugin] = map[string]interface{}{}
		}
		for k, v := range vals {
			if def, ok := m.findDef(plugin, k); ok {
				m.pluginValues[plugin][k] = coerceValue(def.Type, v)
			} else {
				m.pluginValues[plugin][k] = v
			}
		}
	}
}

// AllSettingValues returns a deep copy of all plugin setting values.
func (m *Manager) AllSettingValues() map[string]map[string]interface{} {
	out := make(map[string]map[string]interface{}, len(m.pluginValues))
	for p, vals := range m.pluginValues {
		inner := make(map[string]interface{}, len(vals))
		for k, v := range vals {
			inner[k] = v
		}
		out[p] = inner
	}
	return out
}

func coerceValue(t SettingType, v interface{}) interface{} {
	switch t {
	case SettingBool:
		switch x := v.(type) {
		case bool:
			return x
		case string:
			return x == "true"
		case float64:
			return x != 0
		}
		return false
	case SettingNumber:
		switch x := v.(type) {
		case float64:
			return x
		case int:
			return float64(x)
		case bool:
			if x {
				return float64(1)
			}
			return float64(0)
		case string:
			return float64(0)
		}
		return float64(0)
	case SettingString:
		switch x := v.(type) {
		case string:
			return x
		case float64:
			return ""
		case bool:
			if x {
				return "true"
			}
			return "false"
		}
		return ""
	}
	return v
}
