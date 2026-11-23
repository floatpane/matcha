package daemonclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/floatpane/matcha/backend"
	_ "github.com/floatpane/matcha/backend/imap"    // register imap backend for directService
	_ "github.com/floatpane/matcha/backend/jmap"    // register jmap backend for directService
	_ "github.com/floatpane/matcha/backend/maildir" // register maildir backend for directService
	_ "github.com/floatpane/matcha/backend/pop3"    // register pop3 backend for directService
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonrpc"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/loglevel"
	"github.com/floatpane/matcha/sender"
)

// Service abstracts daemon-backed vs direct email operations.
// TUI and CLI use this interface — they don't care which mode is active.
type Service interface {
	FetchEmails(accountID, folder string, limit, offset uint32) ([]backend.Email, error)
	// FetchEmailBody returns body, MIME type ("text/html"|"text/plain"|""),
	// attachments, and any error.
	FetchEmailBody(accountID, folder string, uid uint32) (string, string, []backend.Attachment, error)
	DeleteEmails(accountID, folder string, uids []uint32) error
	ArchiveEmails(accountID, folder string, uids []uint32) error
	MoveEmails(accountID string, uids []uint32, src, dst string) error
	MarkRead(accountID, folder string, uids []uint32) error
	MarkUnread(accountID, folder string, uids []uint32) error
	QueueEmail(accountID string, to, cc, bcc []string, subject, body, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string, signSMIME, encryptSMIME, signPGP, encryptPGP bool, delaySeconds int, prebuiltRaw []byte) (string, error)
	CancelEmail(jobID string) error
	FetchFolders(accountID string) ([]backend.Folder, error)
	RefreshFolder(accountID, folder string) error
	Subscribe(accountID, folder string) error
	Unsubscribe(accountID, folder string) error
	Search(accountID, folder string, query backend.SearchQuery) ([]backend.Email, error)
	ReloadConfig() error
	Events() <-chan *daemonrpc.Event
	IsDaemon() bool
	Close() error
}

// NewService connects to the daemon, auto-starting it if needed.
// Falls back to direct mode only if daemon cannot be started, or if DisableDaemon is set.
func NewService(cfg *config.Config) Service {
	return NewCLIClient(cfg, true)
}

// NewCLIClient connects to the daemon. If autoStart is true and daemon is not
// running, it auto-starts the daemon. If autoStart is false, it falls back to
// direct mode immediately if the daemon is not running.
func NewCLIClient(cfg *config.Config, autoStart bool) Service {
	if cfg.DisableDaemon {
		log.Println("service: daemon disabled by config, using direct mode")
		return newDirectService(cfg)
	}

	// Try connecting to existing daemon.
	if svc := tryConnectWithConfig(cfg); svc != nil {
		return svc
	}

	if !autoStart {
		loglevel.Debugf("service: daemon not running, auto-start disabled, using direct mode")
		return newDirectService(cfg)
	}

	// Daemon not running — auto-start it.
	loglevel.Debugf("service: daemon not running, auto-starting")
	if err := autoStartDaemon(); err != nil {
		loglevel.Debugf("service: auto-start failed: %v, using direct mode", err)
		return newDirectService(cfg)
	}

	// Wait briefly for daemon to become ready, then connect.
	for i := 0; i < 20; i++ {
		time.Sleep(100 * time.Millisecond)
		if svc := tryConnectWithConfig(cfg); svc != nil {
			loglevel.Debugf("service: connected to auto-started daemon")
			return svc
		}
	}

	log.Println("service: daemon started but not responding, using direct mode")
	return newDirectService(cfg)
}

func tryConnectWithConfig(cfg *config.Config) *daemonService {
	client, err := dialDaemonFunc()
	if err != nil {
		return nil
	}
	return newDaemonService(cfg, client)
}

// dialDaemonFunc is a seam for tests (the real socket may belong to a running
// daemon).
var dialDaemonFunc = dialDaemon

// dialDaemon connects to the daemon socket and checks it responds.
func dialDaemon() (*Client, error) {
	client, err := Dial()
	if err != nil {
		return nil, err
	}
	if err := client.Ping(); err != nil {
		client.Close() //nolint:errcheck,gosec
		return nil, err
	}
	return client, nil
}

func autoStartDaemon() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}

	cmd := exec.Command(exe, "daemon", "run") //nolint:noctx
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = DaemonProcAttr()

	return cmd.Start()
}

// maxReconnectBackoff caps the delay between attempts to reach a daemon that
// went away (crash, restart, update).
const maxReconnectBackoff = 30 * time.Second

type subscription struct {
	accountID string
	folder    string
}

// daemonService routes all operations through the daemon socket.
//
// The connection can be replaced at any time (the daemon restarts, a call
// hits a broken socket). Events() is a single channel that survives those
// reconnects: a pump goroutine forwards events from whichever connection is
// current, re-establishes the connection when it drops, replays the folder
// subscriptions (they are per connection on the daemon side) and then emits a
// SyncComplete event per subscribed folder so the app refetches what it may
// have missed. The channel is closed only by Close.
type daemonService struct {
	cfg *config.Config

	mu     sync.Mutex // guards client, subs and closed
	client *Client
	subs   map[subscription]struct{}
	closed bool

	reconnectMu sync.Mutex // serializes reconnect attempts
	events      chan *daemonrpc.Event
	done        chan struct{}
}

func newDaemonService(cfg *config.Config, client *Client) *daemonService {
	s := &daemonService{
		cfg:    cfg,
		client: client,
		subs:   make(map[subscription]struct{}),
		events: make(chan *daemonrpc.Event, 64),
		done:   make(chan struct{}),
	}
	go s.pumpEvents()
	return s
}

func (s *daemonService) currentClient() *Client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.client
}

func (s *daemonService) isClosed() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

// call executes fn against the current client. If the call fails with a
// connection-level error, it attempts to reconnect once and retries.
func (s *daemonService) call(fn func(*Client) error) error {
	if s.isClosed() {
		// The app may still flush a pending action after closing the service
		// (an undo grace period that expires after leaving the inbox): use a
		// one-off connection instead of resurrecting the event pump.
		client, err := dialDaemonFunc()
		if err != nil {
			return fmt.Errorf("daemon unavailable: %w", err)
		}
		defer client.Close() //nolint:errcheck
		return fn(client)
	}

	client := s.currentClient()
	if client == nil || client.isClosed() {
		var ok bool
		if client, ok = s.reconnect(client); !ok {
			return fmt.Errorf("daemon connection lost and could not be re-established")
		}
	}

	err := fn(client)
	if err == nil {
		return nil
	}

	if !isConnError(err) {
		return err
	}

	loglevel.Debugf("service: daemon connection lost (%v), attempting reconnect", err)
	next, ok := s.reconnect(client)
	if !ok {
		return err
	}
	if err2 := fn(next); err2 != nil {
		return err2
	}
	loglevel.Debugf("service: reconnected to daemon successfully")
	return nil
}

// reconnect replaces the failed client with a fresh connection and replays
// the folder subscriptions on it. If another goroutine already replaced the
// failed client, that connection is returned instead of dialing again.
func (s *daemonService) reconnect(failed *Client) (*Client, bool) {
	s.reconnectMu.Lock()
	defer s.reconnectMu.Unlock()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, false
	}
	cur := s.client
	s.mu.Unlock()

	if cur != nil && cur != failed && !cur.isClosed() {
		return cur, true
	}
	if cur != nil {
		cur.Close() //nolint:errcheck,gosec
	}

	client, err := dialDaemonFunc()
	if err != nil {
		s.mu.Lock()
		if s.client == cur {
			s.client = nil
		}
		s.mu.Unlock()
		return nil, false
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		client.Close() //nolint:errcheck,gosec
		return nil, false
	}
	s.client = client
	subs := s.subscriptionsLocked()
	s.mu.Unlock()

	// Subscriptions belong to the connection on the daemon side.
	for _, sub := range subs {
		if err := client.Call(daemonrpc.MethodSubscribe, daemonrpc.SubscribeParams{
			AccountID: sub.accountID,
			Folder:    sub.folder,
		}, nil); err != nil {
			loglevel.Debugf("service: resubscribe %s/%s failed: %v", sub.accountID, sub.folder, err)
		}
	}
	return client, true
}

func (s *daemonService) subscriptionsLocked() []subscription {
	subs := make([]subscription, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
	}
	return subs
}

// pumpEvents forwards events from the current connection to s.events,
// reconnecting with backoff whenever the connection drops. It is the only
// sender on s.events and closes it when the service is closed.
func (s *daemonService) pumpEvents() {
	defer close(s.events)

	var last *Client
	backoff := time.Second
	for {
		client := s.currentClient()
		if client == nil || client == last || client.isClosed() {
			var ok bool
			client, ok = s.reconnect(last)
			if !ok {
				select {
				case <-s.done:
					return
				case <-time.After(backoff):
				}
				backoff = min(2*backoff, maxReconnectBackoff)
				continue
			}
		}
		backoff = time.Second
		if last != nil {
			// Reconnected: events sent while we were away are lost.
			if !s.emitResync() {
				return
			}
		}
		last = client

		for ev := range client.Events() {
			select {
			case s.events <- ev:
			case <-s.done:
				return
			}
		}
		if s.isClosed() {
			return
		}
	}
}

// emitResync emits one synthetic SyncComplete event per subscribed folder.
// It returns false if the service was closed meanwhile.
func (s *daemonService) emitResync() bool {
	s.mu.Lock()
	subs := s.subscriptionsLocked()
	s.mu.Unlock()

	seen := make(map[string]bool)
	for _, sub := range subs {
		if seen[sub.folder] {
			continue
		}
		seen[sub.folder] = true
		data, err := json.Marshal(daemonrpc.SyncCompleteEvent{AccountID: sub.accountID, Folder: sub.folder})
		if err != nil {
			continue
		}
		select {
		case s.events <- &daemonrpc.Event{Type: daemonrpc.EventSyncComplete, Data: data}:
		case <-s.done:
			return false
		}
	}
	return true
}

// isConnError reports whether err indicates a broken socket connection
// rather than an application-level RPC error.
func isConnError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "connection closed") {
		return true
	}
	if strings.Contains(msg, "use of closed network connection") {
		return true
	}
	if strings.Contains(msg, "broken pipe") {
		return true
	}
	if strings.Contains(msg, "EOF") {
		return true
	}
	if strings.Contains(msg, "connect to daemon") {
		return true
	}
	return false
}

func (s *daemonService) FetchEmails(accountID, folder string, limit, offset uint32) ([]backend.Email, error) {
	var emails []backend.Email
	err := s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodFetchEmails, daemonrpc.FetchEmailsParams{
			AccountID: accountID,
			Folder:    folder,
			Limit:     limit,
			Offset:    offset,
		}, &emails)
	})
	return emails, err
}

func (s *daemonService) FetchEmailBody(accountID, folder string, uid uint32) (string, string, []backend.Attachment, error) {
	var result daemonrpc.FetchEmailBodyResult
	err := s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodFetchEmailBody, daemonrpc.FetchEmailBodyParams{
			AccountID: accountID,
			Folder:    folder,
			UID:       uid,
		}, &result)
	})
	if err != nil {
		return "", "", nil, err
	}

	var attachments []backend.Attachment
	for _, a := range result.Attachments {
		attachments = append(attachments, backend.Attachment{
			Filename: a.Filename,
			PartID:   a.PartID,
			Encoding: a.Encoding,
			MIMEType: a.MIMEType,
		})
	}
	return result.Body, result.BodyMIMEType, attachments, nil
}

func (s *daemonService) DeleteEmails(accountID, folder string, uids []uint32) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodDeleteEmails, daemonrpc.DeleteEmailsParams{
			AccountID: accountID,
			Folder:    folder,
			UIDs:      uids,
		}, nil)
	})
}

func (s *daemonService) ArchiveEmails(accountID, folder string, uids []uint32) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodArchiveEmails, daemonrpc.ArchiveEmailsParams{
			AccountID: accountID,
			Folder:    folder,
			UIDs:      uids,
		}, nil)
	})
}

func (s *daemonService) MoveEmails(accountID string, uids []uint32, src, dst string) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodMoveEmails, daemonrpc.MoveEmailsParams{
			AccountID:    accountID,
			UIDs:         uids,
			SourceFolder: src,
			DestFolder:   dst,
		}, nil)
	})
}

func (s *daemonService) MarkRead(accountID, folder string, uids []uint32) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodMarkRead, daemonrpc.MarkReadParams{
			AccountID: accountID,
			Folder:    folder,
			UIDs:      uids,
			Read:      true,
		}, nil)
	})
}

func (s *daemonService) MarkUnread(accountID, folder string, uids []uint32) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodMarkRead, daemonrpc.MarkReadParams{
			AccountID: accountID,
			Folder:    folder,
			UIDs:      uids,
			Read:      false,
		}, nil)
	})
}

func (s *daemonService) QueueEmail(accountID string, to, cc, bcc []string, subject, body, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string, signSMIME, encryptSMIME, signPGP, encryptPGP bool, delaySeconds int, prebuiltRaw []byte) (string, error) {
	var result daemonrpc.QueueEmailResult
	err := s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodQueueEmail, daemonrpc.QueueEmailParams{
			Email: daemonrpc.SendEmailParams{
				AccountID:    accountID,
				To:           to,
				Cc:           cc,
				Bcc:          bcc,
				Subject:      subject,
				Body:         body,
				HTMLBody:     htmlBody,
				Images:       images,
				Attachments:  attachments,
				InReplyTo:    inReplyTo,
				References:   references,
				SignSMIME:    signSMIME,
				EncryptSMIME: encryptSMIME,
				SignPGP:      signPGP,
				EncryptPGP:   encryptPGP,
				PrebuiltRaw:  prebuiltRaw,
			},
			DelaySeconds: delaySeconds,
		}, &result)
	})
	return result.JobID, err
}

func (s *daemonService) CancelEmail(jobID string) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodCancelEmail, daemonrpc.CancelEmailParams{
			JobID: jobID,
		}, nil)
	})
}

func (s *daemonService) FetchFolders(accountID string) ([]backend.Folder, error) {
	var folders []backend.Folder
	err := s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodFetchFolders, daemonrpc.FetchFoldersParams{
			AccountID: accountID,
		}, &folders)
	})
	return folders, err
}

func (s *daemonService) RefreshFolder(accountID, folder string) error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodRefreshFolder, daemonrpc.RefreshFolderParams{
			AccountID: accountID,
			Folder:    folder,
		}, nil)
	})
}

func (s *daemonService) Subscribe(accountID, folder string) error {
	s.mu.Lock()
	s.subs[subscription{accountID, folder}] = struct{}{}
	s.mu.Unlock()
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodSubscribe, daemonrpc.SubscribeParams{
			AccountID: accountID,
			Folder:    folder,
		}, nil)
	})
}

func (s *daemonService) Unsubscribe(accountID, folder string) error {
	s.mu.Lock()
	delete(s.subs, subscription{accountID, folder})
	s.mu.Unlock()
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodUnsubscribe, daemonrpc.UnsubscribeParams{
			AccountID: accountID,
			Folder:    folder,
		}, nil)
	})
}

func (s *daemonService) Search(accountID, folder string, query backend.SearchQuery) ([]backend.Email, error) {
	var emails []backend.Email
	err := s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodSearch, daemonrpc.SearchParams{
			AccountID: accountID,
			Folder:    folder,
			Query:     query.Raw, // Send the raw query string over RPC
		}, &emails)
	})
	return emails, err
}

func (s *daemonService) ReloadConfig() error {
	return s.call(func(c *Client) error {
		return c.Call(daemonrpc.MethodReloadConfig, nil, nil)
	})
}

// Events returns a channel of daemon push events that stays valid across
// reconnects. It is closed only after Close.
func (s *daemonService) Events() <-chan *daemonrpc.Event {
	return s.events
}

func (s *daemonService) IsDaemon() bool { return true }

func (s *daemonService) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	if s.done != nil {
		close(s.done)
	}
	client := s.client
	s.mu.Unlock()
	if client != nil {
		return client.Close()
	}
	return nil
}

// directService runs operations in-process (no daemon).
// This is the fallback when daemon is not running.
type directService struct {
	cfg       *config.Config // private snapshot; replaced by ReloadConfig
	providers map[string]backend.Provider
	events    chan *daemonrpc.Event
	mu        sync.RWMutex
	closeOnce sync.Once
}

func newDirectService(cfg *config.Config) *directService {
	// Providers are created lazily by getProvider, which runs inside
	// commands: constructing some of them does network I/O (JMAP
	// authenticates), and NewService is called on the UI goroutine.
	return &directService{
		cfg:       snapshotConfig(cfg),
		providers: make(map[string]backend.Provider),
		events:    make(chan *daemonrpc.Event, 64),
	}
}

// snapshotConfig copies cfg and its account list. The caller (the TUI) keeps
// editing its own config in place — Config.RemoveAccount shifts the Accounts
// slice — and providers hold a pointer to their account, so sharing the slice
// once bound account B's provider to account C after deleting account A.
func snapshotConfig(cfg *config.Config) *config.Config {
	if cfg == nil {
		return &config.Config{}
	}
	c := *cfg
	c.Accounts = slices.Clone(cfg.Accounts)
	return &c
}

// accountLocked returns a copy of the account with the given ID. The caller
// holds s.mu.
func (s *directService) accountLocked(accountID string) (config.Account, bool) {
	if acct := s.cfg.GetAccountByID(accountID); acct != nil {
		return *acct, true
	}
	return config.Account{}, false
}

// getProvider returns the provider for an account, creating it on demand.
// A provider missing from the map (account added after the service was built,
// or a constructor that failed once at startup) used to fail every later
// operation with "no provider for account <id>".
func (s *directService) getProvider(accountID string) (backend.Provider, error) {
	s.mu.RLock()
	p, ok := s.providers[accountID]
	acct, found := s.accountLocked(accountID)
	s.mu.RUnlock()
	if ok {
		return p, nil
	}

	if !found {
		// Config in memory may be stale — reload from disk before giving up.
		if err := s.ReloadConfig(); err != nil {
			return nil, &daemonrpc.Error{Code: daemonrpc.ErrCodeInternal, Message: "no provider for account " + accountID + ": " + err.Error()}
		}
		s.mu.RLock()
		acct, found = s.accountLocked(accountID)
		s.mu.RUnlock()
		if !found {
			return nil, &daemonrpc.Error{Code: daemonrpc.ErrCodeInternal, Message: "no provider for account " + accountID + ": account not found in config"}
		}
	}

	// Build outside the lock (may hit the network), bound to a private copy
	// of the account.
	created, err := backend.New(&acct)
	if err != nil {
		return nil, &daemonrpc.Error{Code: daemonrpc.ErrCodeInternal, Message: "create provider for " + acct.Email + ": " + err.Error()}
	}

	s.mu.Lock()
	if existing, ok := s.providers[accountID]; ok {
		s.mu.Unlock()
		created.Close() //nolint:errcheck,gosec
		return existing, nil
	}
	s.providers[accountID] = created
	s.mu.Unlock()
	return created, nil
}

func (s *directService) FetchEmails(accountID, folder string, limit, offset uint32) ([]backend.Email, error) {
	p, err := s.getProvider(accountID)
	if err != nil {
		return nil, err
	}
	return p.FetchEmails(context.Background(), folder, limit, offset)
}

func (s *directService) FetchEmailBody(accountID, folder string, uid uint32) (string, string, []backend.Attachment, error) {
	p, err := s.getProvider(accountID)
	if err != nil {
		return "", "", nil, err
	}
	return p.FetchEmailBody(context.Background(), folder, uid)
}

func (s *directService) DeleteEmails(accountID, folder string, uids []uint32) error {
	p, err := s.getProvider(accountID)
	if err != nil {
		return err
	}
	return p.DeleteEmails(context.Background(), folder, uids)
}

func (s *directService) ArchiveEmails(accountID, folder string, uids []uint32) error {
	p, err := s.getProvider(accountID)
	if err != nil {
		return err
	}
	return p.ArchiveEmails(context.Background(), folder, uids)
}

func (s *directService) MoveEmails(accountID string, uids []uint32, src, dst string) error {
	p, err := s.getProvider(accountID)
	if err != nil {
		return err
	}
	return p.MoveEmails(context.Background(), uids, src, dst)
}

func (s *directService) MarkRead(accountID, folder string, uids []uint32) error {
	p, err := s.getProvider(accountID)
	if err != nil {
		return err
	}
	for _, uid := range uids {
		if err := p.MarkAsRead(context.Background(), folder, uid); err != nil {
			return err
		}
	}
	return nil
}

func (s *directService) MarkUnread(accountID, folder string, uids []uint32) error {
	p, err := s.getProvider(accountID)
	if err != nil {
		return err
	}
	for _, uid := range uids {
		if err := p.MarkAsUnread(context.Background(), folder, uid); err != nil {
			return err
		}
	}
	return nil
}

func (s *directService) FetchFolders(accountID string) ([]backend.Folder, error) {
	p, err := s.getProvider(accountID)
	if err != nil {
		return nil, err
	}
	return p.FetchFolders(context.Background())
}

func (s *directService) RefreshFolder(_, _ string) error {
	// In direct mode, caller handles refresh via their own fetcher calls.
	return nil
}

func (s *directService) Subscribe(_, _ string) error {
	// No-op in direct mode — TUI manages its own IDLE.
	return nil
}

func (s *directService) Unsubscribe(_, _ string) error {
	return nil
}

func (s *directService) Search(accountID, folder string, query backend.SearchQuery) ([]backend.Email, error) {
	p, err := s.getProvider(accountID)
	if err != nil {
		return nil, err
	}
	return p.Search(context.Background(), folder, query)
}

func (s *directService) ReloadConfig() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return err
	}

	// Drop the providers so the reloaded credentials (keyring / pass_cmd)
	// take effect; they are re-created on next use.
	s.mu.Lock()
	s.cfg = cfg
	old := s.providers
	s.providers = make(map[string]backend.Provider, len(cfg.Accounts))
	s.mu.Unlock()

	for _, p := range old {
		p.Close() //nolint:errcheck,gosec
	}
	return nil
}

func (s *directService) Events() <-chan *daemonrpc.Event {
	return s.events
}

func (s *directService) IsDaemon() bool { return false }

// Close releases the providers. It is idempotent; operations still work
// afterwards (providers are re-created on demand) because a pending
// grace-period action may be flushed after the app has closed the service.
func (s *directService) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		old := s.providers
		s.providers = make(map[string]backend.Provider)
		s.mu.Unlock()
		for _, p := range old {
			p.Close() //nolint:errcheck,gosec
		}
		close(s.events)
	})
	return nil
}

func (s *directService) QueueEmail(accountID string, to, cc, bcc []string, subject, body, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string, signSMIME, encryptSMIME, signPGP, encryptPGP bool, _ int, prebuiltRaw []byte) (string, error) {
	s.mu.RLock()
	acctCopy, found := s.accountLocked(accountID)
	s.mu.RUnlock()
	if !found {
		return "", fmt.Errorf("no account for %s", accountID)
	}
	acct := &acctCopy

	var rawMsg []byte
	if len(prebuiltRaw) > 0 {
		allRecipients := append(append([]string{}, to...), append(cc, bcc...)...)
		if err := sender.DeliverRaw(acct, allRecipients, prebuiltRaw); err != nil {
			return "", err
		}
		rawMsg = prebuiltRaw
	} else {
		var err error
		rawMsg, err = sender.SendEmail(
			acct,
			to,
			cc,
			bcc,
			subject,
			body,
			htmlBody,
			images,
			attachments,
			inReplyTo,
			references,
			signSMIME,
			encryptSMIME,
			signPGP,
			encryptPGP,
		)
		if err != nil {
			return "", err
		}
	}

	if acct.ServiceProvider != "gmail" {
		if err := fetcher.AppendToSentMailbox(acct, rawMsg); err != nil {
			log.Printf("direct: append to sent failed: %v", err)
		}
	}

	return "", nil
}

func (s *directService) CancelEmail(_ string) error {
	return nil
}
