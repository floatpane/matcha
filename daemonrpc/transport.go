package daemonrpc

import udsrpc "github.com/floatpane/go-uds-jsonrpc"

// Conn is the newline-delimited JSON-RPC connection provided by the shared
// go-uds-jsonrpc transport. Aliased so the rest of matcha keeps referring to
// daemonrpc.Conn.
type Conn = udsrpc.Conn

// NewConn wraps an existing net.Conn.
var NewConn = udsrpc.NewConn
