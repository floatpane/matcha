package tui

import (
	"os"
	"testing"

	"github.com/floatpane/termimage"
)

// TestMain handles image decoder workers spawned by remote image tests.
func TestMain(m *testing.M) {
	termimage.MaybeRunWorker()
	os.Exit(m.Run())
}
