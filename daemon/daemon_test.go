package daemon

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonrpc"
)

func TestPIDFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.pid")

	if err := WritePID(path); err != nil {
		t.Fatal(err)
	}

	pid, err := ReadPID(path)
	if err != nil {
		t.Fatal(err)
	}
	if pid != os.Getpid() {
		t.Errorf("pid = %d, want %d", pid, os.Getpid())
	}

	gotPID, running := IsRunning(path)
	if !running {
		t.Error("expected running=true for current process")
	}
	if gotPID != os.Getpid() {
		t.Errorf("pid = %d, want %d", gotPID, os.Getpid())
	}

	if err := RemovePID(path); err != nil {
		t.Fatal(err)
	}

	_, running = IsRunning(path)
	if running {
		t.Error("expected running=false after remove")
	}
}

func TestPIDFile_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.pid")

	os.WriteFile(path, []byte("notanumber"), 0644)
	_, err := ReadPID(path)
	if err == nil {
		t.Error("expected error for invalid PID content")
	}
}

func TestPIDFile_DeadProcess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dead.pid")

	os.WriteFile(path, []byte("99999999"), 0644)
	_, running := IsRunning(path)
	if running {
		t.Error("expected running=false for dead PID")
	}
}

// handlerTest sets up a client/server pipe and runs a single RPC exchange.
// The handler runs in a goroutine so the pipe doesn't deadlock.
func handlerTest(t *testing.T, d *Daemon, req *daemonrpc.Request) daemonrpc.Message {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	server := daemonrpc.NewConn(serverConn)
	client := daemonrpc.NewConn(clientConn)

	// Handle request in goroutine (SendResponse blocks until client reads).
	go func() {
		d.handleRequest(server, req)
	}()

	msg, err := client.ReceiveMessage()
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestDaemon_PingHandler(t *testing.T) {
	d := &Daemon{shutdown: make(chan struct{})}
	msg := handlerTest(t, d, &daemonrpc.Request{ID: 1, Method: daemonrpc.MethodPing})

	if msg.Response == nil {
		t.Fatal("expected Response")
	}
	var result daemonrpc.PingResult
	json.Unmarshal(msg.Response.Result, &result)
	if !result.Pong {
		t.Error("expected pong=true")
	}
}

func TestDaemon_StatusHandler(t *testing.T) {
	d := &Daemon{
		startTime: time.Now().Add(-2 * time.Minute),
		shutdown:  make(chan struct{}),
		config:    &config.Config{},
	}

	msg := handlerTest(t, d, &daemonrpc.Request{ID: 1, Method: daemonrpc.MethodGetStatus})

	var result daemonrpc.StatusResult
	json.Unmarshal(msg.Response.Result, &result)

	if !result.Running {
		t.Error("expected running=true")
	}
	if result.Uptime < 120 {
		t.Errorf("uptime = %d, want >= 120", result.Uptime)
	}
}

func TestDaemon_UnknownMethod(t *testing.T) {
	d := &Daemon{shutdown: make(chan struct{})}
	msg := handlerTest(t, d, &daemonrpc.Request{ID: 1, Method: "DoesNotExist"})

	if msg.Response.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if msg.Response.Error.Code != daemonrpc.ErrCodeNotFound {
		t.Errorf("code = %d, want %d", msg.Response.Error.Code, daemonrpc.ErrCodeNotFound)
	}
}

func TestDaemon_Subscribe(t *testing.T) {
	d := &Daemon{
		subscriptions: make(map[*daemonrpc.Conn]map[string]struct{}),
		shutdown:      make(chan struct{}),
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	server := daemonrpc.NewConn(serverConn)
	client := daemonrpc.NewConn(clientConn)

	params, _ := json.Marshal(daemonrpc.SubscribeParams{
		AccountID: "acc1",
		Folder:    "INBOX",
	})

	go func() {
		d.handleRequest(server, &daemonrpc.Request{
			ID:     1,
			Method: daemonrpc.MethodSubscribe,
			Params: params,
		})
	}()

	// Read response.
	msg, err := client.ReceiveMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Response.Error != nil {
		t.Errorf("unexpected error: %v", msg.Response.Error)
	}

	// Verify subscription was recorded.
	d.subMu.RLock()
	subs, ok := d.subscriptions[server]
	d.subMu.RUnlock()

	if !ok {
		t.Fatal("expected subscription entry for connection")
	}
	if _, ok := subs["acc1:INBOX"]; !ok {
		t.Error("expected subscription for acc1:INBOX")
	}
}

func TestDaemon_BroadcastEvent(t *testing.T) {
	d := &Daemon{
		clients:  make(map[*daemonrpc.Conn]struct{}),
		shutdown: make(chan struct{}),
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	server := daemonrpc.NewConn(serverConn)
	client := daemonrpc.NewConn(clientConn)

	d.mu.Lock()
	d.clients[server] = struct{}{}
	d.mu.Unlock()

	go func() {
		d.broadcastEvent(daemonrpc.EventNewMail, daemonrpc.NewMailEvent{
			AccountID: "acc1",
			Folder:    "INBOX",
		})
	}()

	msg, err := client.ReceiveMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Event == nil {
		t.Fatal("expected Event")
	}
	if msg.Event.Type != daemonrpc.EventNewMail {
		t.Errorf("type = %q, want NewMail", msg.Event.Type)
	}
}
