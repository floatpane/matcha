package fetcher

import (
	"fmt"
	"os"
	"testing"
)

// TestMain points HOME (and the XDG directories) at a throwaway directory so
// the tests never read or write the developer's real matcha config and
// caches under ~/.config/matcha and ~/.cache/matcha.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "matcha-test-home-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create test home: %v\n", err)
		os.Exit(1)
	}
	for _, key := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME"} {
		if key == "HOME" {
			_ = os.Setenv(key, home)
		} else {
			_ = os.Unsetenv(key)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}
