package plugin

import (
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// declareForTest mimics what loadPlugin does so we can drive declareSettings
// without writing a real plugin file to disk.
func declareForTest(t *testing.T, m *Manager, name, src string) {
	t.Helper()
	prev := m.currentPlugin
	m.currentPlugin = name
	defer func() { m.currentPlugin = prev }()
	if err := m.state.DoString(src); err != nil {
		t.Fatalf("Lua error loading %q: %v", name, err)
	}
}

func TestPluginSettingsSchemaAndProxy(t *testing.T) {
	m := NewManager()
	defer m.Close()

	src := `
		local matcha = require("matcha")
		cfg = matcha.settings({
			enabled = {type = "boolean", default = true, label = "Enabled"},
			limit   = {type = "number",  default = 5,    label = "Limit"},
			suffix  = {type = "string",  default = "!"},
		})
	`
	declareForTest(t, m, "demo", src)

	defs := m.Schema("demo")
	if len(defs) != 3 {
		t.Fatalf("expected 3 defs, got %d", len(defs))
	}

	v, ok := m.GetSettingValue("demo", "enabled")
	if !ok || v.(bool) != true {
		t.Fatalf("expected default enabled=true, got %v ok=%v", v, ok)
	}
	v, _ = m.GetSettingValue("demo", "limit")
	if v.(float64) != 5 {
		t.Fatalf("expected default limit=5, got %v", v)
	}

	// proxy table should reflect live values
	check := `
		assert(cfg.enabled == true,  "enabled default")
		assert(cfg.limit == 5,        "limit default")
		assert(cfg.suffix == "!",     "suffix default")
	`
	if err := m.state.DoString(check); err != nil {
		t.Fatalf("proxy check failed: %v", err)
	}

	// Override via SetSettingValue and re-check via proxy
	if !m.SetSettingValue("demo", "enabled", false) {
		t.Fatal("SetSettingValue rejected known key")
	}
	if !m.SetSettingValue("demo", "limit", float64(42)) {
		t.Fatal("SetSettingValue rejected number")
	}
	if !m.SetSettingValue("demo", "suffix", "?!") {
		t.Fatal("SetSettingValue rejected string")
	}

	check = `
		assert(cfg.enabled == false, "enabled override")
		assert(cfg.limit == 42,       "limit override")
		assert(cfg.suffix == "?!",    "suffix override")
	`
	if err := m.state.DoString(check); err != nil {
		t.Fatalf("proxy override check failed: %v", err)
	}
}

func TestPluginSettingsLoadValues(t *testing.T) {
	m := NewManager()
	defer m.Close()

	declareForTest(t, m, "demo", `
		local matcha = require("matcha")
		cfg = matcha.settings({
			enabled = {type = "boolean", default = true},
			limit   = {type = "number",  default = 5},
		})
	`)

	// Simulate loading values from config (JSON unmarshals booleans as bool,
	// numbers as float64).
	m.LoadSettingValues(map[string]map[string]interface{}{
		"demo": {
			"enabled": false,
			"limit":   float64(99),
		},
	})

	v, _ := m.GetSettingValue("demo", "enabled")
	if v.(bool) != false {
		t.Fatalf("expected enabled=false after load, got %v", v)
	}
	v, _ = m.GetSettingValue("demo", "limit")
	if v.(float64) != 99 {
		t.Fatalf("expected limit=99 after load, got %v", v)
	}

	// AllSettingValues should round-trip through JSON-friendly types.
	all := m.AllSettingValues()
	if all["demo"]["enabled"] != false || all["demo"]["limit"].(float64) != 99 {
		t.Fatalf("AllSettingValues mismatch: %#v", all)
	}
}

func TestPluginSettingsProxyReadOnly(t *testing.T) {
	m := NewManager()
	defer m.Close()

	declareForTest(t, m, "demo", `
		local matcha = require("matcha")
		cfg = matcha.settings({enabled = {type = "boolean", default = true}})
	`)

	err := m.state.DoString(`cfg.enabled = false`)
	if err == nil {
		t.Fatal("expected error writing to read-only proxy")
	}
}

func TestPluginSettingsRequiresLoadingPlugin(t *testing.T) {
	m := NewManager()
	defer m.Close()

	// currentPlugin is empty (no loadPlugin in flight)
	err := m.state.DoString(`require("matcha").settings({foo = {type = "boolean", default = false}})`)
	if err == nil {
		t.Fatal("expected error when calling matcha.settings outside plugin load")
	}
}

func TestPluginSettingsCoercion(t *testing.T) {
	m := NewManager()
	defer m.Close()

	declareForTest(t, m, "demo", `
		local matcha = require("matcha")
		matcha.settings({
			flag = {type = "boolean", default = false},
			n    = {type = "number",  default = 0},
		})
	`)

	// Strings from a JSON file or older config should coerce.
	m.LoadSettingValues(map[string]map[string]interface{}{
		"demo": {
			"flag": "true",
			"n":    "ignored",
		},
	})

	v, _ := m.GetSettingValue("demo", "flag")
	if v.(bool) != true {
		t.Fatalf("expected flag=true after coercion, got %v", v)
	}
	v, _ = m.GetSettingValue("demo", "n")
	if _, ok := v.(float64); !ok {
		t.Fatalf("expected n coerced to float64, got %T %v", v, v)
	}
}

// Verify that hook callbacks see live values (regression test for the closure
// capture pattern).
func TestPluginSettingsAccessibleFromHook(t *testing.T) {
	m := NewManager()
	defer m.Close()

	declareForTest(t, m, "demo", `
		local matcha = require("matcha")
		local cfg = matcha.settings({n = {type = "number", default = 1}})
		seen = nil
		matcha.on("custom", function() seen = cfg.n end)
	`)

	// Fire hook before any override
	m.CallHook("custom")
	v := m.state.GetGlobal("seen")
	if n, ok := v.(lua.LNumber); !ok || float64(n) != 1 {
		t.Fatalf("expected seen=1, got %v", v)
	}

	// Override and fire again
	m.SetSettingValue("demo", "n", float64(7))
	m.CallHook("custom")
	v = m.state.GetGlobal("seen")
	if n, ok := v.(lua.LNumber); !ok || float64(n) != 7 {
		t.Fatalf("expected seen=7 after override, got %v", v)
	}
}
