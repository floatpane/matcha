package notify

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSend_ReturnsErrorWhenBinaryMissing verifies that Send() reports a
// useful error instead of silently swallowing it when the platform-specific
// notification helper is not on PATH. See #522.
func TestSend_ReturnsErrorWhenBinaryMissing(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skipf("unsupported GOOS for this test: %s", runtime.GOOS)
	}

	// Replace PATH with an empty directory so neither osascript nor
	// notify-send can be resolved. This matches the minimal-Linux /
	// headless scenario in the issue.
	emptyDir := filepath.Join(t.TempDir(), "empty")
	if err := os.Mkdir(emptyDir, 0o755); err != nil {
		t.Fatalf("mkdir empty dir: %v", err)
	}

	oldPath := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", oldPath) })
	if err := os.Setenv("PATH", emptyDir); err != nil {
		t.Fatalf("setenv PATH: %v", err)
	}

	err := Send("title", "body")
	if err == nil {
		t.Fatalf("expected error when notification binary missing, got nil")
	}

	// The wrapped error message should name the expected tool so a caller
	// logging this can tell the user what to install.
	msg := err.Error()
	var wanted string
	switch runtime.GOOS {
	case "darwin":
		wanted = "osascript"
	case "linux":
		wanted = "notify-send"
	}
	if !strings.Contains(msg, wanted) {
		t.Fatalf("error %q does not mention expected binary %q", msg, wanted)
	}
}

// TestSend_ReturnsNilOnUnsupportedGOOS keeps the pre-fix contract intact:
// Windows (and anything else) should fall through without erroring.
func TestSend_ReturnsNilOnUnsupportedGOOS(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		t.Skipf("this test only runs on unsupported platforms, current: %s", runtime.GOOS)
	}
	if err := Send("title", "body"); err != nil {
		t.Fatalf("expected nil on %s, got %v", runtime.GOOS, err)
	}
}
