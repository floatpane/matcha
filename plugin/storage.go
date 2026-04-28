package plugin

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

var validPluginStoreName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type pluginStore struct {
	path string
	mu   sync.Mutex
	data map[string]string
}

func newPluginStore(pluginName string) (*pluginStore, error) {
	if !validPluginStoreName.MatchString(pluginName) {
		return nil, errors.New("invalid plugin name for storage")
	}

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

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".data-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.path)
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

func (m *Manager) currentStore() (*pluginStore, error) {
	if m.currentPlugin == "" {
		return nil, nil
	}
	if m.stores == nil {
		m.stores = make(map[string]*pluginStore)
	}
	if s, ok := m.stores[m.currentPlugin]; ok {
		return s, nil
	}

	s, err := newPluginStore(m.currentPlugin)
	if err != nil {
		return nil, err
	}
	m.stores[m.currentPlugin] = s
	return s, nil
}

func (m *Manager) luaStoreSet(L *lua.LState) int {
	key := L.CheckString(1)
	val := L.CheckString(2)

	s, err := m.currentStore()
	if err != nil {
		L.RaiseError("store_set: %v", err)
		return 0
	}
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

	s, err := m.currentStore()
	if err != nil {
		L.RaiseError("store_get: %v", err)
		return 0
	}
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

	s, err := m.currentStore()
	if err != nil {
		L.RaiseError("store_delete: %v", err)
		return 0
	}
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
	s, err := m.currentStore()
	if err != nil {
		L.RaiseError("store_keys: %v", err)
		return 0
	}
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
