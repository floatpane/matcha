package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestLuaStoreRoundTrip(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()
	m.currentPlugin = "test_plugin"

	err := m.state.DoString(`
		local matcha = require("matcha")
		matcha.store_set("token", "abc123")
		result = matcha.store_get("token")
	`)
	if err != nil {
		t.Fatal(err)
	}

	if got := m.state.GetGlobal("result"); got.String() != "abc123" {
		t.Fatalf("expected abc123, got %q", got.String())
	}
}

func TestLuaStoreSetWithoutPluginContext(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()

	err := m.state.DoString(`
		local matcha = require("matcha")
		matcha.store_set("token", "abc123")
	`)
	if err == nil {
		t.Fatal("expected store_set to fail without plugin context")
	}
	if !strings.Contains(err.Error(), "no plugin context") {
		t.Fatalf("expected plugin context error, got %v", err)
	}
}

// store_delete is intentionally a silent no-op outside a plugin context to
// match store_get's read-side behavior. Only store_set raises in that case.
func TestLuaStoreDeleteWithoutPluginContextIsNoOp(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()

	err := m.state.DoString(`
		local matcha = require("matcha")
		matcha.store_delete("token")
	`)
	if err != nil {
		t.Fatalf("expected store_delete to be silent without plugin context, got %v", err)
	}
}

func TestLuaStorePluginsAreIsolated(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()

	pluginA := writePlugin(t, t.TempDir(), "a.lua", `
		local matcha = require("matcha")
		matcha.store_set("shared", "a")
	`)
	pluginB := writePlugin(t, t.TempDir(), "b.lua", `
		local matcha = require("matcha")
		matcha.store_set("shared", "b")
	`)

	m.loadPlugin("plugin_a", pluginA)
	m.loadPlugin("plugin_b", pluginB)

	storeA, err := newPluginStore("plugin_a")
	if err != nil {
		t.Fatal(err)
	}
	storeB, err := newPluginStore("plugin_b")
	if err != nil {
		t.Fatal(err)
	}

	gotA, ok := storeA.Get("shared")
	if !ok {
		t.Fatal("expected plugin_a key")
	}
	gotB, ok := storeB.Get("shared")
	if !ok {
		t.Fatal("expected plugin_b key")
	}
	if gotA != "a" {
		t.Fatalf("expected plugin_a value a, got %q", gotA)
	}
	if gotB != "b" {
		t.Fatalf("expected plugin_b value b, got %q", gotB)
	}
}

func TestLuaStoreHookUsesRegisteredPluginContext(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()

	pluginA := writePlugin(t, t.TempDir(), "a.lua", `
		local matcha = require("matcha")
		matcha.on("startup", function()
			matcha.store_set("hook", "a")
		end)
	`)
	pluginB := writePlugin(t, t.TempDir(), "b.lua", `
		local matcha = require("matcha")
		matcha.on("startup", function()
			matcha.store_set("hook", "b")
		end)
	`)

	m.loadPlugin("plugin_a", pluginA)
	m.loadPlugin("plugin_b", pluginB)
	m.CallHook(HookStartup)

	assertStoredValue(t, "plugin_a", "hook", "a")
	assertStoredValue(t, "plugin_b", "hook", "b")
}

func TestLuaStoreKeyBindingUsesRegisteredPluginContext(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()

	pluginA := writePlugin(t, t.TempDir(), "a.lua", `
		local matcha = require("matcha")
		matcha.bind_key("ctrl+a", "inbox", "A", function()
			matcha.store_set("binding", "a")
		end)
	`)
	pluginB := writePlugin(t, t.TempDir(), "b.lua", `
		local matcha = require("matcha")
		matcha.bind_key("ctrl+b", "inbox", "B", function()
			matcha.store_set("binding", "b")
		end)
	`)

	m.loadPlugin("plugin_a", pluginA)
	m.loadPlugin("plugin_b", pluginB)

	bindings := m.Bindings(StatusInbox)
	if len(bindings) != 2 {
		t.Fatalf("expected 2 bindings, got %d", len(bindings))
	}
	for _, binding := range bindings {
		m.CallKeyBinding(binding)
	}

	assertStoredValue(t, "plugin_a", "binding", "a")
	assertStoredValue(t, "plugin_b", "binding", "b")
}

func TestLuaStoreKeysAndDelete(t *testing.T) {
	setTestHome(t)

	m := newTestManager()
	defer m.Close()
	m.currentPlugin = "test_plugin"

	err := m.state.DoString(`
		local matcha = require("matcha")
		matcha.store_set("a", "1")
		matcha.store_set("b", "2")
		matcha.store_delete("a")
		keys = matcha.store_keys()
		deleted = matcha.store_get("a")
	`)
	if err != nil {
		t.Fatal(err)
	}

	if got := m.state.GetGlobal("deleted"); got != lua.LNil {
		t.Fatalf("expected deleted key to be nil, got %v", got)
	}

	keys, ok := m.state.GetGlobal("keys").(*lua.LTable)
	if !ok {
		t.Fatalf("expected keys table")
	}
	if keys.Len() != 1 {
		t.Fatalf("expected 1 key, got %d", keys.Len())
	}
	if got := keys.RawGetInt(1); got.String() != "b" {
		t.Fatalf("expected remaining key b, got %q", got.String())
	}
}

func writePlugin(t *testing.T, dir, name, body string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertStoredValue(t *testing.T, pluginName, key, want string) {
	t.Helper()

	store, err := newPluginStore(pluginName)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := store.Get(key)
	if !ok {
		t.Fatalf("expected %s key %q", pluginName, key)
	}
	if got != want {
		t.Fatalf("expected %s key %q to be %q, got %q", pluginName, key, want, got)
	}
}
