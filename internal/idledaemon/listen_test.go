package idledaemon

import (
	"testing"

	"github.com/floatpane/matcha/daemonrpc"
	"github.com/floatpane/matcha/tui"
)

// A closed events channel must produce a message the app can re-arm on.
func TestListenForDaemonEventsReportsClose(t *testing.T) {
	ch := make(chan *daemonrpc.Event)
	close(ch)
	msg, ok := ListenForDaemonEvents(ch)().(tui.DaemonEventMsg)
	if !ok || msg.Event != nil {
		t.Fatalf("got %#v, want DaemonEventMsg{Event: nil}", msg)
	}

	live := make(chan *daemonrpc.Event, 1)
	live <- &daemonrpc.Event{Type: daemonrpc.EventNewMail}
	msg, ok = ListenForDaemonEvents(live)().(tui.DaemonEventMsg)
	if !ok || msg.Event == nil || msg.Event.Type != daemonrpc.EventNewMail {
		t.Fatalf("got %#v", msg)
	}
}
