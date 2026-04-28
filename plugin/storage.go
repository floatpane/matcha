package plugin

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

type pluginStore struct {
	path string
	mu   sync.Mutex
	data map[string]string
}

func newPluginStore(pluginName string) (*pluginStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	dir := filepath.Join(home, ".config", "matcha", "plugins", pluginName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	s := &pluginStore{
		path: filepath.Join(dir, "data.json"),
		data: map[string]string{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *pluginStore) load() error {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, &s.data); err != nil {
		return err
	}
	if s.data == nil {
		s.data = map[string]string{}
	}
	return nil
}

func (s *pluginStore) flush() error {
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *pluginStore) Get(k string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v, ok := s.data[k]
	return v, ok
}

func (s *pluginStore) Set(k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[k] = v
	return s.flush()
}

func (s *pluginStore) Delete(k string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, k)
	return s.flush()
}

func (s *pluginStore) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, 0, len(s.data))
	for k := range s.data {
		out = append(out, k)
	}
	return out
}

func (m *Manager) currentStore() *pluginStore {
	if m.currentPlugin == "" {
		return nil
	}
	if m.stores == nil {
		m.stores = make(map[string]*pluginStore)
	}
	if s, ok := m.stores[m.currentPlugin]; ok {
		return s
	}

	s, err := newPluginStore(m.currentPlugin)
	if err != nil {
		log.Printf("plugin %q: store init: %v", m.currentPlugin, err)
		return nil
	}
	m.stores[m.currentPlugin] = s
	return s
}

func (m *Manager) luaStoreSet(L *lua.LState) int {
	key := L.CheckString(1)
	val := L.CheckString(2)

	s := m.currentStore()
	if s == nil {
		L.RaiseError("store_set: no plugin context")
		return 0
	}
	if err := s.Set(key, val); err != nil {
		L.RaiseError("store_set: %v", err)
	}
	return 0
}

func (m *Manager) luaStoreGet(L *lua.LState) int {
	key := L.CheckString(1)

	s := m.currentStore()
	if s == nil {
		L.Push(lua.LNil)
		return 1
	}
	if v, ok := s.Get(key); ok {
		L.Push(lua.LString(v))
	} else {
		L.Push(lua.LNil)
	}
	return 1
}

func (m *Manager) luaStoreDelete(L *lua.LState) int {
	key := L.CheckString(1)

	s := m.currentStore()
	if s == nil {
		L.RaiseError("store_delete: no plugin context")
		return 0
	}
	if err := s.Delete(key); err != nil {
		L.RaiseError("store_delete: %v", err)
	}
	return 0
}

func (m *Manager) luaStoreKeys(L *lua.LState) int {
	s := m.currentStore()
	if s == nil {
		L.Push(L.NewTable())
		return 1
	}

	t := L.NewTable()
	for i, key := range s.Keys() {
		t.RawSetInt(i+1, lua.LString(key))
	}
	L.Push(t)
	return 1
}
