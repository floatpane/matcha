package daemonrpc

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// runtimeDir returns the base directory for daemon runtime files.
// Linux: $XDG_RUNTIME_DIR/matcha/
// macOS: ~/Library/Caches/matcha/
func runtimeDir() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "Library", "Caches", "matcha")
	default: // linux and others
		if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
			return filepath.Join(dir, "matcha")
		}
		// Fallback: /tmp/matcha-<uid>
		return filepath.Join(os.TempDir(), "matcha-"+uidStr())
	}
}

func uidStr() string {
	return fmt.Sprintf("%d", os.Getuid())
}

// SocketPath returns the path to the daemon's Unix domain socket.
func SocketPath() string {
	return filepath.Join(runtimeDir(), "daemon.sock")
}

// PIDPath returns the path to the daemon's PID file.
func PIDPath() string {
	return filepath.Join(runtimeDir(), "daemon.pid")
}

// EnsureRuntimeDir creates the runtime directory if it doesn't exist.
func EnsureRuntimeDir() error {
	return os.MkdirAll(runtimeDir(), 0700)
}
