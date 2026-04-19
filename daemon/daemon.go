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

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonrpc"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/notify"
)

// Daemon is the long-running background process that manages email
// connections, caching, sync, and notifications.
type Daemon struct {
	config    *config.Config
	providers map[string]backend.Provider
	listener  net.Listener
	startTime time.Time

	// Connected TUI/CLI clients.
	clients map[*daemonrpc.Conn]struct{}
	mu      sync.RWMutex

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
	return &Daemon{
		config:        cfg,
		providers:     make(map[string]backend.Provider),
		clients:       make(map[*daemonrpc.Conn]struct{}),
		subscriptions: make(map[*daemonrpc.Conn]map[string]struct{}),
		idleWatcher:   fetcher.NewIdleWatcher(idleUpdates),
		idleUpdates:   idleUpdates,
		shutdown:      make(chan struct{}),
		done:          make(chan struct{}),
	}
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
	defer RemovePID(pidPath)

	// Remove stale socket file.
	sockPath := daemonrpc.SocketPath()
	os.Remove(sockPath)

	// Listen on Unix domain socket.
	var err error
	d.listener, err = net.Listen("unix", sockPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer d.listener.Close()

	// Set socket permissions (owner only).
	os.Chmod(sockPath, 0700)

	log.Printf("daemon: listening on %s (PID %d)", sockPath, os.Getpid())

	// Initialize providers for all accounts.
	d.initProviders()

	// Start IMAP IDLE watchers for all accounts.
	d.startIdleWatchers()
	go d.idleEventLoop()

	// Start signal handler.
	go d.handleSignals()

	// Start background sync.
	ctx, cancel := context.WithCancel(context.Background())
	d.syncCancel = cancel
	go d.backgroundSync(ctx)

	// Accept client connections.
	go d.acceptLoop()

	// Block until shutdown.
	<-d.shutdown

	// Cleanup.
	log.Println("daemon: shutting down")
	d.listener.Close()
	d.idleWatcher.StopAll()
	cancel()
	d.closeAllClients()
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

func (d *Daemon) acceptLoop() {
	for {
		conn, err := d.listener.Accept()
		if err != nil {
			select {
			case <-d.shutdown:
				return
			default:
				log.Printf("daemon: accept error: %v", err)
				continue
			}
		}
		rpcConn := daemonrpc.NewConn(conn)
		d.addClient(rpcConn)
		go d.handleClient(rpcConn)
	}
}

func (d *Daemon) handleClient(conn *daemonrpc.Conn) {
	defer d.removeClient(conn)
	defer conn.Close()

	for {
		msg, err := conn.ReceiveMessage()
		if err != nil {
			// Client disconnected or read error.
			return
		}
		if msg.Request != nil {
			d.handleRequest(conn, msg.Request)
		}
	}
}

func (d *Daemon) addClient(conn *daemonrpc.Conn) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.clients[conn] = struct{}{}
	log.Println("daemon: client connected")
}

func (d *Daemon) removeClient(conn *daemonrpc.Conn) {
	d.mu.Lock()
	delete(d.clients, conn)
	d.mu.Unlock()

	d.subMu.Lock()
	delete(d.subscriptions, conn)
	d.subMu.Unlock()

	log.Println("daemon: client disconnected")
}

func (d *Daemon) closeAllClients() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for conn := range d.clients {
		conn.Close()
	}
	d.clients = make(map[*daemonrpc.Conn]struct{})
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
	d.mu.RLock()
	defer d.mu.RUnlock()
	for conn := range d.clients {
		if err := conn.SendEvent(eventType, data); err != nil {
			log.Printf("daemon: broadcast error: %v", err)
		}
	}
}

// broadcastToSubscribers sends an event only to clients subscribed to the given account+folder.
func (d *Daemon) broadcastToSubscribers(accountID, folder, eventType string, data interface{}) {
	key := accountID + ":" + folder
	d.subMu.RLock()
	defer d.subMu.RUnlock()

	for conn, subs := range d.subscriptions {
		if _, ok := subs[key]; ok {
			if err := conn.SendEvent(eventType, data); err != nil {
				log.Printf("daemon: subscriber broadcast error: %v", err)
			}
		}
	}
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

		d.broadcastToSubscribers(acct.ID, "INBOX", daemonrpc.EventSyncStarted, daemonrpc.SyncStartedEvent{
			AccountID: acct.ID,
			Folder:    "INBOX",
		})

		p, err := d.getProvider(acct.ID)
		if err != nil {
			continue
		}

		emails, err := p.FetchEmails(ctx, "INBOX", 50, 0)
		if err != nil {
			log.Printf("daemon: sync %s failed: %v", acct.Email, err)
			d.broadcastToSubscribers(acct.ID, "INBOX", daemonrpc.EventSyncError, daemonrpc.SyncErrorEvent{
				AccountID: acct.ID,
				Folder:    "INBOX",
				Error:     err.Error(),
			})
			continue
		}

		// Cache the fetched emails to disk.
		var cached []config.CachedEmail
		for _, e := range emails {
			cached = append(cached, config.CachedEmail{
				UID:       e.UID,
				From:      e.From,
				To:        e.To,
				Subject:   e.Subject,
				Date:      e.Date,
				MessageID: e.MessageID,
				AccountID: e.AccountID,
				IsRead:    e.IsRead,
			})
		}
		if err := d.updateFolderCache("INBOX", acct.ID, cached); err != nil {
			log.Printf("daemon: cache update for INBOX failed: %v", err)
		}

		d.broadcastToSubscribers(acct.ID, "INBOX", daemonrpc.EventSyncComplete, daemonrpc.SyncCompleteEvent{
			AccountID:  acct.ID,
			Folder:     "INBOX",
			EmailCount: len(emails),
		})

		// Send desktop notification if TUI not connected.
		d.mu.RLock()
		noClients := len(d.clients) == 0
		d.mu.RUnlock()

		if noClients && len(emails) > 0 {
			if !d.config.DisableNotifications {
				go notify.Send("Matcha", fmt.Sprintf("New mail for %s", acct.Email))
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
		d.idleWatcher.Watch(acct, "INBOX")
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
			d.mu.RLock()
			noClients := len(d.clients) == 0
			d.mu.RUnlock()

			if noClients && !d.config.DisableNotifications {
				accountName := update.AccountID
				if acct := d.config.GetAccountByID(update.AccountID); acct != nil {
					accountName = acct.Email
				}
				go notify.Send("Matcha", fmt.Sprintf("New mail in %s (%s)", update.FolderName, accountName))
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
			UID:       e.UID,
			From:      e.From,
			To:        e.To,
			Subject:   e.Subject,
			Date:      e.Date,
			MessageID: e.MessageID,
			AccountID: e.AccountID,
			IsRead:    e.IsRead,
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
