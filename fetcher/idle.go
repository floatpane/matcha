package fetcher

import (
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/floatpane/matcha/config"
)

// IdleUpdate is sent when IDLE detects a mailbox change.
type IdleUpdate struct {
	AccountID  string
	FolderName string
}

// IdleWatcher manages IDLE connections for multiple accounts.
type IdleWatcher struct {
	mu       sync.Mutex
	wg       sync.WaitGroup
	watchers map[string]*accountIdle // key: account ID
	notify   chan<- IdleUpdate
}

// ErrStopTimeout is returned when IDLE watcher goroutines do not stop before the timeout.
var ErrStopTimeout = errors.New("idle watcher: stop timed out")

// accountIdle manages a single IDLE connection for one account.
type accountIdle struct {
	account *config.Account
	folder  string
	notify  chan<- IdleUpdate
	stop    chan struct{}
	done    chan struct{}
}

// NewIdleWatcher creates a new IDLE watcher. Updates are sent to the notify channel.
func NewIdleWatcher(notify chan<- IdleUpdate) *IdleWatcher {
	return &IdleWatcher{
		watchers: make(map[string]*accountIdle),
		notify:   notify,
	}
}

// Watch starts (or restarts) an IDLE connection for the given account and folder.
func (w *IdleWatcher) Watch(account *config.Account, folder string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Stop existing watcher for this account if any
	if existing, ok := w.watchers[account.ID]; ok {
		close(existing.stop)
		delete(w.watchers, account.ID)
		// Let old connection tear down in the background
	}

	a := &accountIdle{
		account: account,
		folder:  folder,
		notify:  w.notify,
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	w.watchers[account.ID] = a
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		a.run()
	}()
}

// Stop stops the IDLE watcher for a specific account.
func (w *IdleWatcher) Stop(accountID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if a, ok := w.watchers[accountID]; ok {
		close(a.stop)
		delete(w.watchers, accountID)
		// Let old connection tear down in the background
	}
}

// StopAll stops all IDLE watchers.
func (w *IdleWatcher) StopAll() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for id, a := range w.watchers {
		close(a.stop)
		delete(w.watchers, id)
	}
}

// StopAllAndWait stops all IDLE watchers and waits for them to finish.
func (w *IdleWatcher) StopAllAndWait() {
	w.mu.Lock()
	pending := make([]chan struct{}, 0, len(w.watchers))
	for id, a := range w.watchers {
		close(a.stop)
		pending = append(pending, a.done)
		delete(w.watchers, id)
	}
	w.mu.Unlock()

	for _, done := range pending {
		<-done
	}
	w.wg.Wait()
}

// StopAllAndWaitTimeout stops all IDLE watchers and waits for them to finish up to d.
func (w *IdleWatcher) StopAllAndWaitTimeout(d time.Duration) error {
	w.mu.Lock()
	for id, a := range w.watchers {
		close(a.stop)
		delete(w.watchers, id)
	}
	w.mu.Unlock()

	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(d):
		return ErrStopTimeout
	}
}

func (a *accountIdle) run() {
	defer close(a.done)

	initialBackoff := 5 * time.Second
	maxBackoff := 2 * time.Minute
	backoff := initialBackoff

	for {
		start := time.Now()
		err := a.idleOnce()
		if err == nil {
			// Clean exit (stop was closed)
			return
		}

		// Reset backoff if we had a successful IDLE session (ran for
		// longer than the current backoff period without error).
		if time.Since(start) > backoff {
			backoff = initialBackoff
		}

		// Check if we were told to stop
		select {
		case <-a.stop:
			return
		default:
		}

		// Don't retry on authentication errors — they won't resolve by retrying
		if strings.Contains(err.Error(), "authentication error") || strings.Contains(err.Error(), "XOAUTH2 authentication failed") {
			log.Printf("IDLE stopped for account %s: %v", a.account.ID, err)
			return
		}

		log.Printf("IDLE error for account %s: %v (reconnecting in %v)", a.account.ID, err, backoff)

		// Wait with backoff before reconnecting
		select {
		case <-a.stop:
			return
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// idleOnce connects, selects the mailbox, and runs IDLE until an error or stop.
// Returns nil if stopped cleanly.
func (a *accountIdle) idleOnce() error {
	mailboxUpdates := make(chan uint32, 32)
	c, err := connectWithHandler(a.account, &imapclient.UnilateralDataHandler{
		Mailbox: func(data *imapclient.UnilateralDataMailbox) {
			if data.NumMessages == nil {
				return
			}
			// Non-blocking send: the callback runs on the IMAP socket-reader
			// goroutine, so a synchronous send would stall the socket if the
			// channel is full. The consumer only acts on the latest count
			// (see prevExists tracking below), so dropping a stale update is
			// safe — the next update will deliver the current count.
			select {
			case mailboxUpdates <- *data.NumMessages:
			default:
			}
		},
	})
	if err != nil {
		return err
	}
	defer c.Close() //nolint:errcheck

	// Select the mailbox in read-only mode
	selectData, err := c.Select(a.folder, nil).Wait()
	if err != nil {
		return err
	}
	prevExists := selectData.NumMessages

	// Start IDLE
	idleCmd, err := c.Idle()
	if err != nil {
		return err
	}

	for {
		select {
		case <-a.stop:
			idleCmd.Close() //nolint:errcheck,gosec
			idleCmd.Wait()  //nolint:errcheck,gosec
			return nil

		case newExists := <-mailboxUpdates:
			if newExists > prevExists {
				select {
				case a.notify <- IdleUpdate{
					AccountID:  a.account.ID,
					FolderName: a.folder,
				}:
				case <-a.stop:
					idleCmd.Close() //nolint:errcheck,gosec
					idleCmd.Wait()  //nolint:errcheck,gosec
					return nil
				}
			}
			prevExists = newExists

		case <-c.Closed():
			if err := idleCmd.Close(); err != nil {
				return err
			}
			return idleCmd.Wait()
		}
	}
}
