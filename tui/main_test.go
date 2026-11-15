package tui

import (
	"os"
	"testing"

	"github.com/floatpane/matcha/internal/testhome"
	"github.com/floatpane/termimage"
)

// TestMain handles image decoder workers spawned by remote image tests and
// keeps tests that save settings, drafts or caches away from the
// developer's real matcha config and caches.
func TestMain(m *testing.M) {
	termimage.MaybeRunWorker()
	// Image protocol detection changes inside tmux.
	for _, key := range []string{"TMUX", "TMUX_PANE"} {
		_ = os.Unsetenv(key)
	}
	testhome.Main(m)
}
