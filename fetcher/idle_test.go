package fetcher

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/floatpane/matcha/config"
)

func TestIdleWatcher_StopAllAndWait_TracksReplacedGoroutines(t *testing.T) {
	w := NewIdleWatcher(make(chan IdleUpdate))
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	var exits atomic.Int64

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer close(doneCh)
		<-stopCh
		exits.Add(1)
	}()

	w.watchers["acct"] = &accountIdle{
		account: &config.Account{ID: "acct"},
		stop:    stopCh,
		done:    doneCh,
	}

	if err := w.StopAllAndWaitTimeout(time.Second); err != nil {
		t.Fatalf("StopAllAndWaitTimeout returned error: %v", err)
	}
	if got := exits.Load(); got != 1 {
		t.Fatalf("expected synthetic watcher to exit once, got %d", got)
	}
}

func TestIdleWatcher_StopAllAndWaitTimeout_ReturnsOnSlowExit(t *testing.T) {
	w := NewIdleWatcher(make(chan IdleUpdate))
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	release := make(chan struct{})
	exited := make(chan struct{})

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer close(doneCh)
		defer close(exited)
		<-release
	}()

	w.watchers["acct"] = &accountIdle{
		account: &config.Account{ID: "acct"},
		stop:    stopCh,
		done:    doneCh,
	}

	err := w.StopAllAndWaitTimeout(50 * time.Millisecond)
	if !errors.Is(err, ErrStopTimeout) {
		t.Fatalf("expected ErrStopTimeout, got %v", err)
	}

	close(release)
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("synthetic watcher did not exit during cleanup")
	}
}
