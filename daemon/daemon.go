package daemon

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sort"
	"sync"
	"time"

	udsrpc "github.com/floatpane/go-uds-jsonrpc"
	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonrpc"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/notify"
)

const inboxFolder = "INBOX"

// Daemon is the long-running background process that manages email
// connections, caching, sync, and notifications.
type Daemon struct {
	config    *config.Config
	providers map[string]backend.Provider
	server    *udsrpc.Server
	startTime time.Time

	mu sync.RWMutex

	// Per-client subscriptions: conn → set of "accountID:folder".
	subscriptions map[*daemonrpc.Conn]map[string]struct{}
	subMu         sync.RWMutex

	// Mutex for disk cache updates.
	cacheMu sync.Mutex

	// IMAP IDLE watcher for push notifications.
	idleWatcher *fetcher.IdleWatcher
	idleUpdates chan fetcher.IdleUpdate

	// Background sync cancellation.
	syncCancel context.CancelFunc

	shutdown chan struct{}
	done     chan struct{}
}

// New creates a daemon with the given config.
func New(cfg *config.Config) *Daemon {
	idleUpdates := make(chan fetcher.IdleUpdate, 16)
	d := &Daemon{
		config:        cfg,
		providers:     make(map[string]backend.Provider),
		subscriptions: make(map[*daemonrpc.Conn]map[string]struct{}),
		idleWatcher:   fetcher.NewIdleWatcher(idleUpdates),
		idleUpdates:   idleUpdates,
		shutdown:      make(chan struct{}),
		done:          make(chan struct{}),
	}

	d.server = udsrpc.NewServer()
	d.registerHandlers()
	d.server.OnConnect(func(_ *daemonrpc.Conn) {
		log.Println("daemon: client connected")
	})
	d.server.OnDisconnect(func(conn *daemonrpc.Conn) {
		d.subMu.Lock()
		delete(d.subscriptions, conn)
		d.subMu.Unlock()
		log.Println("daemon: client disconnected")
	})

	return d
}

// registerHandlers wires each RPC method to its handler on the server.
func (d *Daemon) registerHandlers() {
	d.server.Handle(daemonrpc.MethodPing, d.handlePing)
	d.server.Handle(daemonrpc.MethodGetStatus, d.handleGetStatus)
	d.server.Handle(daemonrpc.MethodGetAccounts, d.handleGetAccounts)
	d.server.Handle(daemonrpc.MethodReloadConfig, d.handleReloadConfig)
	d.server.Handle(daemonrpc.MethodFetchEmails, d.handleFetchEmails)
	d.server.Handle(daemonrpc.MethodFetchEmailBody, d.handleFetchEmailBody)
	d.server.Handle(daemonrpc.MethodDeleteEmails, d.handleDeleteEmails)
	d.server.Handle(daemonrpc.MethodArchiveEmails, d.handleArchiveEmails)
	d.server.Handle(daemonrpc.MethodMoveEmails, d.handleMoveEmails)
	d.server.Handle(daemonrpc.MethodMarkRead, d.handleMarkRead)
	d.server.Handle(daemonrpc.MethodFetchFolders, d.handleFetchFolders)
	d.server.Handle(daemonrpc.MethodRefreshFolder, d.handleRefreshFolder)
	d.server.Handle(daemonrpc.MethodSubscribe, d.handleSubscribe)
	d.server.Handle(daemonrpc.MethodUnsubscribe, d.handleUnsubscribe)
}

// Run starts the daemon: creates providers, starts the socket listener,
// starts background sync, and blocks until shutdown.
func (d *Daemon) Run() error {
	d.startTime = time.Now()

	// Ensure runtime directory exists.
	if err := daemonrpc.EnsureRuntimeDir(); err != nil {
		return fmt.Errorf("create runtime dir: %w", err)
	}

	// Check for existing daemon.
	pidPath := daemonrpc.PIDPath()
	if pid, running := IsRunning(pidPath); running {
		return fmt.Errorf("daemon already running (PID %d)", pid)
	}

	// Write PID file.
	if err := WritePID(pidPath); err != nil {
		return fmt.Errorf("write PID file: %w", err)
	}
	defer RemovePID(pidPath) //nolint:errcheck

	// Remove stale socket file.
	sockPath := daemonrpc.SocketPath()
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	// Listen on Unix domain socket.
	listener, err := net.Listen("unix", sockPath) //nolint:noctx
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer listener.Close() //nolint:errcheck

	// Set socket permissions (owner only).
	if err := os.Chmod(sockPath, 0700); err != nil { // #nosec G302
		return fmt.Errorf("set socket permissions: %w", err)
	}

	log.Printf("daemon: listening on %s (PID %d)", sockPath, os.Getpid())

	// Initialize providers for all accounts.
	d.initProviders()

	// Start IMAP IDLE watchers for all accounts.
	d.startIdleWatchers()
	go d.idleEventLoop()

	// Handle OS signals: SIGTERM/SIGINT → shutdown, SIGHUP → reload config.
	stopSignals := udsrpc.HandleSignals(d.Shutdown, func() {
		log.Println("daemon: received SIGHUP, reloading config")
		if err := d.ReloadConfig(); err != nil {
			log.Printf("daemon: config reload failed: %v", err)
		}
	})
	defer stopSignals()

	// Start background sync.
	ctx, cancel := context.WithCancel(context.Background())
	d.syncCancel = cancel
	go d.backgroundSync(ctx)

	// Serve client connections via the shared RPC server. Canceling serveCtx
	// closes the listener and unblocks Serve.
	serveCtx, serveCancel := context.WithCancel(context.Background())
	go func() {
		if err := d.server.Serve(serveCtx, listener); err != nil {
			log.Printf("daemon: serve error: %v", err)
		}
	}()

	// Block until shutdown.
	<-d.shutdown

	// Cleanup.
	log.Println("daemon: shutting down")
	serveCancel()
	for _, conn := range d.server.Clients() {
		conn.Close() //nolint:errcheck,gosec
	}
	if err := d.idleWatcher.StopAllAndWaitTimeout(5 * time.Second); err != nil {
		log.Printf("daemon: %v", err)
	}
	cancel()
	d.closeProviders()

	close(d.done)
	return nil
}

// Shutdown triggers a graceful shutdown.
func (d *Daemon) Shutdown() {
	select {
	case <-d.shutdown:
		// Already shutting down.
	default:
		close(d.shutdown)
	}
}

// ReloadConfig reloads the configuration from disk.
func (d *Daemon) ReloadConfig() error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	d.mu.Lock()
	d.config = cfg
	d.mu.Unlock()

	// Reinitialize providers for new/changed accounts.
	d.initProviders()

	// Notify clients.
	d.broadcastEvent(daemonrpc.EventConfigReloaded, nil)

	log.Println("daemon: config reloaded")
	return nil
}

func (d *Daemon) initProviders() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for i := range d.config.Accounts {
		acct := &d.config.Accounts[i]
		if _, exists := d.providers[acct.ID]; exists {
			continue
		}
		p, err := backend.New(acct)
		if err != nil {
			log.Printf("daemon: failed to create provider for %s: %v", acct.Email, err)
			continue
		}
		d.providers[acct.ID] = p
		log.Printf("daemon: provider ready for %s (%s)", acct.Email, acct.Protocol)
	}
}

func (d *Daemon) closeProviders() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for id, p := range d.providers {
		if err := p.Close(); err != nil {
			log.Printf("daemon: error closing provider %s: %v", id, err)
		}
	}
}

// broadcastEvent sends an event to all connected clients.
func (d *Daemon) broadcastEvent(eventType string, data interface{}) {
	d.server.Broadcast(eventType, data)
}

// broadcastToSubscribers sends an event only to clients subscribed to the given account+folder.
func (d *Daemon) broadcastToSubscribers(accountID, folder, eventType string, data interface{}) {
	key := accountID + ":" + folder
	d.server.BroadcastFunc(eventType, data, func(conn *daemonrpc.Conn) bool {
		d.subMu.RLock()
		defer d.subMu.RUnlock()
		_, ok := d.subscriptions[conn][key]
		return ok
	})
}

// getProvider returns the provider for the given account ID.
func (d *Daemon) getProvider(accountID string) (backend.Provider, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	p, ok := d.providers[accountID]
	if !ok {
		return nil, fmt.Errorf("no provider for account %s", accountID)
	}
	return p, nil
}

// getAccount returns the account config for the given ID.
func (d *Daemon) getAccount(accountID string) *config.Account {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.config.GetAccountByID(accountID)
}

// backgroundSync handles periodic sync and IDLE-like notifications.
func (d *Daemon) backgroundSync(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.syncAllAccounts(ctx)
		}
	}
}

func (d *Daemon) syncAllAccounts(ctx context.Context) {
	d.mu.RLock()
	accounts := make([]config.Account, len(d.config.Accounts))
	copy(accounts, d.config.Accounts)
	d.mu.RUnlock()

	for _, acct := range accounts {
		select {
		case <-ctx.Done():
			return
		default:
		}

		d.broadcastToSubscribers(acct.ID, inboxFolder, daemonrpc.EventSyncStarted, daemonrpc.SyncStartedEvent{
			AccountID: acct.ID,
			Folder:    inboxFolder,
		})

		p, err := d.getProvider(acct.ID)
		if err != nil {
			continue
		}

		emails, err := p.FetchEmails(ctx, inboxFolder, 50, 0)
		if err != nil {
			log.Printf("daemon: sync %s failed: %v", acct.Email, err)
			d.broadcastToSubscribers(acct.ID, inboxFolder, daemonrpc.EventSyncError, daemonrpc.SyncErrorEvent{
				AccountID: acct.ID,
				Folder:    inboxFolder,
				Error:     err.Error(),
			})
			continue
		}

		oldCached, _ := config.LoadFolderEmailCache(inboxFolder)
		oldUIDs := make(map[uint32]struct{}, len(oldCached))
		for _, e := range oldCached {
			if e.AccountID == acct.ID {
				oldUIDs[e.UID] = struct{}{}
			}
		}

		// Cache the fetched emails to disk.
		var cached []config.CachedEmail
		for _, e := range emails {
			cached = append(cached, config.CachedEmail{
				UID:        e.UID,
				From:       e.From,
				To:         e.To,
				Subject:    e.Subject,
				Date:       e.Date,
				MessageID:  e.MessageID,
				InReplyTo:  e.InReplyTo,
				References: e.References,
				AccountID:  e.AccountID,
				IsRead:     e.IsRead,
			})
		}
		if err := d.updateFolderCache(inboxFolder, acct.ID, cached); err != nil {
			log.Printf("daemon: cache update for INBOX failed: %v", err)
		}

		d.broadcastToSubscribers(acct.ID, inboxFolder, daemonrpc.EventSyncComplete, daemonrpc.SyncCompleteEvent{
			AccountID:  acct.ID,
			Folder:     inboxFolder,
			EmailCount: len(emails),
		})

		newCount := 0
		for _, e := range emails {
			if _, seen := oldUIDs[e.UID]; !seen {
				newCount++
			}
		}

		// Send desktop notification if TUI not connected.
		noClients := len(d.server.Clients()) == 0

		if noClients && newCount > 0 {
			if !d.config.DisableNotifications {
				go notify.Send("Matcha", fmt.Sprintf("New mail for %s", acct.FetchEmail)) //nolint:errcheck
			}
		}
	}
}

// startIdleWatchers starts IMAP IDLE watchers for all accounts on INBOX.
func (d *Daemon) startIdleWatchers() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	for i := range d.config.Accounts {
		acct := &d.config.Accounts[i]
		// Only IMAP accounts support IDLE.
		protocol := acct.Protocol
		if protocol == "" {
			protocol = "imap"
		}
		if protocol != "imap" {
			continue
		}
		d.idleWatcher.Watch(acct, inboxFolder)
		log.Printf("daemon: IDLE watcher started for %s", acct.Email)
	}
}

// idleEventLoop listens for IDLE updates and broadcasts them as events.
func (d *Daemon) idleEventLoop() {
	for {
		select {
		case <-d.shutdown:
			return
		case update, ok := <-d.idleUpdates:
			if !ok {
				return
			}
			log.Printf("daemon: IDLE update for %s/%s", update.AccountID, update.FolderName)

			// Desktop notification when no clients connected.
			noClients := len(d.server.Clients()) == 0

			if noClients && !d.config.DisableNotifications {
				accountName := update.AccountID
				if acct := d.config.GetAccountByID(update.AccountID); acct != nil {
					accountName = acct.Email
				}
				go notify.Send("Matcha", fmt.Sprintf("New mail in %s (%s)", update.FolderName, accountName)) //nolint:errcheck
			}

			// Broadcast to subscribed clients.
			d.broadcastToSubscribers(update.AccountID, update.FolderName, daemonrpc.EventNewMail, daemonrpc.NewMailEvent{
				AccountID: update.AccountID,
				Folder:    update.FolderName,
			})

			// Fetch and cache emails so they're fresh when TUI next connects.
			go d.fetchAndCache(update.AccountID, update.FolderName)
		}
	}
}

// fetchAndCache fetches emails for an account/folder and saves to disk cache.
func (d *Daemon) fetchAndCache(accountID, folder string) {
	acct := d.getAccount(accountID)
	if acct == nil {
		return
	}

	emails, err := fetcher.FetchFolderEmails(acct, folder, 50, 0)
	if err != nil {
		log.Printf("daemon: cache fetch for %s/%s failed: %v", accountID, folder, err)
		return
	}

	// Convert to cache format and save.
	var cached []config.CachedEmail
	for _, e := range emails {
		cached = append(cached, config.CachedEmail{
			UID:        e.UID,
			From:       e.From,
			To:         e.To,
			Subject:    e.Subject,
			Date:       e.Date,
			MessageID:  e.MessageID,
			InReplyTo:  e.InReplyTo,
			References: e.References,
			AccountID:  e.AccountID,
			IsRead:     e.IsRead,
		})
	}

	if err := d.updateFolderCache(folder, accountID, cached); err != nil {
		log.Printf("daemon: cache update for %s failed: %v", folder, err)
		return
	}

	log.Printf("daemon: cached %d emails for %s/%s", len(cached), accountID, folder)

	// Also notify subscribers that emails were updated.
	d.broadcastToSubscribers(accountID, folder, daemonrpc.EventSyncComplete, daemonrpc.SyncCompleteEvent{
		AccountID:  accountID,
		Folder:     folder,
		EmailCount: len(emails),
	})
}

// updateFolderCache safely merges new emails for a specific account into the existing folder cache.
func (d *Daemon) updateFolderCache(folderName, accountID string, newEmails []config.CachedEmail) error {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()

	// Load existing cache
	existing, _ := config.LoadFolderEmailCache(folderName) // Ignore error, assume empty if missing

	// Filter out old emails for this account
	var merged []config.CachedEmail
	for _, e := range existing {
		if e.AccountID != accountID {
			merged = append(merged, e)
		}
	}

	// Append new emails
	merged = append(merged, newEmails...)

	// Sort newest first
	sort.Slice(merged, func(i, j int) bool {
		return merged[i].Date.After(merged[j].Date)
	})

	// Save merged cache
	return config.SaveFolderEmailCache(folderName, merged)
}
