//go:build windows

package daemon

import (
	"fmt"

	udsrpc "github.com/floatpane/go-uds-jsonrpc"
)

// IsRunning reports whether the process recorded in the PID file at path is
// alive, and its PID.
var IsRunning = udsrpc.IsRunning

// LockPID records this process as the running daemon in the PID file at
// path. release removes the file.
func LockPID(path string) (release func(), err error) {
	if pid, running := IsRunning(path); running {
		return nil, fmt.Errorf("daemon already running (PID %d)", pid)
	}
	if err := WritePID(path); err != nil {
		return nil, fmt.Errorf("write PID file: %w", err)
	}
	return func() { _ = RemovePID(path) }, nil
}
