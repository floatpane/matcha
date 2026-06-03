package daemonrpc

import (
	"path/filepath"

	udsrpc "github.com/floatpane/go-uds-jsonrpc"
)

// appName is the per-user runtime directory namespace for matcha's daemon
// files. It matches the historical layout: $XDG_RUNTIME_DIR/matcha/ on Linux,
// ~/Library/Caches/matcha/ on macOS.
const appName = "matcha"

// SocketPath returns the path to the daemon's Unix domain socket.
func SocketPath() string {
	return filepath.Join(udsrpc.RuntimeDir(appName), "daemon.sock")
}

// PIDPath returns the path to the daemon's PID file.
func PIDPath() string {
	return filepath.Join(udsrpc.RuntimeDir(appName), "daemon.pid")
}

// EnsureRuntimeDir creates the runtime directory if it doesn't exist.
func EnsureRuntimeDir() error {
	return udsrpc.EnsureRuntimeDir(appName)
}
