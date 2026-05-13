package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// setTestHome makes t.TempDir() the effective home directory for the duration
// of the test on both Unix and Windows. Go's os.UserHomeDir() reads $HOME on
// Unix but %USERPROFILE% on Windows, so we set both.
func setTestHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", dir)
	}
	return dir
}

func TestPluginStoreSetGet(t *testing.T) {
	setTestHome(t)

	store, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}

	if err := store.Set("token", "abc123"); err != nil {
		t.Fatal(err)
	}

	got, ok := store.Get("token")
	if !ok {
		t.Fatal("expected stored key")
	}
	if got != "abc123" {
		t.Fatalf("expected abc123, got %q", got)
	}
}

func TestPluginStoreDelete(t *testing.T) {
	setTestHome(t)

	store, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("token", "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("token"); err != nil {
		t.Fatal(err)
	}

	if got, ok := store.Get("token"); ok {
		t.Fatalf("expected key to be deleted, got %q", got)
	}
}

func TestPluginStoreKeys(t *testing.T) {
	setTestHome(t)

	store, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("b", "2"); err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, key := range store.Keys() {
		got[key] = true
	}

	want := map[string]bool{"a": true, "b": true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected keys %v, got %v", want, got)
	}
}

func TestPluginStoreKeysSortedOrder(t *testing.T) {
	setTestHome(t)

	store, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}
	// Insert in non-sorted order so map iteration order won't accidentally
	// produce the expected result.
	for _, k := range []string{"c", "a", "b", "z", "m"} {
		if err := store.Set(k, k); err != nil {
			t.Fatal(err)
		}
	}

	got := store.Keys()
	want := []string{"a", "b", "c", "m", "z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected sorted keys %v, got %v", want, got)
	}
}

func TestPluginStoreKeysEmpty(t *testing.T) {
	setTestHome(t)

	store, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}

	if keys := store.Keys(); len(keys) != 0 {
		t.Fatalf("expected no keys, got %v", keys)
	}
}

func TestPluginStoreConcurrentSets(t *testing.T) {
	setTestHome(t)

	store, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := store.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i)); err != nil {
				t.Errorf("set key%d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < 20; i++ {
		key := fmt.Sprintf("key%d", i)
		want := fmt.Sprintf("value%d", i)
		got, ok := store.Get(key)
		if !ok {
			t.Fatalf("expected %s to be stored", key)
		}
		if got != want {
			t.Fatalf("expected %s, got %q", want, got)
		}
	}
}

func TestPluginStorePersistence(t *testing.T) {
	setTestHome(t)

	store, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("token", "abc123"); err != nil {
		t.Fatal(err)
	}

	reloaded, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}

	got, ok := reloaded.Get("token")
	if !ok {
		t.Fatal("expected persisted key")
	}
	if got != "abc123" {
		t.Fatalf("expected abc123, got %q", got)
	}
}

func TestPluginStoreFileMode(t *testing.T) {
	setTestHome(t)

	store, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("token", "abc123"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("expected mode 0600, got %o", got)
		}
	}
}

func TestPluginStoreFileModeAfterOverwrite(t *testing.T) {
	setTestHome(t)

	store, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("token", "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.path, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("token", "def456"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("expected mode 0600 after overwrite, got %o", got)
		}
	}
}

func TestNewPluginStoreRejectsInvalidPluginName(t *testing.T) {
	setTestHome(t)

	for _, name := range []string{"", ".", "..", "../etc", "foo/bar", `foo\bar`, "foo.bar"} {
		t.Run(name, func(t *testing.T) {
			if _, err := newPluginStore(name); err == nil {
				t.Fatal("expected invalid plugin name error")
			}
		})
	}
}

func TestLuaStoreInitErrorPropagates(t *testing.T) {
	home := setTestHome(t)

	dir := filepath.Join(home, ".config", "matcha", "plugins", "test_plugin")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := newTestManager()
	defer m.Close()
	m.currentPlugin = "test_plugin"

	err := m.state.DoString(`
		local matcha = require("matcha")
		matcha.store_get("token")
	`)
	if err == nil {
		t.Fatal("expected store_get to fail on store init error")
	}
	if !strings.Contains(err.Error(), "store_get:") {
		t.Fatalf("expected store_get error, got %v", err)
	}
}
