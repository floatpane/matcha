package daemonclient

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/floatpane/matcha/daemonrpc"
)

func mockDaemon(t *testing.T) (*Client, *daemonrpc.Conn) {
	t.Helper()
	serverConn, clientConn := net.Pipe()

	server := daemonrpc.NewConn(serverConn)
	client := &Client{
		conn:    daemonrpc.NewConn(clientConn),
		pending: make(map[uint64]chan *daemonrpc.Response),
		events:  make(chan *daemonrpc.Event, 64),
		done:    make(chan struct{}),
	}
	go client.readLoop()

	return client, server
}

func TestClient_Ping(t *testing.T) {
	client, server := mockDaemon(t)
	defer client.Close()
	defer server.Close()

	// Mock server: respond to Ping.
	go func() {
		msg, err := server.ReceiveMessage()
		if err != nil {
			t.Error(err)
			return
		}
		if msg.Request.Method != daemonrpc.MethodPing {
			t.Errorf("method = %q, want Ping", msg.Request.Method)
		}
		server.SendResponse(msg.Request.ID, daemonrpc.PingResult{Pong: true})
	}()

	if err := client.Ping(); err != nil {
		t.Fatal(err)
	}
}

func TestClient_Status(t *testing.T) {
	client, server := mockDaemon(t)
	defer client.Close()
	defer server.Close()

	go func() {
		msg, _ := server.ReceiveMessage()
		server.SendResponse(msg.Request.ID, daemonrpc.StatusResult{
			Running:  true,
			Uptime:   120,
			Accounts: []string{"alice@example.com"},
			PID:      12345,
		})
	}()

	status, err := client.Status()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Running {
		t.Error("expected running=true")
	}
	if status.PID != 12345 {
		t.Errorf("PID = %d, want 12345", status.PID)
	}
	if len(status.Accounts) != 1 || status.Accounts[0] != "alice@example.com" {
		t.Errorf("accounts = %v, want [alice@example.com]", status.Accounts)
	}
}

func TestClient_CallError(t *testing.T) {
	client, server := mockDaemon(t)
	defer client.Close()
	defer server.Close()

	go func() {
		msg, _ := server.ReceiveMessage()
		server.SendError(msg.Request.ID, daemonrpc.ErrCodeNotFound, "method not found")
	}()

	var result daemonrpc.PingResult
	err := client.Call("NonExistent", nil, &result)
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "method not found" {
		t.Errorf("error = %q, want 'method not found'", err.Error())
	}
}

func TestClient_Events(t *testing.T) {
	client, server := mockDaemon(t)
	defer client.Close()
	defer server.Close()

	// Server pushes an event.
	go func() {
		server.SendEvent(daemonrpc.EventNewMail, daemonrpc.NewMailEvent{
			AccountID: "acc1",
			Folder:    "INBOX",
		})
	}()

	ev := <-client.Events()
	if ev.Type != daemonrpc.EventNewMail {
		t.Errorf("type = %q, want NewMail", ev.Type)
	}

	var data daemonrpc.NewMailEvent
	if err := json.Unmarshal(ev.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.AccountID != "acc1" {
		t.Errorf("account_id = %q, want acc1", data.AccountID)
	}
}

func TestClient_ConcurrentCalls(t *testing.T) {
	client, server := mockDaemon(t)
	defer client.Close()
	defer server.Close()

	// Server handles two requests.
	go func() {
		for i := 0; i < 2; i++ {
			msg, err := server.ReceiveMessage()
			if err != nil {
				return
			}
			server.SendResponse(msg.Request.ID, daemonrpc.PingResult{Pong: true})
		}
	}()

	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			errs <- client.Ping()
		}()
	}

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Errorf("call %d failed: %v", i, err)
		}
	}
}

// TestClient_CloseWhileReadLoopErrors hammers the race between Close() and
// readLoop() detecting a connection error. Without closeMu, concurrent
// close(done) panics ("close of closed channel").
func TestClient_CloseWhileReadLoopErrors(t *testing.T) {
	const iterations = 200
	var panics atomic.Int32

	for i := 0; i < iterations; i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					panics.Add(1)
					t.Errorf("panic on iteration %d: %v", i, r)
				}
			}()

			serverConn, clientConn := net.Pipe()
			client := &Client{
				conn:    daemonrpc.NewConn(clientConn),
				pending: make(map[uint64]chan *daemonrpc.Response),
				events:  make(chan *daemonrpc.Event, 64),
				done:    make(chan struct{}),
			}
			go client.readLoop()

			// Close the server side so readLoop's ReceiveMessage gets an
			// error roughly at the same time as we call Close.
			serverConn.Close()

			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				_ = client.Close()
			}()
			go func() {
				defer wg.Done()
				// Give readLoop a chance to observe the error.
				time.Sleep(time.Microsecond)
				_ = client.Close()
			}()
			wg.Wait()
		}()
	}

	if panics.Load() > 0 {
		t.Fatalf("detected %d panics across %d iterations", panics.Load(), iterations)
	}
}

// TestClient_CallAfterConnectionClosed verifies that Call returns
// "connection closed" promptly after the socket dies, not "send request:
// use of closed network connection".
func TestClient_CallAfterConnectionClosed(t *testing.T) {
	client, server := mockDaemon(t)

	// Close the server side to simulate daemon death.
	server.Close()

	// Wait for readLoop to notice the error and close done.
	<-client.done

	// Now any Call should return "connection closed" quickly.
	done := make(chan error, 1)
	go func() {
		done <- client.Call(daemonrpc.MethodPing, nil, nil)
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error after connection closed")
		}
		// Should be "connection closed", not a hang or a send error.
	case <-time.After(2 * time.Second):
		t.Fatal("Call hung after connection closed")
	}
}

// TestDaemonService_Reconnect verifies that daemonService transparently
// reconnects after the socket dies.
func TestDaemonService_Reconnect(t *testing.T) {
	// We can't use tryConnectWithConfig (it dials the real daemon), so
	// we build the daemonService manually with a mock client.

	// Create first connection pair (original daemon).
	serverConn1, clientConn1 := net.Pipe()
	server1 := daemonrpc.NewConn(serverConn1)
	client1 := &Client{
		conn:    daemonrpc.NewConn(clientConn1),
		pending: make(map[uint64]chan *daemonrpc.Response),
		events:  make(chan *daemonrpc.Event, 64),
		done:    make(chan struct{}),
	}
	go client1.readLoop()

	svc := &daemonService{
		client: client1,
	}

	// Server1 responds to one Ping, then we kill it.
	go func() {
		msg, err := server1.ReceiveMessage()
		if err != nil {
			return
		}
		server1.SendResponse(msg.Request.ID, daemonrpc.PingResult{Pong: true})
	}()

	// First call succeeds.
	if err := svc.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodPing, nil, nil)
	}); err != nil {
		t.Fatalf("first call failed: %v", err)
	}

	// Kill the server side to simulate daemon crash.
	server1.Close()
	// Wait for readLoop to notice.
	<-client1.done

	// Now set up a "reconnected" daemon by replacing the client.
	// We simulate a successful reconnect by directly swapping the client.
	serverConn2, clientConn2 := net.Pipe()
	server2 := daemonrpc.NewConn(serverConn2)
	client2 := &Client{
		conn:    daemonrpc.NewConn(clientConn2),
		pending: make(map[uint64]chan *daemonrpc.Response),
		events:  make(chan *daemonrpc.Event, 64),
		done:    make(chan struct{}),
	}
	go client2.readLoop()

	// Override reconnect to swap in client2.
	svc.mu.Lock()
	svc.client = client2
	svc.mu.Unlock()

	// Server2 responds to Ping.
	go func() {
		msg, err := server2.ReceiveMessage()
		if err != nil {
			return
		}
		server2.SendResponse(msg.Request.ID, daemonrpc.PingResult{Pong: true})
	}()

	// After reconnect, the call should succeed.
	if err := svc.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodPing, nil, nil)
	}); err != nil {
		t.Fatalf("call after reconnect failed: %v", err)
	}

	svc.Close()
	server2.Close()
}

// TestIsConnError verifies the error classification.
func TestIsConnError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("connection closed"), true},
		{fmt.Errorf("send request: use of closed network connection"), true},
		{fmt.Errorf("write: broken pipe"), true},
		{fmt.Errorf("unexpected EOF"), true},
		{fmt.Errorf("connect to daemon: dial unix: no such file"), true},
		{fmt.Errorf("method not found"), false},
		{fmt.Errorf("no provider for account foo"), false},
		{&daemonrpc.Error{Code: daemonrpc.ErrCodeNotFound, Message: "not found"}, false},
	}

	for _, tt := range tests {
		got := isConnError(tt.err)
		if got != tt.want {
			t.Errorf("isConnError(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
