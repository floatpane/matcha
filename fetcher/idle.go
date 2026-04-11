package fetcher

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/emersion/go-imap/client"
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
	watchers map[string]*accountIdle // key: account ID
	notify   chan<- IdleUpdate
}

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
	go a.run()
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
	var pending []chan struct{}
	for id, a := range w.watchers {
		close(a.stop)
		pending = append(pending, a.done)
		delete(w.watchers, id)
	}
	w.mu.Unlock()

	for _, done := range pending {
		<-done
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
	c, err := connect(a.account)
	if err != nil {
		return err
	}
	defer func() {
		_ = c.Logout()
	}()

	// Select the mailbox in read-only mode
	mbox, err := c.Select(a.folder, true)
	if err != nil {
		return err
	}
	prevExists := mbox.Messages

	// Set up update channel
	updates := make(chan client.Update, 32)
	c.Updates = updates

	// Run IDLE in a goroutine
	idleDone := make(chan error, 1)
	idleStop := make(chan struct{})
	go func() {
		idleDone <- c.Idle(idleStop, nil)
	}()

	for {
		select {
		case <-a.stop:
			close(idleStop)
			<-idleDone
			return nil

		case update := <-updates:
			switch u := update.(type) {
			case *client.MailboxUpdate:
				newExists := u.Mailbox.Messages
				if newExists > prevExists {
					// New mail arrived
					select {
					case a.notify <- IdleUpdate{
						AccountID:  a.account.ID,
						FolderName: a.folder,
					}:
					case <-a.stop:
						close(idleStop)
						<-idleDone
						return nil
					}
				}
				prevExists = newExists
			}

		case err := <-idleDone:
			return err
		}
	}
}
