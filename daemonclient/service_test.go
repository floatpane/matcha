package daemonclient

import (
	"encoding/json"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonrpc"
)

// connPair returns a connected pair of real (kernel-buffered) sockets.
// net.Pipe is unbuffered: with Go 1.27's encoding/json the decoder can leave
// a message's trailing newline unread, blocking the writer forever.
func connPair(t *testing.T) (server, client net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			accepted <- nil
			return
		}
		accepted <- c
	}()
	client, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	server = <-accepted
	if server == nil {
		t.Fatal("accept failed")
	}
	return server, client
}

func newTestClient(t *testing.T) (*Client, *daemonrpc.Conn) {
	t.Helper()
	serverConn, clientConn := connPair(t)
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

// A wedged daemon must not hang the caller (several calls run on the UI goroutine).
func TestClientCallTimesOut(t *testing.T) {
	client, server := newTestClient(t)
	defer client.Close()
	defer server.Close()

	start := time.Now()
	err := client.CallTimeout(daemonrpc.MethodPing, nil, nil, 100*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "no reply") {
		t.Fatalf("err = %v, want timeout", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("timeout not honoured")
	}
	if isConnError(err) {
		t.Fatal("a timeout must not be treated as a broken connection (no blind retry)")
	}
	if callTimeout(daemonrpc.MethodSubscribe) > 10*time.Second {
		t.Fatal("UI-path calls need a short timeout")
	}
}

func expectEvent(t *testing.T, ch <-chan *daemonrpc.Event, typ string) *daemonrpc.Event {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatalf("events channel closed, want %s", typ)
		}
		if ev.Type != typ {
			t.Fatalf("event %s, want %s", ev.Type, typ)
		}
		return ev
	case <-time.After(5 * time.Second):
		t.Fatalf("no %s event", typ)
	}
	return nil
}

// After the daemon connection drops, the service must reconnect, replay the
// folder subscriptions, tell the app to resync, and keep delivering events on
// the same channel.
func TestDaemonServiceEventsSurviveReconnect(t *testing.T) {
	client1, server1 := newTestClient(t)
	client2, server2 := newTestClient(t)
	defer server2.Close()

	dials := make(chan struct{}, 4)
	origDial := dialDaemonFunc
	t.Cleanup(func() { dialDaemonFunc = origDial })
	dialDaemonFunc = func() (*Client, error) {
		dials <- struct{}{}
		if len(dials) == 1 {
			return client2, nil
		}
		return nil, errors.New("no daemon")
	}

	svc := newDaemonService(nil, client1)
	events := svc.Events()

	// Subscribe on the first connection.
	go func() {
		msg, err := server1.ReceiveMessage()
		if err != nil {
			return
		}
		_ = server1.SendResponse(msg.Request.ID, true)
		_ = server1.SendEvent(daemonrpc.EventNewMail, daemonrpc.NewMailEvent{AccountID: "a", Folder: "INBOX"})
	}()
	if err := svc.Subscribe("a", "INBOX"); err != nil {
		t.Fatal(err)
	}
	expectEvent(t, events, daemonrpc.EventNewMail)

	// The second connection must receive the replayed subscription.
	replayed := make(chan daemonrpc.SubscribeParams, 1)
	go func() {
		msg, err := server2.ReceiveMessage()
		if err != nil {
			return
		}
		var p daemonrpc.SubscribeParams
		_ = json.Unmarshal(msg.Request.Params, &p)
		replayed <- p
		_ = server2.SendResponse(msg.Request.ID, true)
	}()

	server1.Close() // daemon restarts

	select {
	case p := <-replayed:
		if p.AccountID != "a" || p.Folder != "INBOX" {
			t.Fatalf("replayed %+v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("subscription not replayed after reconnect")
	}

	ev := expectEvent(t, events, daemonrpc.EventSyncComplete)
	var sc daemonrpc.SyncCompleteEvent
	if err := json.Unmarshal(ev.Data, &sc); err != nil || sc.Folder != "INBOX" {
		t.Fatalf("resync event = %s (%v)", ev.Data, err)
	}

	_ = server2.SendEvent(daemonrpc.EventNewMail, daemonrpc.NewMailEvent{AccountID: "a", Folder: "INBOX"})
	expectEvent(t, events, daemonrpc.EventNewMail)

	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case _, ok := <-events:
		if ok {
			// drain at most one in-flight event
			if _, ok := <-events; ok {
				t.Fatal("events channel still open after Close")
			}
		}
	case <-time.After(5 * time.Second):
		t.Fatal("events channel not closed after Close")
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

// Direct mode used to bind each provider to &cfg.Accounts[i]; deleting an
// account (which shifts the slice in place) re-bound account B's provider to
// account C, so B's deletes ran against C's mailbox.
func TestDirectServiceProvidersSurviveAccountRemoval(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	mk := func(id, email string) config.Account {
		return config.Account{ID: id, Email: email, ServiceProvider: "custom", IMAPServer: email + ".imap", SC: &config.SessionCache{}}
	}
	cfg := &config.Config{DisableDaemon: true, Accounts: []config.Account{mk("A", "a@x"), mk("B", "b@x"), mk("C", "c@x")}}
	svc := newDirectService(cfg)
	defer svc.Close()

	before, err := svc.getProvider("B")
	if err != nil {
		t.Fatal(err)
	}

	// Settings → delete account A (what Config.RemoveAccount does to the slice).
	cfg.Accounts = append(cfg.Accounts[:0], cfg.Accounts[1:]...)

	after, err := svc.getProvider("B")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []any{before, after} {
		acct := reflect.ValueOf(p).Elem().FieldByName("account").Elem()
		if got := acct.FieldByName("ID").String(); got != "B" {
			t.Fatalf("provider for B targets account %s", got)
		}
	}

	// Closing twice must not panic; operations after Close still work
	// (a pending undo action may be flushed after the app closed the service).
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if err := svc.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.getProvider("C"); err != nil {
		t.Fatalf("getProvider after Close: %v", err)
	}
}
