package daemon

import udsrpc "github.com/floatpane/go-uds-jsonrpc"

// PID file helpers come from the shared go-uds-jsonrpc library, which handles
// the Unix (signal-0 probe) and Windows (OpenProcess) implementations behind
// build tags. Re-exported so daemon.WritePID/IsRunning/etc. keep working for
// callers in main.go.
var (
	WritePID  = udsrpc.WritePID
	ReadPID   = udsrpc.ReadPID
	IsRunning = udsrpc.IsRunning
	RemovePID = udsrpc.RemovePID
)
