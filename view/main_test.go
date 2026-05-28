package view

import (
	"os"
	"testing"

	"github.com/floatpane/termimage"
)

// TestMain intercepts subprocess invocations made by termimage's sandbox
// worker (TERMIMAGE_WORKER=1). Without this, a test binary spawned as a
// worker would re-run the full test suite, which itself triggers more
// workers — a fork bomb that exhausts RAM and freezes the machine.
// Mirrors the call in main.go's main().
func TestMain(m *testing.M) {
	termimage.MaybeRunWorker()
	os.Exit(m.Run())
}
