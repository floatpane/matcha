package daemonclient

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/floatpane/matcha/daemonrpc"
)

// stubService records the calls the tests care about.
type stubService struct {
	Service
	daemon bool
	events chan *daemonrpc.Event
	closed atomic.Int32
	queued atomic.Int32
}

func (s *stubService) IsDaemon() bool                     { return s.daemon }
func (s *stubService) Events() <-chan *daemonrpc.Event    { return s.events }
func (s *stubService) Close() error                       { s.closed.Add(1); return nil }
func (s *stubService) CancelEmail(string) error           { return nil }
func (s *stubService) Subscribe(string, string) error     { return nil }
func (s *stubService) Unsubscribe(string, string) error   { return nil }
func (s *stubService) RefreshFolder(string, string) error { return nil }
func (s *stubService) QueueEmail(string, []string, []string, []string, string, string, string, map[string][]byte, map[string][]byte, string, []string, bool, bool, bool, bool, int, []byte) (string, error) {
	s.queued.Add(1)
	return "job", nil
}

func TestStartServiceDoesNotBlock(t *testing.T) {
	release := make(chan struct{})
	stub := &stubService{daemon: true, events: make(chan *daemonrpc.Event)}
	start := time.Now()
	s := StartServiceFunc(func() Service { <-release; return stub })
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("StartServiceFunc waited for the connection")
	}
	if s.IsReady() || s.IsDaemon() || s.Events() != nil {
		t.Fatal("a starting service must not report itself ready, daemon-backed or with events")
	}

	// Operations wait for the connection instead of failing.
	done := make(chan string)
	go func() {
		id, _ := s.QueueEmail("a", nil, nil, nil, "", "", "", nil, nil, "", nil, false, false, false, false, 0, nil)
		done <- id
	}()
	select {
	case <-done:
		t.Fatal("QueueEmail returned before the service was ready")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	select {
	case id := <-done:
		if id != "job" || stub.queued.Load() != 1 {
			t.Fatalf("QueueEmail = %q (queued %d), want the stub's job", id, stub.queued.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("QueueEmail never completed")
	}
	<-s.Ready()
	if !s.IsReady() || !s.IsDaemon() || s.Events() == nil {
		t.Fatal("ready service should expose the daemon and its events")
	}
}

func TestCloseWhileStartingClosesTheServiceOnceReady(t *testing.T) {
	release := make(chan struct{})
	stub := &stubService{}
	s := StartServiceFunc(func() Service { <-release; return stub })
	start := time.Now()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("Close waited for the connection")
	}
	close(release)
	<-s.Ready()
	deadline := time.Now().Add(2 * time.Second)
	for stub.closed.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := stub.closed.Load(); got != 1 {
		t.Fatalf("underlying service closed %d times, want 1", got)
	}
	_ = s.Close() // second close is a no-op
	time.Sleep(20 * time.Millisecond)
	if got := stub.closed.Load(); got != 1 {
		t.Fatalf("underlying service closed %d times after a second Close, want 1", got)
	}
}

func TestCloseReadyServiceDoesNotBlock(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	slow := &slowCloseService{block: block}
	s := StartServiceFunc(func() Service { return slow })
	<-s.Ready()
	start := time.Now()
	_ = s.Close()
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("Close waited for the underlying service to shut down")
	}
}

type slowCloseService struct {
	Service
	block chan struct{}
}

func (s *slowCloseService) Close() error { <-s.block; return nil }

// orderedService records whether calls arrive before or after Close
// finished.
type orderedService struct {
	Service
	block      chan struct{}
	closedDone atomic.Bool
	early      atomic.Int32 // calls that raced the shutdown
	late       atomic.Int32 // calls made once it was closed
}

func (s *orderedService) Close() error {
	<-s.block
	s.closedDone.Store(true)
	return nil
}

func (s *orderedService) DeleteEmails(string, string, []uint32) error {
	if s.closedDone.Load() {
		s.late.Add(1)
	} else {
		s.early.Add(1)
	}
	return nil
}

func TestCallsAfterCloseWaitForTheShutdown(t *testing.T) {
	for _, startedFirst := range []bool{true, false} {
		block := make(chan struct{})
		svc := &orderedService{block: block}
		release := make(chan struct{})
		s := StartServiceFunc(func() Service { <-release; return svc })
		if startedFirst {
			close(release)
			<-s.Ready()
		}
		_ = s.Close() // e.g. ctrl+c, whose flush then deletes pending mail
		done := make(chan struct{})
		go func() {
			_ = s.DeleteEmails("a", "INBOX", []uint32{1})
			close(done)
		}()
		if !startedFirst {
			close(release)
		}
		select {
		case <-done:
			t.Fatalf("startedFirst=%v: a call made after Close ran while the service was shutting down", startedFirst)
		case <-time.After(50 * time.Millisecond):
		}
		close(block)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("startedFirst=%v: the call never ran", startedFirst)
		}
		if svc.early.Load() != 0 || svc.late.Load() != 1 {
			t.Fatalf("startedFirst=%v: early=%d late=%d, want the call after the shutdown", startedFirst, svc.early.Load(), svc.late.Load())
		}
	}
}
