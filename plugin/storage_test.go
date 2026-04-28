package plugin

import (
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
)

func TestPluginStoreSetGet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
	t.Setenv("HOME", t.TempDir())

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
	t.Setenv("HOME", t.TempDir())

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

func TestPluginStoreKeysEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	store, err := newPluginStore("test_plugin")
	if err != nil {
		t.Fatal(err)
	}

	if keys := store.Keys(); len(keys) != 0 {
		t.Fatalf("expected no keys, got %v", keys)
	}
}

func TestPluginStoreConcurrentSets(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
	t.Setenv("HOME", t.TempDir())

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
	t.Setenv("HOME", t.TempDir())

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
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected mode 0600, got %o", got)
	}
}
