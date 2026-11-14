package tui

import (
	"fmt"
	"os"
	"testing"

	"github.com/floatpane/termimage"
)

// TestMain handles image decoder workers spawned by remote image tests and
// points HOME at a throwaway directory, so tests that save settings, drafts or
// caches never touch the developer's real ~/.config/matcha or ~/.cache/matcha.
func TestMain(m *testing.M) {
	termimage.MaybeRunWorker()

	home, err := os.MkdirTemp("", "matcha-tui-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create test home: %v\n", err)
		os.Exit(1)
	}
	_ = os.Setenv("HOME", home)
	for _, key := range []string{"XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		_ = os.Unsetenv(key)
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
