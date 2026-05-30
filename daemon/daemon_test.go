package daemon

import (
	"context"
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

// serveDaemon starts d's RPC server on a temporary unix socket and returns a
// connected client. The server and connection are torn down via t.Cleanup.
func serveDaemon(t *testing.T, d *Daemon) *daemonrpc.Conn {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = d.server.Serve(ctx, l) }()
	t.Cleanup(func() {
		cancel()
		_ = l.Close()
	})

	c, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn := daemonrpc.NewConn(c)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// roundTrip sends a request and returns the decoded response message.
func roundTrip(t *testing.T, conn *daemonrpc.Conn, req *daemonrpc.Request) daemonrpc.Message {
	t.Helper()
	if err := conn.Send(req); err != nil {
		t.Fatal(err)
	}
	msg, err := conn.ReceiveMessage()
	if err != nil {
		t.Fatal(err)
	}
	return msg
}

func TestDaemon_PingHandler(t *testing.T) {
	d := New(&config.Config{})
	res, err := d.handlePing(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("handlePing: %v", err)
	}
	if !res.(daemonrpc.PingResult).Pong {
		t.Error("expected pong=true")
	}
}

func TestDaemon_StatusHandler(t *testing.T) {
	d := New(&config.Config{})
	d.startTime = time.Now().Add(-2 * time.Minute)

	res, err := d.handleGetStatus(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("handleGetStatus: %v", err)
	}
	result := res.(daemonrpc.StatusResult)

	if !result.Running {
		t.Error("expected running=true")
	}
	if result.Uptime < 120 {
		t.Errorf("uptime = %d, want >= 120", result.Uptime)
	}
}

func TestDaemon_UnknownMethod(t *testing.T) {
	d := New(&config.Config{})
	conn := serveDaemon(t, d)

	msg := roundTrip(t, conn, &daemonrpc.Request{ID: 1, Method: "DoesNotExist"})
	if msg.Response == nil || msg.Response.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if msg.Response.Error.Code != daemonrpc.ErrCodeNotFound {
		t.Errorf("code = %d, want %d", msg.Response.Error.Code, daemonrpc.ErrCodeNotFound)
	}
}

func TestDaemon_Subscribe(t *testing.T) {
	d := New(&config.Config{})
	conn := serveDaemon(t, d)

	params, _ := json.Marshal(daemonrpc.SubscribeParams{
		AccountID: "acc1",
		Folder:    "INBOX",
	})

	msg := roundTrip(t, conn, &daemonrpc.Request{
		ID:     1,
		Method: daemonrpc.MethodSubscribe,
		Params: params,
	})
	if msg.Response.Error != nil {
		t.Errorf("unexpected error: %v", msg.Response.Error)
	}

	// The response is sent after the handler records the subscription, so it
	// is visible by the time we read the reply.
	d.subMu.RLock()
	defer d.subMu.RUnlock()
	found := false
	for _, subs := range d.subscriptions {
		if _, ok := subs["acc1:INBOX"]; ok {
			found = true
		}
	}
	if !found {
		t.Error("expected subscription for acc1:INBOX")
	}
}

func TestDaemon_BroadcastEvent(t *testing.T) {
	d := New(&config.Config{})
	conn := serveDaemon(t, d)

	// Ping round-trip ensures the client is registered with the server before
	// we broadcast.
	roundTrip(t, conn, &daemonrpc.Request{ID: 1, Method: daemonrpc.MethodPing})

	d.broadcastEvent(daemonrpc.EventNewMail, daemonrpc.NewMailEvent{
		AccountID: "acc1",
		Folder:    "INBOX",
	})

	msg, err := conn.ReceiveMessage()
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
