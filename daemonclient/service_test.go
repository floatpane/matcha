package daemonclient

import (
	"testing"

	"github.com/floatpane/matcha/config"
)

func TestNewCLIClient_NoAutoStart(t *testing.T) {
	// Set XDG_RUNTIME_DIR to a non-existent path to ensure we cannot connect to a running daemon
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	cfg := &config.Config{
		DisableDaemon: false,
	}

	// When autoStart is false and daemon is not running, we expect a directService fallback (IsDaemon() == false)
	svc := NewCLIClient(cfg, false)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	defer svc.Close()

	if svc.IsDaemon() {
		t.Error("expected IsDaemon() to be false (direct service fallback) when autoStart is false and daemon is not running")
	}
}
