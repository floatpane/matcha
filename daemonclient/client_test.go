package daemonclient

import (
	"encoding/json"
	"net"
	"testing"

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
