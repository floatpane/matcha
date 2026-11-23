package daemonrpc

import (
	"encoding/json"
	"net"
	"testing"
)

// testPipe returns two connected Conns over a real loopback socket.
//
// net.Pipe is unbuffered: every Write blocks until the peer reads all of it.
// Since Go 1.27, encoding/json's stream decoder stops right after a complete
// value and leaves the trailing newline written by Encoder.Encode unread, so a
// sender whose peer reads exactly one message blocked forever. Kernel sockets
// buffer the newline like the real Unix socket does.
func testPipe(t *testing.T) (*Conn, *Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	type result struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan result, 1)
	go func() {
		c, err := ln.Accept()
		accepted <- result{c, err}
	}()

	a, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	r := <-accepted
	if r.err != nil {
		a.Close()
		t.Fatal(r.err)
	}
	return NewConn(a), NewConn(r.conn)
}

func TestConn_SendReceiveRequest(t *testing.T) {
	client, server := testPipe(t)
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		params, _ := json.Marshal(PingResult{Pong: true})
		err := client.Send(&Request{ID: 1, Method: MethodPing, Params: params})
		done <- err
	}()

	msg, err := server.ReceiveMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Request == nil {
		t.Fatal("expected Request")
	}
	if msg.Request.Method != MethodPing {
		t.Errorf("method = %q, want Ping", msg.Request.Method)
	}

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestConn_SendResponse(t *testing.T) {
	client, server := testPipe(t)
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		err := server.SendResponse(1, PingResult{Pong: true})
		done <- err
	}()

	msg, err := client.ReceiveMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Response == nil {
		t.Fatal("expected Response")
	}
	if msg.Response.ID != 1 {
		t.Errorf("id = %d, want 1", msg.Response.ID)
	}

	var result PingResult
	if err := json.Unmarshal(msg.Response.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Pong {
		t.Error("expected pong=true")
	}

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestConn_SendError(t *testing.T) {
	client, server := testPipe(t)
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		err := server.SendError(5, ErrCodeNotFound, "method not found")
		done <- err
	}()

	msg, err := client.ReceiveMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Response == nil {
		t.Fatal("expected Response")
	}
	if msg.Response.Error == nil {
		t.Fatal("expected error")
	}
	if msg.Response.Error.Code != ErrCodeNotFound {
		t.Errorf("code = %d, want %d", msg.Response.Error.Code, ErrCodeNotFound)
	}

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestConn_SendEvent(t *testing.T) {
	client, server := testPipe(t)
	defer client.Close()
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		err := server.SendEvent(EventNewMail, NewMailEvent{
			AccountID: "acc1",
			Folder:    "INBOX",
		})
		done <- err
	}()

	msg, err := client.ReceiveMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg.Event == nil {
		t.Fatal("expected Event")
	}
	if msg.Event.Type != EventNewMail {
		t.Errorf("type = %q, want NewMail", msg.Event.Type)
	}

	var ev NewMailEvent
	if err := json.Unmarshal(msg.Event.Data, &ev); err != nil {
		t.Fatal(err)
	}
	if ev.AccountID != "acc1" {
		t.Errorf("account_id = %q, want acc1", ev.AccountID)
	}

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestConn_MultipleMessages(t *testing.T) {
	client, server := testPipe(t)
	defer client.Close()
	defer server.Close()

	go func() {
		client.Send(&Request{ID: 1, Method: MethodPing})
		client.Send(&Request{ID: 2, Method: MethodGetStatus})
	}()

	msg1, err := server.ReceiveMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg1.Request.ID != 1 {
		t.Errorf("first id = %d, want 1", msg1.Request.ID)
	}

	msg2, err := server.ReceiveMessage()
	if err != nil {
		t.Fatal(err)
	}
	if msg2.Request.ID != 2 {
		t.Errorf("second id = %d, want 2", msg2.Request.ID)
	}
}
