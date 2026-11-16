// Package testhome keeps tests away from the developer's real matcha
// configuration, drafts and caches.
package testhome

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Isolate points the home directory at a fresh temporary directory: HOME,
// and USERPROFILE, APPDATA and LOCALAPPDATA, which Windows uses instead. The
// XDG directories are cleared. cleanup removes the directory.
func Isolate() (cleanup func(), err error) {
	home, err := os.MkdirTemp("", "matcha-test-home-*")
	if err != nil {
		return nil, err
	}
	for key, value := range map[string]string{
		"HOME":         home,
		"USERPROFILE":  home,
		"APPDATA":      filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA": filepath.Join(home, "AppData", "Local"),
	} {
		_ = os.Setenv(key, value)
	}
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		_ = os.Unsetenv(key)
	}
	return func() { _ = os.RemoveAll(home) }, nil
}

// Main runs a package's tests with an isolated home directory and exits.
// Call it from TestMain.
func Main(m *testing.M) {
	cleanup, err := Isolate()
	if err != nil {
		fmt.Fprintf(os.Stderr, "isolate test home: %v\n", err)
		os.Exit(1)
	}
	code := m.Run()
	cleanup()
	os.Exit(code)
}
