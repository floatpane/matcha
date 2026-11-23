package daemon

import udsrpc "github.com/floatpane/go-uds-jsonrpc"

// PID file helpers come from the shared go-uds-jsonrpc library. IsRunning
// and LockPID, which decide whether a daemon is running, are per platform
// (pidlock_unix.go, pidlock_windows.go).
var (
	WritePID  = udsrpc.WritePID
	ReadPID   = udsrpc.ReadPID
	RemovePID = udsrpc.RemovePID
)
