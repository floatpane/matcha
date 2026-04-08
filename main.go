package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/backend"
	_ "github.com/floatpane/matcha/backend/imap"
	_ "github.com/floatpane/matcha/backend/jmap"
	_ "github.com/floatpane/matcha/backend/pop3"
	"github.com/floatpane/matcha/clib"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/notify"
	"github.com/floatpane/matcha/plugin"
	"github.com/floatpane/matcha/sender"
	"github.com/floatpane/matcha/theme"
	"github.com/floatpane/matcha/tui"
	"github.com/google/uuid"
	lua "github.com/yuin/gopher-lua"
)

const (
	initialEmailLimit = 50
	paginationLimit   = 50
	maxCacheEmails    = 100
)

// Version variables are injected by the build (GoReleaser ldflags).
// They default to "dev" when not set by the build system.
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// UpdateAvailableMsg is sent into the TUI when a newer release is detected.
type UpdateAvailableMsg struct {
	Latest  string
	Current string
}

// internal struct for parsing GitHub release JSON.
type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type mainModel struct {
	current       tea.Model
	previousModel tea.Model
	config        *config.Config
	plugins       *plugin.Manager
	// Folder-based email storage
	folderEmails map[string][]fetcher.Email // key: folderName
	folderInbox  *tui.FolderInbox
	// Legacy fields kept for email actions
	emails       []fetcher.Email
	emailsByAcct map[string][]fetcher.Email
	width        int
	height       int
	err          error
	// IMAP IDLE
	idleWatcher *fetcher.IdleWatcher
	idleUpdates chan fetcher.IdleUpdate
	// Multi-protocol backend providers (keyed by account ID)
	providers map[string]backend.Provider
	// Plugin prompt waiting for user input
	pendingPrompt *plugin.PendingPrompt
}

func newInitialModel(cfg *config.Config) *mainModel {
	idleUpdates := make(chan fetcher.IdleUpdate, 16)
	initialModel := &mainModel{
		emailsByAcct: make(map[string][]fetcher.Email),
		folderEmails: make(map[string][]fetcher.Email),
		idleUpdates:  idleUpdates,
		idleWatcher:  fetcher.NewIdleWatcher(idleUpdates),
		providers:    make(map[string]backend.Provider),
	}

	if cfg == nil || !cfg.HasAccounts() {
		hideTips := false
		if cfg != nil {
			hideTips = cfg.HideTips
		}
		initialModel.current = tui.NewLogin(hideTips)
	} else {
		initialModel.current = tui.NewChoice()
		initialModel.config = cfg
	}
	return initialModel
}

// ensureProviders creates backend providers for all configured accounts.
func (m *mainModel) ensureProviders() {
	if m.config == nil {
		return
	}
	for _, acct := range m.config.Accounts {
		if _, ok := m.providers[acct.ID]; ok {
			continue
		}
		p, err := backend.New(&acct)
		if err != nil {
			log.Printf("backend: failed to create provider for %s: %v", acct.Email, err)
			continue
		}
		m.providers[acct.ID] = p
	}
}

// getProvider returns the backend provider for the given account.
func (m *mainModel) getProvider(acct *config.Account) backend.Provider {
	if acct == nil {
		return nil
	}
	return m.providers[acct.ID]
}

func (m *mainModel) Init() tea.Cmd {
	return tea.Batch(m.current.Init(), checkForUpdatesCmd())
}

func (m *mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	m.current, cmd = m.current.Update(msg)
	cmds = append(cmds, cmd)

	// Fire composer_updated hook on key presses when the composer is active
	if keyMsg, isKey := msg.(tea.KeyPressMsg); isKey {
		if composer, ok := m.current.(*tui.Composer); ok && m.plugins != nil {
			m.plugins.CallComposerHook(plugin.HookComposerUpdated, composer.GetBody(), composer.GetSubject(), composer.GetTo(), composer.GetCc(), composer.GetBcc())
			m.syncPluginStatus()
			m.applyPluginFields(composer)
		}

		// Check plugin key bindings for the current view
		if m.plugins != nil {
			m.handlePluginKeyBinding(keyMsg)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			m.idleWatcher.StopAll()
			return m, tea.Quit
		}
		if msg.String() == "esc" {
			switch m.current.(type) {
			case *tui.FilePicker:
				return m, func() tea.Msg { return tui.CancelFilePickerMsg{} }
			case *tui.FolderInbox, *tui.Inbox, *tui.Login:
				m.idleWatcher.StopAll()
				m.current = tui.NewChoice()
				m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
				return m, m.current.Init()
			}
		}

	case tui.BackToInboxMsg:
		if m.folderInbox != nil {
			m.current = m.folderInbox
		} else {
			m.current = tui.NewChoice()
			m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		return m, nil

	case tui.BackToMailboxMsg:
		// Ensure kitty graphics are cleared when leaving email view
		tui.ClearKittyGraphics()
		if m.folderInbox != nil {
			m.current = m.folderInbox
			return m, nil
		}
		m.current = tui.NewChoice()
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, nil

	case tui.DiscardDraftMsg:
		// Save draft to disk
		if msg.ComposerState != nil {
			draft := msg.ComposerState.ToDraft()

			if err := config.SaveDraft(draft); err != nil {
				log.Printf("Error saving draft: %v", err)
			}

		}
		m.current = tui.NewChoice()
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.OAuth2CompleteMsg:
		if msg.Err != nil {
			log.Printf("OAuth2 authorization failed: %v", msg.Err)
		}
		// After OAuth2 flow, go to the choice menu so user can proceed
		m.current = tui.NewChoice()
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.Credentials:
		// Split FetchEmail by commas to support multiple fetch addresses.
		// Each address creates a separate account sharing the same login credentials.
		fetchEmails := []string{""}
		if msg.FetchEmail != "" {
			fetchEmails = fetchEmails[:0]
			for _, fe := range strings.Split(msg.FetchEmail, ",") {
				if trimmed := strings.TrimSpace(fe); trimmed != "" {
					fetchEmails = append(fetchEmails, trimmed)
				}
			}
			if len(fetchEmails) == 0 {
				fetchEmails = []string{""}
			}
		}

		if m.config == nil {
			m.config = &config.Config{}
		}

		// Check if we're editing an existing account
		isEdit := false
		var lastAccount config.Account
		if login, ok := m.current.(*tui.Login); ok && login.IsEditMode() {
			isEdit = true
			existingID := login.GetAccountID()

			account := config.Account{
				ID:              existingID,
				Name:            msg.Name,
				Email:           msg.Host,
				Password:        msg.Password,
				ServiceProvider: msg.Provider,
				FetchEmail:      fetchEmails[0],
				AuthMethod:      msg.AuthMethod,
				Protocol:        msg.Protocol,
				JMAPEndpoint:    msg.JMAPEndpoint,
				POP3Server:      msg.POP3Server,
				POP3Port:        msg.POP3Port,
			}

			if msg.Provider == "custom" || msg.Protocol == "pop3" {
				account.IMAPServer = msg.IMAPServer
				account.IMAPPort = msg.IMAPPort
				account.SMTPServer = msg.SMTPServer
				account.SMTPPort = msg.SMTPPort
			}

			if account.FetchEmail == "" && account.Email != "" {
				account.FetchEmail = account.Email
			}

			// Find and update the existing account, preserving S/MIME settings
			for i, acc := range m.config.Accounts {
				if acc.ID == existingID {
					account.SMIMECert = acc.SMIMECert
					account.SMIMEKey = acc.SMIMEKey
					account.SMIMESignByDefault = acc.SMIMESignByDefault
					if account.Password == "" {
						account.Password = acc.Password
					}
					m.config.Accounts[i] = account
					break
				}
			}
			lastAccount = account
		} else {
			// New account: create one account per fetch email address
			for _, fe := range fetchEmails {
				account := config.Account{
					ID:              uuid.New().String(),
					Name:            msg.Name,
					Email:           msg.Host,
					Password:        msg.Password,
					ServiceProvider: msg.Provider,
					FetchEmail:      fe,
					AuthMethod:      msg.AuthMethod,
					Protocol:        msg.Protocol,
					JMAPEndpoint:    msg.JMAPEndpoint,
					POP3Server:      msg.POP3Server,
					POP3Port:        msg.POP3Port,
				}

				if msg.Provider == "custom" || msg.Protocol == "pop3" {
					account.IMAPServer = msg.IMAPServer
					account.IMAPPort = msg.IMAPPort
					account.SMTPServer = msg.SMTPServer
					account.SMTPPort = msg.SMTPPort
				}

				if account.FetchEmail == "" && account.Email != "" {
					account.FetchEmail = account.Email
				}

				m.config.AddAccount(account)
				lastAccount = account
			}
		}

		if err := config.SaveConfig(m.config); err != nil {
			log.Printf("could not save config: %v", err)
			return m, tea.Quit
		}

		// If OAuth2, launch the authorization flow after saving the account
		if lastAccount.IsOAuth2() {
			email := lastAccount.Email
			return m, func() tea.Msg {
				err := config.RunOAuth2Flow(email, "", "")
				return tui.OAuth2CompleteMsg{Email: email, Err: err}
			}
		}

		if isEdit {
			m.current = tui.NewSettings(m.config)
		} else {
			m.current = tui.NewChoice()
		}
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToInboxMsg:
		if m.config == nil || !m.config.HasAccounts() {
			hideTips := false
			if m.config != nil {
				hideTips = m.config.HideTips
			}
			m.current = tui.NewLogin(hideTips)
			return m, m.current.Init()
		}
		m.ensureProviders()
		// Load cached folders from all accounts, merge unique names
		seen := make(map[string]bool)
		var cachedFolders []string
		for _, acc := range m.config.Accounts {
			for _, f := range config.GetCachedFolders(acc.ID) {
				if !seen[f] {
					seen[f] = true
					cachedFolders = append(cachedFolders, f)
				}
			}
		}
		if len(cachedFolders) == 0 {
			cachedFolders = []string{"INBOX"}
		}
		m.folderInbox = tui.NewFolderInbox(cachedFolders, m.config.Accounts)
		// Use cached INBOX emails for instant display (memory first, then disk)
		if cached, ok := m.folderEmails["INBOX"]; ok && len(cached) > 0 {
			m.folderInbox.SetEmails(cached, m.config.Accounts)
		} else if diskCached := loadFolderEmailsFromCache("INBOX"); len(diskCached) > 0 {
			m.folderEmails["INBOX"] = diskCached
			m.emails = diskCached
			m.emailsByAcct = make(map[string][]fetcher.Email)
			for _, email := range diskCached {
				m.emailsByAcct[email.AccountID] = append(m.emailsByAcct[email.AccountID], email)
			}
			m.folderInbox.SetEmails(diskCached, m.config.Accounts)
		}
		m.current = m.folderInbox
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		// Start IDLE watchers for all accounts on INBOX
		for i := range m.config.Accounts {
			m.idleWatcher.Watch(&m.config.Accounts[i], "INBOX")
		}
		// Fetch folders and INBOX emails in parallel (background refresh)
		return m, tea.Batch(
			m.current.Init(),
			fetchFoldersCmd(m.config),
			fetchFolderEmailsCmd(m.config, "INBOX"),
			listenForIdleUpdates(m.idleUpdates),
		)

	case tui.FoldersFetchedMsg:
		if m.folderInbox == nil {
			return m, nil
		}
		var folderNames []string
		for _, f := range msg.MergedFolders {
			folderNames = append(folderNames, f.Name)
		}
		m.folderInbox.SetFolders(folderNames)
		// Cache folder lists per account
		for accID, folders := range msg.FoldersByAccount {
			var names []string
			for _, f := range folders {
				names = append(names, f.Name)
			}
			go config.SaveAccountFolders(accID, names)
		}
		return m, nil

	case tui.SwitchFolderMsg:
		if m.config == nil {
			return m, nil
		}
		// Update IDLE watchers to monitor the new folder
		for i := range m.config.Accounts {
			// Only start IDLE for accounts that actually have this folder
			folders := config.GetCachedFolders(m.config.Accounts[i].ID)
			if !slices.Contains(folders, msg.FolderName) {
				m.idleWatcher.Stop(m.config.Accounts[i].ID)
				continue
			}
			m.idleWatcher.Watch(&m.config.Accounts[i], msg.FolderName)
		}
		if m.plugins != nil {
			m.plugins.CallFolderHook(plugin.HookFolderChanged, msg.FolderName)
			m.syncPluginStatus()
			m.syncPluginKeyBindings()
		}
		// Use in-memory cache if available
		if cached, ok := m.folderEmails[msg.FolderName]; ok {
			m.emails = cached
			m.emailsByAcct = make(map[string][]fetcher.Email)
			for _, email := range cached {
				m.emailsByAcct[email.AccountID] = append(m.emailsByAcct[email.AccountID], email)
			}
			if m.folderInbox != nil {
				m.folderInbox.SetEmails(cached, m.config.Accounts)
				m.folderInbox.GetInbox().SetFolderName(msg.FolderName)
				m.folderInbox.SetLoadingEmails(false)
			}
			return m, m.pluginNotifyCmd()
		}
		// Fall back to disk cache for instant display, then fetch fresh in background
		if diskCached := loadFolderEmailsFromCache(msg.FolderName); len(diskCached) > 0 {
			m.folderEmails[msg.FolderName] = diskCached
			m.emails = diskCached
			m.emailsByAcct = make(map[string][]fetcher.Email)
			for _, email := range diskCached {
				m.emailsByAcct[email.AccountID] = append(m.emailsByAcct[email.AccountID], email)
			}
			if m.folderInbox != nil {
				m.folderInbox.SetEmails(diskCached, m.config.Accounts)
				m.folderInbox.GetInbox().SetFolderName(msg.FolderName)
				m.folderInbox.SetLoadingEmails(false)
			}
			// Still fetch fresh emails in background
			return m, tea.Batch(fetchFolderEmailsCmd(m.config, msg.FolderName), m.pluginNotifyCmd())
		}
		if m.folderInbox != nil {
			m.folderInbox.SetLoadingEmails(true)
		}
		return m, tea.Batch(fetchFolderEmailsCmd(m.config, msg.FolderName), m.pluginNotifyCmd())

	case tui.PluginNotifyMsg:
		m.previousModel = m.current
		m.current = tui.NewStatus(msg.Message)
		dur := time.Duration(msg.Duration * float64(time.Second))
		if dur <= 0 {
			dur = 2 * time.Second
		}
		return m, tea.Tick(dur, func(t time.Time) tea.Msg {
			return tui.RestoreViewMsg{}
		})

	case tui.PluginPromptSubmitMsg:
		if m.pendingPrompt != nil {
			if composer, ok := m.current.(*tui.Composer); ok {
				composer.HidePluginPrompt()
				m.plugins.ResolvePrompt(m.pendingPrompt, msg.Value)
				m.applyPluginFields(composer)
				m.syncPluginStatus()
			}
			m.pendingPrompt = nil
		}
		return m, nil

	case tui.PluginPromptCancelMsg:
		if composer, ok := m.current.(*tui.Composer); ok {
			composer.HidePluginPrompt()
		}
		m.pendingPrompt = nil
		return m, nil

	case tui.FolderEmailsFetchedMsg:
		if m.folderInbox == nil {
			return m, nil
		}
		// Call plugin hooks for received emails
		if m.plugins != nil {
			for _, email := range msg.Emails {
				t := m.plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, msg.FolderName)
				m.plugins.CallHook(plugin.HookEmailReceived, t)
			}
		}
		// Always cache in memory and to disk
		m.folderEmails[msg.FolderName] = msg.Emails
		go saveFolderEmailsToCache(msg.FolderName, msg.Emails)
		// Only update the view if the user is still on this folder
		if m.folderInbox.GetCurrentFolder() != msg.FolderName {
			return m, nil
		}
		m.emails = msg.Emails
		m.emailsByAcct = make(map[string][]fetcher.Email)
		for _, email := range msg.Emails {
			m.emailsByAcct[email.AccountID] = append(m.emailsByAcct[email.AccountID], email)
		}
		m.folderInbox.SetEmails(msg.Emails, m.config.Accounts)
		m.folderInbox.GetInbox().SetFolderName(msg.FolderName)
		m.folderInbox.SetLoadingEmails(false)
		m.syncPluginStatus()
		m.syncPluginKeyBindings()
		return m, m.pluginNotifyCmd()

	case tui.FetchFolderMoreEmailsMsg:
		if msg.AccountID == "" || m.config == nil {
			return m, nil
		}
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			return m, nil
		}
		limit := uint32(paginationLimit)
		if msg.Limit > 0 {
			limit = msg.Limit
		}
		return m, tea.Batch(
			func() tea.Msg { return tui.FetchingMoreEmailsMsg{} },
			fetchFolderEmailsPaginatedCmd(account, msg.FolderName, limit, msg.Offset),
		)

	case tui.FolderEmailsAppendedMsg:
		// Ignore stale appends for a folder the user has moved away from
		if m.folderInbox == nil || m.folderInbox.GetCurrentFolder() != msg.FolderName {
			return m, nil
		}
		m.folderInbox.Update(msg)
		// Update local stores and per-folder cache
		for _, email := range msg.Emails {
			m.emails = append(m.emails, email)
			m.emailsByAcct[email.AccountID] = append(m.emailsByAcct[email.AccountID], email)
		}
		m.folderEmails[msg.FolderName] = append(m.folderEmails[msg.FolderName], msg.Emails...)
		go saveFolderEmailsToCache(msg.FolderName, m.folderEmails[msg.FolderName])
		return m, nil

	case tui.MoveEmailToFolderMsg:
		if m.config == nil {
			return m, nil
		}
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			return m, nil
		}
		m.previousModel = m.current
		m.current = tui.NewStatus("Moving email...")
		return m, tea.Batch(m.current.Init(), moveEmailToFolderCmd(account, msg.UID, msg.AccountID, msg.SourceFolder, msg.DestFolder))

	case tui.EmailMovedMsg:
		if msg.Err != nil {
			log.Printf("Move failed: %v", msg.Err)
			if m.folderInbox != nil {
				m.previousModel = m.folderInbox
			}
			m.current = tui.NewStatus(fmt.Sprintf("Error: %v", msg.Err))
			return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
				return tui.RestoreViewMsg{}
			})
		}
		// Remove email from current view
		if m.folderInbox != nil {
			m.folderInbox.RemoveEmail(msg.UID, msg.AccountID)
			m.current = m.folderInbox
		}
		return m, nil

	case tui.CachedEmailsLoadedMsg:
		// Cache is no longer used for the folder-based inbox flow
		// This handler is kept for backwards compatibility but simply fetches normally
		if m.folderInbox == nil {
			return m, nil
		}
		return m, fetchFolderEmailsCmd(m.config, m.folderInbox.GetCurrentFolder())

	case tui.IdleNewMailMsg:
		// Send desktop notification for new mail (if enabled)
		if m.config == nil || !m.config.DisableNotifications {
			accountName := msg.AccountID
			if m.config != nil {
				if acc := m.config.GetAccountByID(msg.AccountID); acc != nil {
					accountName = acc.Email
				}
			}
			go notify.Send("Matcha", fmt.Sprintf("New mail in %s (%s)", msg.FolderName, accountName))
		}

		// IDLE detected new mail — refetch the folder if we're viewing it
		if m.folderInbox != nil && m.folderInbox.GetCurrentFolder() == msg.FolderName {
			return m, tea.Batch(
				fetchFolderEmailsCmd(m.config, msg.FolderName),
				listenForIdleUpdates(m.idleUpdates),
			)
		}
		// Re-subscribe even if not viewing the affected folder
		return m, listenForIdleUpdates(m.idleUpdates)

	case tui.RequestRefreshMsg:
		// Folder-based refresh: clear folder cache and refetch
		if msg.FolderName != "" && m.config != nil {
			delete(m.folderEmails, msg.FolderName)
			if m.folderInbox != nil {
				m.folderInbox.SetRefreshing(true)
			}
			return m, fetchFolderEmailsCmd(m.config, msg.FolderName)
		}
		return m, tea.Batch(
			func() tea.Msg { return tui.RefreshingEmailsMsg{Mailbox: msg.Mailbox} },
			refreshEmails(m.config, msg.Mailbox, msg.Counts),
		)

	case tui.EmailsRefreshedMsg:
		// Merge refreshed emails with any paginated emails already loaded.
		for accID, refreshed := range msg.EmailsByAccount {
			refreshedUIDs := make(map[uint32]struct{}, len(refreshed))
			for _, e := range refreshed {
				refreshedUIDs[e.UID] = struct{}{}
			}
			if existing, ok := m.emailsByAcct[accID]; ok {
				for _, e := range existing {
					if _, found := refreshedUIDs[e.UID]; !found {
						refreshed = append(refreshed, e)
					}
				}
			}
			m.emailsByAcct[accID] = refreshed
		}
		m.emails = flattenAndSort(m.emailsByAcct)

		// Update folder inbox if it exists
		if m.folderInbox != nil {
			m.folderInbox.SetEmails(m.emails, m.config.Accounts)
			m.folderInbox.GetInbox().Update(msg)
		}
		return m, nil

	case tui.AllEmailsFetchedMsg:
		m.emailsByAcct = msg.EmailsByAccount
		m.emails = flattenAndSort(msg.EmailsByAccount)

		if m.folderInbox != nil {
			m.folderInbox.SetEmails(m.emails, m.config.Accounts)
			m.folderInbox.SetLoadingEmails(false)
		}
		return m, nil

	case tui.EmailsFetchedMsg:
		if m.emailsByAcct == nil {
			m.emailsByAcct = make(map[string][]fetcher.Email)
		}
		m.emailsByAcct[msg.AccountID] = msg.Emails
		m.emails = flattenAndSort(m.emailsByAcct)
		if m.folderInbox != nil {
			m.folderInbox.SetEmails(m.emails, m.config.Accounts)
		}
		return m, nil

	case tui.FetchMoreEmailsMsg:
		if msg.AccountID == "" {
			return m, nil
		}
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			return m, nil
		}
		limit := uint32(paginationLimit)
		if msg.Limit > 0 {
			limit = msg.Limit
		}
		folderName := "INBOX"
		if m.folderInbox != nil {
			folderName = m.folderInbox.GetCurrentFolder()
		}
		return m, tea.Batch(
			func() tea.Msg { return tui.FetchingMoreEmailsMsg{} },
			fetchFolderEmailsPaginatedCmd(account, folderName, limit, msg.Offset),
		)

	case tui.EmailsAppendedMsg:
		if m.emailsByAcct == nil {
			m.emailsByAcct = make(map[string][]fetcher.Email)
		}
		unique := filterUnique(m.emailsByAcct[msg.AccountID], msg.Emails)
		m.emailsByAcct[msg.AccountID] = append(m.emailsByAcct[msg.AccountID], unique...)
		m.emails = append(m.emails, unique...)
		return m, nil

	case tui.GoToSendMsg:
		hideTips := false
		if m.config != nil {
			hideTips = m.config.HideTips
		}
		if m.config != nil && len(m.config.Accounts) > 0 {
			firstAccount := m.config.GetFirstAccount()
			composer := tui.NewComposerWithAccounts(m.config.Accounts, firstAccount.ID, msg.To, msg.Subject, msg.Body, hideTips)
			m.current = composer
		} else {
			m.current = tui.NewComposer("", msg.To, msg.Subject, msg.Body, hideTips)
		}
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.syncPluginKeyBindings()
		return m, m.current.Init()

	case tui.GoToDraftsMsg:
		drafts := config.GetAllDrafts()
		m.current = tui.NewDrafts(drafts)
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.OpenDraftMsg:
		var accounts []config.Account
		hideTips := false
		if m.config != nil {
			accounts = m.config.Accounts
			hideTips = m.config.HideTips
		}
		composer := tui.NewComposerFromDraft(msg.Draft, accounts, hideTips)
		m.current = composer
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.syncPluginKeyBindings()
		return m, m.current.Init()

	case tui.DeleteSavedDraftMsg:
		go func() {
			if err := config.DeleteDraft(msg.DraftID); err != nil {
				log.Printf("Error deleting draft: %v", err)
			}
		}()
		// Send message back to drafts view
		m.current, cmd = m.current.Update(tui.DraftDeletedMsg{DraftID: msg.DraftID})
		return m, cmd

	case tui.GoToSettingsMsg:
		m.current = tui.NewSettings(m.config)
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToAddAccountMsg:
		hideTips := false
		if m.config != nil {
			hideTips = m.config.HideTips
		}
		m.current = tui.NewLogin(hideTips)
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToAddMailingListMsg:
		m.current = tui.NewMailingListEditor()
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToEditAccountMsg:
		hideTips := false
		if m.config != nil {
			hideTips = m.config.HideTips
		}
		login := tui.NewLogin(hideTips)
		login.SetEditMode(msg.AccountID, msg.Protocol, msg.Provider, msg.Name, msg.Email, msg.FetchEmail, msg.IMAPServer, msg.IMAPPort, msg.SMTPServer, msg.SMTPPort, msg.JMAPEndpoint, msg.POP3Server, msg.POP3Port)
		m.current = login
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToEditMailingListMsg:
		editor := tui.NewMailingListEditor()
		editor.SetEditMode(msg.Index, msg.Name, msg.Addresses)
		m.current = editor
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.SaveMailingListMsg:
		if m.config != nil {
			var addrs []string
			for _, part := range strings.Split(msg.Addresses, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					addrs = append(addrs, trimmed)
				}
			}
			if msg.EditIndex >= 0 && msg.EditIndex < len(m.config.MailingLists) {
				m.config.MailingLists[msg.EditIndex] = config.MailingList{
					Name:      msg.Name,
					Addresses: addrs,
				}
			} else {
				m.config.MailingLists = append(m.config.MailingLists, config.MailingList{
					Name:      msg.Name,
					Addresses: addrs,
				})
			}
			if err := config.SaveConfig(m.config); err != nil {
				log.Printf("could not save config: %v", err)
			}
		}
		// Return to settings
		m.current = tui.NewSettings(m.config)
		// Try to navigate to the mailing list view internally if possible, but NewSettings will go to SettingsMain by default.
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToSignatureEditorMsg:
		m.current = tui.NewSignatureEditor()
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToChoiceMenuMsg:
		m.current = tui.NewChoice()
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.DeleteAccountMsg:
		if m.config != nil {
			m.config.RemoveAccount(msg.AccountID)
			if err := config.SaveConfig(m.config); err != nil {
				log.Printf("could not save config: %v", err)
			}
			// Remove emails for this account
			delete(m.emailsByAcct, msg.AccountID)

			// Rebuild all emails
			var allEmails []fetcher.Email
			for _, emails := range m.emailsByAcct {
				allEmails = append(allEmails, emails...)
			}
			m.emails = allEmails

			// Go back to settings
			m.current = tui.NewSettings(m.config)
			m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		return m, m.current.Init()

	case tui.ViewEmailMsg:
		email := m.getEmailByUIDAndAccount(msg.UID, msg.AccountID, msg.Mailbox)
		if email == nil {
			return m, nil
		}
		folderName := "INBOX"
		if m.folderInbox != nil {
			folderName = m.folderInbox.GetCurrentFolder()
		}
		if m.plugins != nil {
			t := m.plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, folderName)
			m.plugins.CallHook(plugin.HookEmailViewed, t)
		}
		m.current = tui.NewStatus("Fetching email content...")
		return m, tea.Batch(m.current.Init(), fetchFolderEmailBodyCmd(m.config, msg.UID, msg.AccountID, folderName, msg.Mailbox), m.pluginNotifyCmd())

	case tui.EmailBodyFetchedMsg:
		if msg.Err != nil {
			log.Printf("could not fetch email body: %v", msg.Err)
			if m.folderInbox != nil {
				m.current = m.folderInbox
			}
			return m, nil
		}

		// Update the email in our stores
		m.updateEmailBodyByUID(msg.UID, msg.AccountID, msg.Mailbox, msg.Body, msg.Attachments)

		email := m.getEmailByUIDAndAccount(msg.UID, msg.AccountID, msg.Mailbox)
		if email == nil {
			if m.folderInbox != nil {
				m.current = m.folderInbox
			}
			return m, nil
		}

		// Mark as read in UI immediately and on the server
		var markReadCmd tea.Cmd
		if !email.IsRead {
			m.markEmailAsReadInStores(msg.UID, msg.AccountID)

			folderName := "INBOX"
			if m.folderInbox != nil {
				folderName = m.folderInbox.GetCurrentFolder()
			}
			account := m.config.GetAccountByID(msg.AccountID)
			if account != nil {
				markReadCmd = markEmailAsReadCmd(account, msg.UID, msg.AccountID, folderName)
			}
		}

		// Find the index for the email view (used for display purposes)
		emailIndex := m.getEmailIndex(msg.UID, msg.AccountID, msg.Mailbox)
		emailView := tui.NewEmailView(*email, emailIndex, m.width, m.height, msg.Mailbox, m.config.DisableImages)
		m.current = emailView
		m.syncPluginStatus()
		m.syncPluginKeyBindings()
		cmds := []tea.Cmd{m.current.Init()}
		if markReadCmd != nil {
			cmds = append(cmds, markReadCmd)
		}
		return m, tea.Batch(cmds...)

	case tui.ReplyToEmailMsg:
		to := msg.Email.From
		subject := msg.Email.Subject
		normalizedSubject := strings.ToLower(strings.TrimSpace(subject))
		if !strings.HasPrefix(normalizedSubject, "re:") {
			subject = "Re: " + subject
		}
		quotedText := fmt.Sprintf("\n\nOn %s, %s wrote:\n> %s", msg.Email.Date.Format("Jan 2, 2006 at 3:04 PM"), msg.Email.From, strings.ReplaceAll(msg.Email.Body, "\n", "\n> "))

		var composer *tui.Composer
		hideTips := false
		if m.config != nil {
			hideTips = m.config.HideTips
		}
		if m.config != nil && len(m.config.Accounts) > 0 {
			// Use the account that received the email
			accountID := msg.Email.AccountID
			if accountID == "" {
				accountID = m.config.GetFirstAccount().ID
			}
			composer = tui.NewComposerWithAccounts(m.config.Accounts, accountID, to, subject, "", hideTips)
		} else {
			composer = tui.NewComposer("", to, subject, "", hideTips)
		}
		composer.SetQuotedText(quotedText)

		// Set reply headers
		inReplyTo := msg.Email.MessageID
		references := append(msg.Email.References, msg.Email.MessageID)
		composer.SetReplyContext(inReplyTo, references)

		m.current = composer
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.syncPluginKeyBindings()
		return m, m.current.Init()

	case tui.ForwardEmailMsg:
		subject := msg.Email.Subject
		if !strings.HasPrefix(strings.ToLower(subject), "fwd:") {
			subject = "Fwd: " + subject
		}

		forwardHeader := fmt.Sprintf("\n\n---------- Forwarded message ----------\nFrom: %s\nDate: %s\nSubject: %s\nTo: %s\n\n",
			msg.Email.From,
			msg.Email.Date.Format("Mon, Jan 2, 2006 at 3:04 PM"),
			msg.Email.Subject,
			msg.Email.To,
		)

		body := forwardHeader + msg.Email.Body

		var composer *tui.Composer
		hideTips := false
		if m.config != nil {
			hideTips = m.config.HideTips
		}
		if m.config != nil && len(m.config.Accounts) > 0 {
			// Use the account that received the email
			accountID := msg.Email.AccountID
			if accountID == "" {
				accountID = m.config.GetFirstAccount().ID
			}
			composer = tui.NewComposerWithAccounts(m.config.Accounts, accountID, "", subject, body, hideTips)
		} else {
			composer = tui.NewComposer("", "", subject, body, hideTips)
		}

		m.current = composer
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		m.syncPluginKeyBindings()
		return m, m.current.Init()

	case tui.OpenEditorMsg:
		composer, ok := m.current.(*tui.Composer)
		if !ok {
			return m, nil
		}
		return m, openExternalEditor(composer.GetBody())

	case tui.EditorFinishedMsg:
		if msg.Err != nil {
			log.Printf("Editor error: %v", msg.Err)
			return m, nil
		}
		if composer, ok := m.current.(*tui.Composer); ok {
			composer.SetBody(msg.Body)
		}
		return m, nil

	case tui.GoToFilePickerMsg:
		m.previousModel = m.current
		wd, _ := os.Getwd()
		m.current = tui.NewFilePicker(wd)
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.FileSelectedMsg, tui.CancelFilePickerMsg:
		if m.previousModel != nil {
			m.current = m.previousModel
			m.previousModel = nil
		}
		m.current, cmd = m.current.Update(msg)
		cmds = append(cmds, cmd)

	case tui.SendEmailMsg:
		if m.plugins != nil {
			m.plugins.CallSendHook(plugin.HookEmailSendBefore, msg.To, msg.Cc, msg.Subject, msg.AccountID)
		}
		// Get draft ID before clearing composer (if it's a composer)
		var draftID string
		if composer, ok := m.current.(*tui.Composer); ok {
			draftID = composer.GetDraftID()
		}
		// Get the account to send from
		var account *config.Account
		if msg.AccountID != "" && m.config != nil {
			account = m.config.GetAccountByID(msg.AccountID)
		}
		if account == nil && m.config != nil {
			account = m.config.GetFirstAccount()
		}

		statusText := "Sending email..."
		if msg.SignPGP && account != nil && account.PGPKeySource == "yubikey" {
			statusText = "Touch your YubiKey to sign..."
		}
		m.current = tui.NewStatus(statusText)

		// Save contact and delete draft in background
		go func() {
			// Save the recipient as a contact
			if msg.To != "" {
				recipients := strings.Split(msg.To, ",")
				for _, r := range recipients {
					r = strings.TrimSpace(r)
					if r == "" {
						continue
					}
					name, email := parseEmailAddress(r)
					if err := config.AddContact(name, email); err != nil {
						log.Printf("Error saving contact: %v", err)
					}
				}
			}
			// Delete the draft since email is being sent
			if draftID != "" {
				if err := config.DeleteDraft(draftID); err != nil {
					log.Printf("Error deleting draft after send: %v", err)
				}
			}
		}()

		return m, tea.Batch(m.current.Init(), sendEmail(account, msg))

	case tui.EmailResultMsg:
		if msg.Err != nil {
			log.Printf("Failed to send email: %v", msg.Err)
			m.previousModel = tui.NewChoice()
			m.previousModel, _ = m.previousModel.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			m.current = tui.NewStatus(fmt.Sprintf("Error: %v", msg.Err))
			return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
				return tui.RestoreViewMsg{}
			})
		}
		if m.plugins != nil {
			m.plugins.CallHook(plugin.HookEmailSendAfter)
		}
		m.current = tui.NewChoice()
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.DeleteEmailMsg:
		tui.ClearKittyGraphics()
		m.previousModel = m.current
		m.current = tui.NewStatus("Deleting email...")

		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			if m.folderInbox != nil {
				m.current = m.folderInbox
			}
			return m, nil
		}

		folderName := "INBOX"
		if m.folderInbox != nil {
			folderName = m.folderInbox.GetCurrentFolder()
		}
		return m, tea.Batch(m.current.Init(), deleteFolderEmailCmd(account, msg.UID, msg.AccountID, folderName, msg.Mailbox))

	case tui.ArchiveEmailMsg:
		tui.ClearKittyGraphics()
		m.previousModel = m.current
		m.current = tui.NewStatus("Archiving email...")

		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			if m.folderInbox != nil {
				m.current = m.folderInbox
			}
			return m, nil
		}

		folderName := "INBOX"
		if m.folderInbox != nil {
			folderName = m.folderInbox.GetCurrentFolder()
		}
		return m, tea.Batch(m.current.Init(), archiveFolderEmailCmd(account, msg.UID, msg.AccountID, folderName, msg.Mailbox))

	case tui.EmailMarkedReadMsg:
		if msg.Err != nil {
			log.Printf("Error marking email as read: %v", msg.Err)
		}
		return m, nil

	case tui.EmailActionDoneMsg:
		if msg.Err != nil {
			log.Printf("Action failed: %v", msg.Err)
			if m.folderInbox != nil {
				m.previousModel = m.folderInbox
			}
			m.current = tui.NewStatus(fmt.Sprintf("Error: %v", msg.Err))
			return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
				return tui.RestoreViewMsg{}
			})
		}

		// Remove email from stores
		m.removeEmailFromStores(msg.UID, msg.AccountID)

		if m.folderInbox != nil {
			m.folderInbox.RemoveEmail(msg.UID, msg.AccountID)
			m.current = m.folderInbox
			m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, m.current.Init()
		}
		m.current = tui.NewChoice()
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.DownloadAttachmentMsg:
		m.previousModel = m.current
		m.current = tui.NewStatus(fmt.Sprintf("Downloading %s...", msg.Filename))

		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			m.current = m.previousModel
			return m, nil
		}

		email := m.getEmailByIndex(msg.Index, msg.Mailbox)
		if email == nil {
			m.current = m.previousModel
			return m, nil
		}

		// Find the correct attachment to get encoding
		var encoding string
		for _, att := range email.Attachments {
			if att.PartID == msg.PartID {
				encoding = att.Encoding
				break
			}
		}
		newMsg := tui.DownloadAttachmentMsg{
			Index:     msg.Index,
			Filename:  msg.Filename,
			PartID:    msg.PartID,
			Data:      msg.Data,
			AccountID: msg.AccountID,
			Encoding:  encoding,
			Mailbox:   msg.Mailbox,
		}
		return m, tea.Batch(m.current.Init(), downloadAttachmentCmd(account, email.UID, newMsg))

	case tui.AttachmentDownloadedMsg:
		var statusMsg string
		if msg.Err != nil {
			statusMsg = fmt.Sprintf("Error downloading: %v", msg.Err)
		} else {
			statusMsg = fmt.Sprintf("Saved to %s", msg.Path)
		}
		m.current = tui.NewStatus(statusMsg)
		return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return tui.RestoreViewMsg{}
		})

	case tui.RestoreViewMsg:
		if m.previousModel != nil {
			m.current = m.previousModel
			m.previousModel = nil
		}
		return m, nil
	}

	if cmd := m.pluginNotifyCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *mainModel) View() tea.View {
	v := m.current.View()
	v.AltScreen = true
	return v
}

func (m *mainModel) getEmailByIndex(index int, mailbox tui.MailboxKind) *fetcher.Email {
	if index >= 0 && index < len(m.emails) {
		return &m.emails[index]
	}
	return nil
}

func (m *mainModel) getEmailByUIDAndAccount(uid uint32, accountID string, mailbox tui.MailboxKind) *fetcher.Email {
	for i := range m.emails {
		if m.emails[i].UID == uid && m.emails[i].AccountID == accountID {
			return &m.emails[i]
		}
	}
	return nil
}

func (m *mainModel) getEmailIndex(uid uint32, accountID string, mailbox tui.MailboxKind) int {
	for i := range m.emails {
		if m.emails[i].UID == uid && m.emails[i].AccountID == accountID {
			return i
		}
	}
	return -1
}

func (m *mainModel) updateEmailBodyByUID(uid uint32, accountID string, mailbox tui.MailboxKind, body string, attachments []fetcher.Attachment) {
	for i := range m.emails {
		if m.emails[i].UID == uid && m.emails[i].AccountID == accountID {
			m.emails[i].Body = body
			m.emails[i].Attachments = attachments
			break
		}
	}
	if emails, ok := m.emailsByAcct[accountID]; ok {
		for i := range emails {
			if emails[i].UID == uid {
				emails[i].Body = body
				emails[i].Attachments = attachments
				break
			}
		}
	}
}

func (m *mainModel) markEmailAsReadInStores(uid uint32, accountID string) {
	for i := range m.emails {
		if m.emails[i].UID == uid && m.emails[i].AccountID == accountID {
			m.emails[i].IsRead = true
			break
		}
	}
	if emails, ok := m.emailsByAcct[accountID]; ok {
		for i := range emails {
			if emails[i].UID == uid {
				emails[i].IsRead = true
				break
			}
		}
	}
	// Update folder email cache
	for folderName, folderEmails := range m.folderEmails {
		for i := range folderEmails {
			if folderEmails[i].UID == uid && folderEmails[i].AccountID == accountID {
				folderEmails[i].IsRead = true
				m.folderEmails[folderName] = folderEmails
				go saveFolderEmailsToCache(folderName, folderEmails)
				break
			}
		}
	}
	// Update the inbox UI
	if m.folderInbox != nil {
		m.folderInbox.GetInbox().MarkEmailAsRead(uid, accountID)
	}
}

func (m *mainModel) removeEmailFromStores(uid uint32, accountID string) {
	var filtered []fetcher.Email
	for _, e := range m.emails {
		if !(e.UID == uid && e.AccountID == accountID) {
			filtered = append(filtered, e)
		}
	}
	m.emails = filtered
	if emails, ok := m.emailsByAcct[accountID]; ok {
		var filteredAcct []fetcher.Email
		for _, e := range emails {
			if e.UID != uid {
				filteredAcct = append(filteredAcct, e)
			}
		}
		m.emailsByAcct[accountID] = filteredAcct
	}
}

// pluginNotifyCmd checks for a pending plugin notification and returns a command if one exists.
func (m *mainModel) pluginNotifyCmd() tea.Cmd {
	if m.plugins == nil {
		return nil
	}
	if n, ok := m.plugins.TakePendingNotification(); ok {
		return func() tea.Msg {
			return tui.PluginNotifyMsg{Message: n.Message, Duration: n.Duration}
		}
	}
	return nil
}

func (m *mainModel) syncPluginStatus() {
	if m.plugins == nil {
		return
	}
	if m.folderInbox != nil {
		m.folderInbox.GetInbox().SetPluginStatus(m.plugins.StatusText(plugin.StatusInbox))
	}
	switch v := m.current.(type) {
	case *tui.Composer:
		v.SetPluginStatus(m.plugins.StatusText(plugin.StatusComposer))
	case *tui.EmailView:
		v.SetPluginStatus(m.plugins.StatusText(plugin.StatusEmailView))
	}
}

func (m *mainModel) handlePluginKeyBinding(msg tea.KeyPressMsg) {
	keyStr := msg.String()

	var area string
	switch m.current.(type) {
	case *tui.Inbox:
		area = plugin.StatusInbox
	case *tui.FolderInbox:
		area = plugin.StatusInbox
	case *tui.EmailView:
		area = plugin.StatusEmailView
	case *tui.Composer:
		area = plugin.StatusComposer
	default:
		return
	}

	bindings := m.plugins.Bindings(area)
	for _, binding := range bindings {
		if binding.Key != keyStr {
			continue
		}

		// Build context table based on the current view
		switch v := m.current.(type) {
		case *tui.Inbox:
			if email := v.GetSelectedEmail(); email != nil {
				t := m.plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, "")
				m.plugins.CallKeyBinding(binding, t)
			} else {
				m.plugins.CallKeyBinding(binding)
			}
		case *tui.FolderInbox:
			if email := v.GetInbox().GetSelectedEmail(); email != nil {
				t := m.plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, v.GetCurrentFolder())
				m.plugins.CallKeyBinding(binding, t)
			} else {
				m.plugins.CallKeyBinding(binding)
			}
		case *tui.EmailView:
			email := v.GetEmail()
			t := m.plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, "")
			m.plugins.CallKeyBinding(binding, t)
		case *tui.Composer:
			L := m.plugins.LuaState()
			t := L.NewTable()
			t.RawSetString("body", lua.LString(v.GetBody()))
			t.RawSetString("body_len", lua.LNumber(len(v.GetBody())))
			t.RawSetString("subject", lua.LString(v.GetSubject()))
			t.RawSetString("to", lua.LString(v.GetTo()))
			t.RawSetString("cc", lua.LString(v.GetCc()))
			t.RawSetString("bcc", lua.LString(v.GetBcc()))
			m.plugins.CallKeyBinding(binding, t)
			m.applyPluginFields(v)

			// Check if the plugin requested a prompt overlay
			if p, ok := m.plugins.TakePendingPrompt(); ok {
				m.pendingPrompt = p
				v.ShowPluginPrompt(p.Placeholder)
			}
		}

		m.syncPluginStatus()
		return
	}
}

func (m *mainModel) syncPluginKeyBindings() {
	if m.plugins == nil {
		return
	}

	toPluginKeyBindings := func(bindings []plugin.KeyBinding) []tui.PluginKeyBinding {
		result := make([]tui.PluginKeyBinding, len(bindings))
		for i, b := range bindings {
			result[i] = tui.PluginKeyBinding{Key: b.Key, Description: b.Description}
		}
		return result
	}

	if m.folderInbox != nil {
		m.folderInbox.GetInbox().SetPluginKeyBindings(toPluginKeyBindings(m.plugins.Bindings(plugin.StatusInbox)))
	}
	switch v := m.current.(type) {
	case *tui.Composer:
		v.SetPluginKeyBindings(toPluginKeyBindings(m.plugins.Bindings(plugin.StatusComposer)))
	case *tui.EmailView:
		v.SetPluginKeyBindings(toPluginKeyBindings(m.plugins.Bindings(plugin.StatusEmailView)))
	}
}

func (m *mainModel) applyPluginFields(composer *tui.Composer) {
	fields := m.plugins.TakePendingFields()
	if fields == nil {
		return
	}
	for field, value := range fields {
		switch field {
		case "to":
			composer.SetTo(value)
		case "cc":
			composer.SetCc(value)
		case "bcc":
			composer.SetBcc(value)
		case "subject":
			composer.SetSubject(value)
		case "body":
			composer.SetBody(value)
		}
	}
}

func flattenAndSort(emailsByAccount map[string][]fetcher.Email) []fetcher.Email {
	var allEmails []fetcher.Email
	for _, emails := range emailsByAccount {
		allEmails = append(allEmails, emails...)
	}
	for i := 0; i < len(allEmails); i++ {
		for j := i + 1; j < len(allEmails); j++ {
			if allEmails[j].Date.After(allEmails[i].Date) {
				allEmails[i], allEmails[j] = allEmails[j], allEmails[i]
			}
		}
	}
	return allEmails
}

func fetchAllAccountsEmails(cfg *config.Config, mailbox tui.MailboxKind) tea.Cmd {
	return func() tea.Msg {
		emailsByAccount := make(map[string][]fetcher.Email)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, account := range cfg.Accounts {
			wg.Add(1)
			go func(acc config.Account) {
				defer wg.Done()
				var emails []fetcher.Email
				var err error
				switch mailbox {
				case tui.MailboxSent:
					emails, err = fetcher.FetchSentEmails(&acc, initialEmailLimit, 0)
				case tui.MailboxTrash:
					emails, err = fetcher.FetchTrashEmails(&acc, initialEmailLimit, 0)
				case tui.MailboxArchive:
					emails, err = fetcher.FetchArchiveEmails(&acc, initialEmailLimit, 0)
				default:
					emails, err = fetcher.FetchEmails(&acc, initialEmailLimit, 0)
				}
				if err != nil {
					log.Printf("Error fetching from %s: %v", acc.Email, err)
					return
				}
				mu.Lock()
				emailsByAccount[acc.ID] = emails
				mu.Unlock()
			}(account)
		}

		wg.Wait()
		return tui.AllEmailsFetchedMsg{EmailsByAccount: emailsByAccount, Mailbox: mailbox}
	}
}

func fetchEmails(account *config.Account, limit, offset uint32, mailbox tui.MailboxKind) tea.Cmd {
	return func() tea.Msg {
		var emails []fetcher.Email
		var err error
		if mailbox == tui.MailboxSent {
			emails, err = fetcher.FetchSentEmails(account, limit, offset)
		} else {
			emails, err = fetcher.FetchEmails(account, limit, offset)
		}
		if err != nil {
			return tui.FetchErr(err)
		}
		if offset == 0 {
			return tui.EmailsFetchedMsg{Emails: emails, AccountID: account.ID, Mailbox: mailbox}
		}
		return tui.EmailsAppendedMsg{Emails: emails, AccountID: account.ID, Mailbox: mailbox}
	}
}

func fetchEmailsForMailbox(account *config.Account, limit, offset uint32, mailbox tui.MailboxKind) tea.Cmd {
	return func() tea.Msg {
		var emails []fetcher.Email
		var err error
		switch mailbox {
		case tui.MailboxSent:
			emails, err = fetcher.FetchSentEmails(account, limit, offset)
		case tui.MailboxTrash:
			emails, err = fetcher.FetchTrashEmails(account, limit, offset)
		case tui.MailboxArchive:
			emails, err = fetcher.FetchArchiveEmails(account, limit, offset)
		default:
			emails, err = fetcher.FetchEmails(account, limit, offset)
		}
		if err != nil {
			return tui.FetchErr(err)
		}
		if offset == 0 {
			return tui.EmailsFetchedMsg{Emails: emails, AccountID: account.ID, Mailbox: mailbox}
		}
		return tui.EmailsAppendedMsg{Emails: emails, AccountID: account.ID, Mailbox: mailbox}
	}
}

func loadCachedEmails() tea.Cmd {
	return func() tea.Msg {
		cache, err := config.LoadEmailCache()
		if err != nil {
			return tui.CachedEmailsLoadedMsg{Cache: nil}
		}
		return tui.CachedEmailsLoadedMsg{Cache: cache}
	}
}

func refreshEmails(cfg *config.Config, mailbox tui.MailboxKind, counts map[string]int) tea.Cmd {
	return func() tea.Msg {
		emailsByAccount := make(map[string][]fetcher.Email)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, account := range cfg.Accounts {
			wg.Add(1)
			go func(acc config.Account) {
				defer wg.Done()
				var emails []fetcher.Email
				var err error

				limit := uint32(initialEmailLimit)
				if counts != nil {
					if c, ok := counts[acc.ID]; ok && c > 0 {
						limit = uint32(c)
					}
				}

				if mailbox == tui.MailboxSent {
					emails, err = fetcher.FetchSentEmails(&acc, limit, 0)
				} else {
					emails, err = fetcher.FetchEmails(&acc, limit, 0)
				}
				if err != nil {
					log.Printf("Error fetching from %s: %v", acc.Email, err)
					return
				}
				mu.Lock()
				emailsByAccount[acc.ID] = emails
				mu.Unlock()
			}(account)
		}

		wg.Wait()
		return tui.EmailsRefreshedMsg{EmailsByAccount: emailsByAccount, Mailbox: mailbox}
	}
}

func emailsToCache(emails []fetcher.Email) []config.CachedEmail {
	var cached []config.CachedEmail
	for _, email := range emails {
		cached = append(cached, config.CachedEmail{
			UID:       email.UID,
			From:      email.From,
			To:        email.To,
			Subject:   email.Subject,
			Date:      email.Date,
			MessageID: email.MessageID,
			AccountID: email.AccountID,
			IsRead:    email.IsRead,
		})
	}
	return cached
}

func cacheToEmails(cached []config.CachedEmail) []fetcher.Email {
	var emails []fetcher.Email
	for _, c := range cached {
		emails = append(emails, fetcher.Email{
			UID:       c.UID,
			From:      c.From,
			To:        c.To,
			Subject:   c.Subject,
			Date:      c.Date,
			MessageID: c.MessageID,
			AccountID: c.AccountID,
			IsRead:    c.IsRead,
		})
	}
	return emails
}

func saveFolderEmailsToCache(folderName string, emails []fetcher.Email) {
	cached := emailsToCache(emails)
	if err := config.SaveFolderEmailCache(folderName, cached); err != nil {
		log.Printf("Error saving folder email cache for %s: %v", folderName, err)
	}
}

func loadFolderEmailsFromCache(folderName string) []fetcher.Email {
	cached, err := config.LoadFolderEmailCache(folderName)
	if err != nil {
		return nil
	}
	return cacheToEmails(cached)
}

func saveEmailsToCache(emails []fetcher.Email) {
	if len(emails) > maxCacheEmails {
		emails = emails[:maxCacheEmails]
	}
	var cachedEmails []config.CachedEmail
	for _, email := range emails {
		cachedEmails = append(cachedEmails, config.CachedEmail{
			UID:       email.UID,
			From:      email.From,
			To:        email.To,
			Subject:   email.Subject,
			Date:      email.Date,
			MessageID: email.MessageID,
			AccountID: email.AccountID,
			IsRead:    email.IsRead,
		})

		// Save sender as a contact
		if email.From != "" {
			name, emailAddr := parseEmailAddress(email.From)
			if err := config.AddContact(name, emailAddr); err != nil {
				log.Printf("Error saving contact from email: %v", err)
			}
		}
	}
	cache := &config.EmailCache{Emails: cachedEmails}
	if err := config.SaveEmailCache(cache); err != nil {
		log.Printf("Error saving email cache: %v", err)
	}
}

// parseEmailAddress parses "Name <email>" or just "email" format
func parseEmailAddress(addr string) (name, email string) {
	addr = strings.TrimSpace(addr)
	if idx := strings.Index(addr, "<"); idx != -1 {
		name = strings.TrimSpace(addr[:idx])
		endIdx := strings.Index(addr, ">")
		if endIdx > idx {
			email = strings.TrimSpace(addr[idx+1 : endIdx])
		} else {
			email = strings.TrimSpace(addr[idx+1:])
		}
	} else {
		email = addr
	}
	return name, email
}

func fetchEmailBodyCmd(cfg *config.Config, uid uint32, accountID string, mailbox tui.MailboxKind) tea.Cmd {
	return func() tea.Msg {
		account := cfg.GetAccountByID(accountID)
		if account == nil {
			return tui.EmailBodyFetchedMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Err: fmt.Errorf("account not found")}
		}

		var (
			body        string
			attachments []fetcher.Attachment
			err         error
		)
		switch mailbox {
		case tui.MailboxSent:
			body, attachments, err = fetcher.FetchSentEmailBody(account, uid)
		case tui.MailboxTrash:
			body, attachments, err = fetcher.FetchTrashEmailBody(account, uid)
		case tui.MailboxArchive:
			body, attachments, err = fetcher.FetchArchiveEmailBody(account, uid)
		default:
			body, attachments, err = fetcher.FetchEmailBody(account, uid)
		}
		if err != nil {
			return tui.EmailBodyFetchedMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Err: err}
		}

		return tui.EmailBodyFetchedMsg{
			UID:         uid,
			Body:        body,
			Attachments: attachments,
			AccountID:   accountID,
			Mailbox:     mailbox,
		}
	}
}

func markdownToHTML(md []byte) []byte {
	return clib.MarkdownToHTML(md)
}

func splitEmails(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var res []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

func sendEmail(account *config.Account, msg tui.SendEmailMsg) tea.Cmd {
	return func() tea.Msg {
		if account == nil {
			return tui.EmailResultMsg{Err: fmt.Errorf("no account configured")}
		}

		recipients := splitEmails(msg.To)
		cc := splitEmails(msg.Cc)
		bcc := splitEmails(msg.Bcc)
		body := msg.Body
		// Append signature if present
		if msg.Signature != "" {
			body = body + "\n\n" + msg.Signature
		}
		// Append quoted text if present (for replies)
		if msg.QuotedText != "" {
			body = body + msg.QuotedText
		}
		images := make(map[string][]byte)
		attachments := make(map[string][]byte)

		re := regexp.MustCompile(`!\[.*?\]\((.*?)\)`)
		matches := re.FindAllStringSubmatch(body, -1)

		for _, match := range matches {
			imgPath := match[1]
			imgData, err := os.ReadFile(imgPath)
			if err != nil {
				log.Printf("Could not read image file %s: %v", imgPath, err)
				continue
			}
			cid := fmt.Sprintf("%s%s@%s", uuid.NewString(), filepath.Ext(imgPath), "matcha")
			images[cid] = []byte(base64.StdEncoding.EncodeToString(imgData))
			body = strings.Replace(body, imgPath, "cid:"+cid, 1)
		}

		htmlBody := markdownToHTML([]byte(body))

		for _, attachPath := range msg.AttachmentPaths {
			fileData, err := os.ReadFile(attachPath)
			if err != nil {
				log.Printf("Could not read attachment file %s: %v", attachPath, err)
				continue
			}
			_, filename := filepath.Split(attachPath)
			attachments[filename] = fileData
		}

		err := sender.SendEmail(account, recipients, cc, bcc, msg.Subject, body, string(htmlBody), images, attachments, msg.InReplyTo, msg.References, msg.SignSMIME, msg.EncryptSMIME, msg.SignPGP, false)
		if err != nil {
			log.Printf("Failed to send email: %v", err)
			return tui.EmailResultMsg{Err: err}
		}
		return tui.EmailResultMsg{}
	}
}

func deleteEmailCmd(account *config.Account, uid uint32, accountID string, mailbox tui.MailboxKind) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch mailbox {
		case tui.MailboxSent:
			err = fetcher.DeleteSentEmail(account, uid)
		case tui.MailboxTrash:
			err = fetcher.DeleteTrashEmail(account, uid)
		case tui.MailboxArchive:
			err = fetcher.DeleteArchiveEmail(account, uid)
		default:
			err = fetcher.DeleteEmail(account, uid)
		}
		return tui.EmailActionDoneMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Err: err}
	}
}

func archiveEmailCmd(account *config.Account, uid uint32, accountID string, mailbox tui.MailboxKind) tea.Cmd {
	return func() tea.Msg {
		var err error
		if mailbox == tui.MailboxSent {
			err = fetcher.ArchiveSentEmail(account, uid)
		} else {
			err = fetcher.ArchiveEmail(account, uid)
		}
		return tui.EmailActionDoneMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Err: err}
	}
}

// --- External editor command ---

// openExternalEditor writes the body to a temp file, opens $EDITOR, and reads back the result.
func openExternalEditor(body string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = "vi"
	}

	tmpFile, err := os.CreateTemp("", "matcha-*.md")
	if err != nil {
		return func() tea.Msg {
			return tui.EditorFinishedMsg{Err: fmt.Errorf("creating temp file: %w", err)}
		}
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.WriteString(body); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return func() tea.Msg {
			return tui.EditorFinishedMsg{Err: fmt.Errorf("writing temp file: %w", err)}
		}
	}
	tmpFile.Close()

	parts := strings.Fields(editor)
	args := append(parts[1:], tmpPath)
	c := exec.Command(parts[0], args...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer os.Remove(tmpPath)
		if err != nil {
			return tui.EditorFinishedMsg{Err: err}
		}
		content, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return tui.EditorFinishedMsg{Err: readErr}
		}
		return tui.EditorFinishedMsg{Body: string(content)}
	})
}

// --- IDLE command ---

// listenForIdleUpdates blocks until an IDLE update arrives, then returns it as a tea.Msg.
func listenForIdleUpdates(ch <-chan fetcher.IdleUpdate) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return nil
		}
		return tui.IdleNewMailMsg{
			AccountID:  update.AccountID,
			FolderName: update.FolderName,
		}
	}
}

// --- Folder-based command functions ---

func fetchFoldersCmd(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		if !cfg.HasAccounts() {
			return nil
		}
		foldersByAccount := make(map[string][]fetcher.Folder)
		seen := make(map[string]fetcher.Folder)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, account := range cfg.Accounts {
			wg.Add(1)
			go func(acc config.Account) {
				defer wg.Done()
				folders, err := fetcher.FetchFolders(&acc)
				if err != nil {
					return
				}
				mu.Lock()
				foldersByAccount[acc.ID] = folders
				for _, f := range folders {
					if _, ok := seen[f.Name]; !ok {
						seen[f.Name] = f
					}
				}
				mu.Unlock()
			}(account)
		}
		wg.Wait()

		var merged []fetcher.Folder
		for _, f := range seen {
			merged = append(merged, f)
		}

		return tui.FoldersFetchedMsg{
			FoldersByAccount: foldersByAccount,
			MergedFolders:    merged,
		}
	}
}

func fetchFolderEmailsCmd(cfg *config.Config, folderName string) tea.Cmd {
	return func() tea.Msg {
		emailsByAccount := make(map[string][]fetcher.Email)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, account := range cfg.Accounts {
			wg.Add(1)
			go func(acc config.Account) {
				defer wg.Done()
				emails, err := fetcher.FetchFolderEmails(&acc, folderName, initialEmailLimit, 0)
				if err != nil {
					// Folder may not exist for this account — silently skip
					return
				}
				mu.Lock()
				emailsByAccount[acc.ID] = emails
				mu.Unlock()
			}(account)
		}

		wg.Wait()

		// Flatten all account emails
		var allEmails []fetcher.Email
		for _, emails := range emailsByAccount {
			allEmails = append(allEmails, emails...)
		}
		// Sort newest first
		for i := 0; i < len(allEmails); i++ {
			for j := i + 1; j < len(allEmails); j++ {
				if allEmails[j].Date.After(allEmails[i].Date) {
					allEmails[i], allEmails[j] = allEmails[j], allEmails[i]
				}
			}
		}

		return tui.FolderEmailsFetchedMsg{
			Emails:     allEmails,
			FolderName: folderName,
		}
	}
}

func fetchFolderEmailsPaginatedCmd(account *config.Account, folderName string, limit, offset uint32) tea.Cmd {
	return func() tea.Msg {
		emails, err := fetcher.FetchFolderEmails(account, folderName, limit, offset)
		if err != nil {
			return tui.FetchErr(err)
		}
		return tui.FolderEmailsAppendedMsg{
			Emails:     emails,
			AccountID:  account.ID,
			FolderName: folderName,
		}
	}
}

func fetchFolderEmailBodyCmd(cfg *config.Config, uid uint32, accountID string, folderName string, mailbox tui.MailboxKind) tea.Cmd {
	return func() tea.Msg {
		account := cfg.GetAccountByID(accountID)
		if account == nil {
			return tui.EmailBodyFetchedMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Err: fmt.Errorf("account not found")}
		}

		body, attachments, err := fetcher.FetchFolderEmailBody(account, folderName, uid)
		if err != nil {
			return tui.EmailBodyFetchedMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Err: err}
		}

		return tui.EmailBodyFetchedMsg{
			UID:         uid,
			Body:        body,
			Attachments: attachments,
			AccountID:   accountID,
			Mailbox:     mailbox,
		}
	}
}

func markEmailAsReadCmd(account *config.Account, uid uint32, accountID string, folderName string) tea.Cmd {
	return func() tea.Msg {
		err := fetcher.MarkEmailAsReadInMailbox(account, folderName, uid)
		return tui.EmailMarkedReadMsg{UID: uid, AccountID: accountID, Err: err}
	}
}

func deleteFolderEmailCmd(account *config.Account, uid uint32, accountID string, folderName string, mailbox tui.MailboxKind) tea.Cmd {
	return func() tea.Msg {
		err := fetcher.DeleteFolderEmail(account, folderName, uid)
		return tui.EmailActionDoneMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Err: err}
	}
}

func archiveFolderEmailCmd(account *config.Account, uid uint32, accountID string, folderName string, mailbox tui.MailboxKind) tea.Cmd {
	return func() tea.Msg {
		err := fetcher.ArchiveFolderEmail(account, folderName, uid)
		return tui.EmailActionDoneMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Err: err}
	}
}

func moveEmailToFolderCmd(account *config.Account, uid uint32, accountID string, sourceFolder, destFolder string) tea.Cmd {
	return func() tea.Msg {
		err := fetcher.MoveEmailToFolder(account, uid, sourceFolder, destFolder)
		return tui.EmailMovedMsg{
			UID:          uid,
			AccountID:    accountID,
			SourceFolder: sourceFolder,
			DestFolder:   destFolder,
			Err:          err,
		}
	}
}

func downloadAttachmentCmd(account *config.Account, uid uint32, msg tui.DownloadAttachmentMsg) tea.Cmd {
	return func() tea.Msg {
		// Download and decode the attachment using encoding provided in msg.Encoding.
		var data []byte
		var err error
		switch msg.Mailbox {
		case tui.MailboxSent:
			data, err = fetcher.FetchSentAttachment(account, uid, msg.PartID, msg.Encoding)
		case tui.MailboxTrash:
			data, err = fetcher.FetchTrashAttachment(account, uid, msg.PartID, msg.Encoding)
		case tui.MailboxArchive:
			data, err = fetcher.FetchArchiveAttachment(account, uid, msg.PartID, msg.Encoding)
		default:
			data, err = fetcher.FetchAttachment(account, uid, msg.PartID, msg.Encoding)
		}
		if err != nil {
			return tui.AttachmentDownloadedMsg{Err: err}
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return tui.AttachmentDownloadedMsg{Err: err}
		}
		downloadsPath := filepath.Join(homeDir, "Downloads")
		if _, err := os.Stat(downloadsPath); os.IsNotExist(err) {
			if mkErr := os.MkdirAll(downloadsPath, 0755); mkErr != nil {
				return tui.AttachmentDownloadedMsg{Err: mkErr}
			}
		}

		// Save the attachment using an exclusive create so we never overwrite an existing file.
		// If the filename already exists, append \" (n)\" before the extension.
		origName := msg.Filename
		ext := filepath.Ext(origName)
		base := strings.TrimSuffix(origName, ext)
		candidate := origName
		i := 1
		var filePath string

		for {
			filePath = filepath.Join(downloadsPath, candidate)

			// Try to create file exclusively. If it already exists, os.OpenFile will return an error
			// that satisfies os.IsExist(err), so we can increment the candidate.
			f, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
			if err != nil {
				if os.IsExist(err) {
					// file exists, try next candidate
					candidate = fmt.Sprintf("%s (%d)%s", base, i, ext)
					i++
					continue
				}
				// Some other error while attempting to create file
				log.Printf("error creating file %s: %v", filePath, err)
				return tui.AttachmentDownloadedMsg{Err: err}
			}

			// Successfully created the file descriptor; write and close.
			if _, writeErr := f.Write(data); writeErr != nil {
				_ = f.Close()
				log.Printf("error writing to file %s: %v", filePath, writeErr)
				return tui.AttachmentDownloadedMsg{Err: writeErr}
			}
			if closeErr := f.Close(); closeErr != nil {
				log.Printf("warning: error closing file %s: %v", filePath, closeErr)
			}

			// file saved successfully
			break
		}

		log.Printf("attachment saved to %s", filePath)

		// Try to open the file using a platform-specific opener asynchronously and log the outcome.
		go func(p string) {
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "darwin":
				cmd = exec.Command("open", p)
			case "linux":
				cmd = exec.Command("xdg-open", p)
			case "windows":
				// 'start' is a cmd builtin; provide an empty title argument to avoid interpreting the path as the title.
				cmd = exec.Command("cmd", "/c", "start", "", p)
			default:
				// Unsupported OS: nothing to do.
				return
			}
			if err := cmd.Start(); err != nil {
				log.Printf("failed to open file %s: %v", p, err)
			}
		}(filePath)

		return tui.AttachmentDownloadedMsg{Path: filePath, Err: nil}
	}
}

/*
detectInstalledVersion returns a best-effort installed version string.
Priority:
 1. If the build-in `version` variable is set to something other than "dev", return it.
 2. If Homebrew is present and reports a version for `matcha`, return that.
 3. If snap is present and lists `matcha`, return that.
 4. Fallback to the build `version` (likely "dev").
*/
func detectInstalledVersion() string {
	v := strings.TrimSpace(version)
	if v != "dev" && v != "" {
		return v
	}

	// Try Homebrew (macOS)
	if runtime.GOOS == "darwin" {
		if _, err := exec.LookPath("brew"); err == nil {
			// `brew list --versions matcha` prints: matcha 1.2.3
			if out, err := exec.Command("brew", "list", "--versions", "matcha").Output(); err == nil {
				parts := strings.Fields(string(out))
				if len(parts) >= 2 {
					return parts[1]
				}
			}
		}
	}

	// Try WinGet (Windows)
	if runtime.GOOS == "windows" {
		if _, err := exec.LookPath("winget"); err == nil {
			if out, err := exec.Command("winget", "list", "--id", "floatpane.matcha", "--disable-interactivity").Output(); err == nil {
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				for _, line := range lines {
					if strings.Contains(strings.ToLower(line), "floatpane.matcha") {
						fields := strings.Fields(line)
						for _, f := range fields {
							if len(f) > 0 && f[0] >= '0' && f[0] <= '9' && strings.Contains(f, ".") {
								return f
							}
						}
					}
				}
			}
		}
	}

	// Try snap (Linux)
	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("snap"); err == nil {
			if out, err := exec.Command("snap", "list", "matcha").Output(); err == nil {
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				if len(lines) >= 2 {
					fields := strings.Fields(lines[1])
					if len(fields) >= 2 {
						return fields[1]
					}
				}
			}
		}

		if _, err := exec.LookPath("flatpak"); err == nil {
			if out, err := exec.Command("flatpak", "info", "com.floatpane.matcha").Output(); err == nil {
				lines := strings.Split(strings.TrimSpace(string(out)), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if strings.HasPrefix(line, "Version:") {
						fields := strings.Fields(line)
						if len(fields) >= 2 {
							return fields[1]
						}
					}
				}
			}
		}
	}

	return v
}

/*
checkForUpdatesCmd queries GitHub for the latest release tag and returns a
tea.Msg (UpdateAvailableMsg) if the latest version differs from the current
installed version. This runs in the background when the TUI initializes.
*/
func checkForUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		// Non-fatal: if anything goes wrong we just don't show the update message.
		const api = "https://api.github.com/repos/floatpane/matcha/releases/latest"
		resp, err := http.Get(api)
		if err != nil {
			return nil
		}
		defer resp.Body.Close()

		var rel githubRelease
		if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
			return nil
		}

		latest := strings.TrimPrefix(rel.TagName, "v")
		installed := strings.TrimPrefix(detectInstalledVersion(), "v")
		if latest != "" && installed != "" && latest != installed {
			return UpdateAvailableMsg{Latest: latest, Current: installed}
		}
		return nil
	}
}

// runUpdateCLI implements the CLI entrypoint for `matcha update`.
// It detects the likely installation method and attempts the appropriate
// update path (Homebrew, Snap, or GitHub release binary extract).
// runGmailOAuthCLI handles the "matcha gmail" subcommand for OAuth2 management.
// Usage:
//
//	matcha gmail auth   <email> [--client-id ID --client-secret SECRET]
//	matcha gmail token  <email>
//	matcha gmail revoke <email>
func runGmailOAuthCLI(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: matcha gmail <auth|token|revoke> <email> [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  auth   <email>  Authorize a Gmail account via OAuth2 (opens browser)")
		fmt.Fprintln(os.Stderr, "  token  <email>  Print a fresh access token (refreshes automatically)")
		fmt.Fprintln(os.Stderr, "  revoke <email>  Revoke and delete stored OAuth2 tokens")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Before using OAuth2, create ~/.config/matcha/oauth_client.json with:")
		fmt.Fprintln(os.Stderr, `  {"client_id": "YOUR_ID", "client_secret": "YOUR_SECRET"}`)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Get credentials at: https://console.cloud.google.com/apis/credentials")
		os.Exit(1)
	}

	// Find the Python script and pass through to it
	script, err := config.OAuthScriptPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	cmdArgs := append([]string{script}, args...)
	cmd := exec.Command("python3", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// stringSliceFlag implements flag.Value to allow repeated --attach flags.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ", ") }
func (s *stringSliceFlag) Set(val string) error {
	*s = append(*s, val)
	return nil
}

// runSendCLI implements the CLI entrypoint for `matcha send`.
// It sends an email non-interactively using configured accounts.
func runSendCLI(args []string) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)

	to := fs.String("to", "", "Recipient(s), comma-separated (required)")
	cc := fs.String("cc", "", "CC recipient(s), comma-separated")
	bcc := fs.String("bcc", "", "BCC recipient(s), comma-separated")
	subject := fs.String("subject", "", "Email subject (required)")
	body := fs.String("body", "", `Email body (Markdown supported). Use "-" to read from stdin`)
	from := fs.String("from", "", "Sender account email (defaults to first configured account)")
	withSignature := fs.Bool("signature", true, "Append default signature")
	signSMIME := fs.Bool("sign-smime", false, "Sign with S/MIME")
	encryptSMIME := fs.Bool("encrypt-smime", false, "Encrypt with S/MIME")
	signPGP := fs.Bool("sign-pgp", false, "Sign with PGP")

	var attachments stringSliceFlag
	fs.Var(&attachments, "attach", "Attachment file path (can be repeated)")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: matcha send [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Send an email non-interactively using a configured account.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, `  matcha send --to user@example.com --subject "Hello" --body "Hi there"`)
		fmt.Fprintln(os.Stderr, `  echo "Body text" | matcha send --to user@example.com --subject "Hello" --body -`)
		fmt.Fprintln(os.Stderr, `  matcha send --to user@example.com --subject "Report" --body "See attached" --attach report.pdf`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *to == "" || *subject == "" {
		fmt.Fprintln(os.Stderr, "Error: --to and --subject are required")
		fs.Usage()
		os.Exit(1)
	}

	// Read body from stdin if "-"
	emailBody := *body
	if emailBody == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		emailBody = string(data)
	}

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	if !cfg.HasAccounts() {
		fmt.Fprintln(os.Stderr, "Error: no accounts configured. Run matcha to set up an account first.")
		os.Exit(1)
	}

	// Resolve account
	var account *config.Account
	if *from != "" {
		account = cfg.GetAccountByEmail(*from)
		if account == nil {
			// Also try matching against FetchEmail
			for i := range cfg.Accounts {
				if strings.EqualFold(cfg.Accounts[i].FetchEmail, *from) {
					account = &cfg.Accounts[i]
					break
				}
			}
		}
		if account == nil {
			fmt.Fprintf(os.Stderr, "Error: no account found matching %q\n", *from)
			os.Exit(1)
		}
	} else {
		account = cfg.GetFirstAccount()
	}

	// Use account S/MIME/PGP defaults unless explicitly set
	if !isFlagSet(fs, "sign-smime") {
		*signSMIME = account.SMIMESignByDefault
	}
	if !isFlagSet(fs, "sign-pgp") {
		*signPGP = account.PGPSignByDefault
	}

	// Append signature
	if *withSignature {
		if sig, err := config.LoadSignature(); err == nil && sig != "" {
			emailBody = emailBody + "\n\n" + sig
		}
	}

	// Process inline images (same logic as TUI sendEmail)
	images := make(map[string][]byte)
	re := regexp.MustCompile(`!\[.*?\]\((.*?)\)`)
	matches := re.FindAllStringSubmatch(emailBody, -1)
	for _, match := range matches {
		imgPath := match[1]
		imgData, err := os.ReadFile(imgPath)
		if err != nil {
			log.Printf("Could not read image file %s: %v", imgPath, err)
			continue
		}
		cid := fmt.Sprintf("%s%s@%s", uuid.NewString(), filepath.Ext(imgPath), "matcha")
		images[cid] = []byte(base64.StdEncoding.EncodeToString(imgData))
		emailBody = strings.Replace(emailBody, imgPath, "cid:"+cid, 1)
	}

	htmlBody := markdownToHTML([]byte(emailBody))

	// Process attachments
	attachMap := make(map[string][]byte)
	for _, attachPath := range attachments {
		fileData, err := os.ReadFile(attachPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading attachment %s: %v\n", attachPath, err)
			os.Exit(1)
		}
		attachMap[filepath.Base(attachPath)] = fileData
	}

	// Send
	recipients := splitEmails(*to)
	ccList := splitEmails(*cc)
	bccList := splitEmails(*bcc)

	err = sender.SendEmail(account, recipients, ccList, bccList, *subject, emailBody, string(htmlBody), images, attachMap, "", nil, *signSMIME, *encryptSMIME, *signPGP, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Email sent successfully.")
}

// isFlagSet returns true if the named flag was explicitly provided on the command line.
func isFlagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func runUpdateCLI() error {
	const api = "https://api.github.com/repos/floatpane/matcha/releases/latest"
	resp, err := http.Get(api)
	if err != nil {
		return fmt.Errorf("could not query releases: %w", err)
	}
	defer resp.Body.Close()

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return fmt.Errorf("could not parse release info: %w", err)
	}

	latestTag := rel.TagName
	if strings.HasPrefix(latestTag, "v") {
		latestTag = latestTag[1:]
	}

	fmt.Printf("Current version: %s\n", version)
	fmt.Printf("Latest version: %s\n", latestTag)

	// Quick check: if already up-to-date, exit
	cur := version
	if strings.HasPrefix(cur, "v") {
		cur = cur[1:]
	}
	if latestTag == "" || cur == latestTag {
		fmt.Println("Already up to date.")
		return nil
	}

	// Detect Homebrew
	if _, err := exec.LookPath("brew"); err == nil {
		fmt.Println("Detected Homebrew — updating taps and attempting to upgrade via brew.")

		updateCmd := exec.Command("brew", "update")
		updateCmd.Stdout = os.Stdout
		updateCmd.Stderr = os.Stderr
		if err := updateCmd.Run(); err != nil {
			fmt.Printf("Homebrew update failed: %v\n", err)
			// continue to attempt upgrade even if update failed
		}

		upgradeCmd := exec.Command("brew", "upgrade", "floatpane/matcha/matcha")
		upgradeCmd.Stdout = os.Stdout
		upgradeCmd.Stderr = os.Stderr
		if err := upgradeCmd.Run(); err == nil {
			fmt.Println("Successfully upgraded via Homebrew.")
			return nil
		}
		fmt.Printf("Homebrew upgrade failed: %v\n", err)
		// fallthrough to other methods
	}

	// Detect snap
	if _, err := exec.LookPath("snap"); err == nil {
		// Check if matcha is installed as a snap
		cmdCheck := exec.Command("snap", "list", "matcha")
		if err := cmdCheck.Run(); err == nil {
			fmt.Println("Detected Snap package — attempting to refresh.")
			cmd := exec.Command("snap", "refresh", "matcha")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				fmt.Println("Successfully refreshed snap.")
				return nil
			}
			fmt.Printf("Snap refresh failed: %v\n", err)
			// fallthrough
		}
	}
	// Detect flatpak
	if _, err := exec.LookPath("flatpak"); err == nil {
		// Check if matcha is installed as a flatpak
		cmdCheck := exec.Command("flatpak", "info", "com.floatpane.matcha")
		if err := cmdCheck.Run(); err == nil {
			fmt.Println("Detected Flatpak package — attempting to update.")
			cmd := exec.Command("flatpak", "update", "-y", "com.floatpane.matcha")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				fmt.Println("Successfully updated flatpak.")
				return nil
			}
			fmt.Printf("Flatpak update failed: %v\n", err)
			// fallthrough
		}
	}

	// Detect WinGet
	if _, err := exec.LookPath("winget"); err == nil {
		cmdCheck := exec.Command("winget", "list", "--id", "floatpane.matcha", "--disable-interactivity")
		if err := cmdCheck.Run(); err == nil {
			fmt.Println("Detected WinGet package — attempting to upgrade.")
			cmd := exec.Command("winget", "upgrade", "--id", "floatpane.matcha", "--disable-interactivity")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				fmt.Println("Successfully upgraded via WinGet.")
				return nil
			}
			fmt.Printf("WinGet upgrade failed: %v\n", err)
			// fallthrough
		}
	}

	// Otherwise attempt to download the proper release asset and replace the binary.
	osName := runtime.GOOS
	arch := runtime.GOARCH

	// Try to find a matching asset
	var assetURL, assetName string
	for _, a := range rel.Assets {
		n := strings.ToLower(a.Name)
		if strings.Contains(n, osName) && strings.Contains(n, arch) && (strings.HasSuffix(n, ".tar.gz") || strings.HasSuffix(n, ".tgz") || strings.HasSuffix(n, ".zip")) {
			assetURL = a.BrowserDownloadURL
			assetName = a.Name
			break
		}
	}
	if assetURL == "" {
		// Try any asset that contains 'matcha' and os/arch as a fallback
		for _, a := range rel.Assets {
			n := strings.ToLower(a.Name)
			if strings.Contains(n, "matcha") && (strings.Contains(n, osName) || strings.Contains(n, arch)) {
				assetURL = a.BrowserDownloadURL
				assetName = a.Name
				break
			}
		}
	}

	if assetURL == "" {
		return fmt.Errorf("no suitable release artifact found for %s/%s", osName, arch)
	}

	fmt.Printf("Found release asset: %s\n", assetName)
	fmt.Println("Downloading...")

	// Download asset
	respAsset, err := http.Get(assetURL)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer respAsset.Body.Close()

	// Create a temp file for the download
	tmpDir, err := os.MkdirTemp("", "matcha-update-*")
	if err != nil {
		return fmt.Errorf("could not create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	assetPath := filepath.Join(tmpDir, assetName)
	outFile, err := os.Create(assetPath)
	if err != nil {
		return fmt.Errorf("could not create temp file: %w", err)
	}
	_, err = io.Copy(outFile, respAsset.Body)
	outFile.Close()
	if err != nil {
		return fmt.Errorf("could not write asset to disk: %w", err)
	}

	// Determine the expected binary name based on the OS.
	binaryName := "matcha"
	if runtime.GOOS == "windows" {
		binaryName = "matcha.exe"
	}

	// Extract the binary from the archive.
	var binPath string
	if strings.HasSuffix(assetName, ".tar.gz") || strings.HasSuffix(assetName, ".tgz") {
		f, err := os.Open(assetPath)
		if err != nil {
			return fmt.Errorf("could not open archive: %w", err)
		}
		defer f.Close()
		gzr, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("could not create gzip reader: %w", err)
		}
		tr := tar.NewReader(gzr)
		for {
			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("error reading tar: %w", err)
			}
			name := filepath.Base(hdr.Name)
			if name == binaryName || strings.Contains(strings.ToLower(name), "matcha") && (hdr.Typeflag == tar.TypeReg) {
				binPath = filepath.Join(tmpDir, binaryName)
				out, err := os.Create(binPath)
				if err != nil {
					return fmt.Errorf("could not create binary file: %w", err)
				}
				if _, err := io.Copy(out, tr); err != nil {
					out.Close()
					return fmt.Errorf("could not extract binary: %w", err)
				}
				out.Close()
				if err := os.Chmod(binPath, 0755); err != nil {
					return fmt.Errorf("could not make binary executable: %w", err)
				}
				break
			}
		}
	} else if strings.HasSuffix(assetName, ".zip") {
		zr, err := zip.OpenReader(assetPath)
		if err != nil {
			return fmt.Errorf("could not open zip archive: %w", err)
		}
		defer zr.Close()
		for _, zf := range zr.File {
			name := filepath.Base(zf.Name)
			if name == binaryName || strings.Contains(strings.ToLower(name), "matcha") && !zf.FileInfo().IsDir() {
				rc, err := zf.Open()
				if err != nil {
					return fmt.Errorf("could not open file in zip: %w", err)
				}
				binPath = filepath.Join(tmpDir, binaryName)
				out, err := os.Create(binPath)
				if err != nil {
					rc.Close()
					return fmt.Errorf("could not create binary file: %w", err)
				}
				if _, err := io.Copy(out, rc); err != nil {
					out.Close()
					rc.Close()
					return fmt.Errorf("could not extract binary: %w", err)
				}
				out.Close()
				rc.Close()
				if err := os.Chmod(binPath, 0755); err != nil {
					return fmt.Errorf("could not make binary executable: %w", err)
				}
				break
			}
		}
	} else {
		// For non-archive assets, assume the asset is the binary itself.
		binPath = assetPath
		if err := os.Chmod(binPath, 0755); err != nil {
			// ignore chmod errors but warn
			fmt.Printf("warning: could not chmod downloaded binary: %v\n", err)
		}
	}

	if binPath == "" {
		return fmt.Errorf("could not locate matcha binary inside the release artifact")
	}

	// Replace the running executable with the new binary
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not determine executable path: %w", err)
	}

	// Write the new binary to a temp file in same dir, then rename for atomic replacement.
	execDir := filepath.Dir(execPath)
	tmpNew := filepath.Join(execDir, fmt.Sprintf("matcha.new.%d", time.Now().Unix()))
	in, err := os.Open(binPath)
	if err != nil {
		return fmt.Errorf("could not open new binary: %w", err)
	}
	out, err := os.OpenFile(tmpNew, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		in.Close()
		return fmt.Errorf("could not create temp binary in target dir: %w", err)
	}
	if _, err := io.Copy(out, in); err != nil {
		in.Close()
		out.Close()
		return fmt.Errorf("could not write new binary to disk: %w", err)
	}
	in.Close()
	out.Close()

	// On Windows, a running executable cannot be overwritten directly.
	// Move the old binary out of the way first, then rename the new one in.
	if runtime.GOOS == "windows" {
		oldPath := execPath + ".old"
		_ = os.Remove(oldPath) // clean up any previous leftover
		if err := os.Rename(execPath, oldPath); err != nil {
			return fmt.Errorf("could not move old executable out of the way: %w", err)
		}
	}

	if err := os.Rename(tmpNew, execPath); err != nil {
		return fmt.Errorf("could not replace executable: %w", err)
	}

	fmt.Println("Successfully updated matcha to", latestTag)
	return nil
}

func filterUnique(existing, incoming []fetcher.Email) []fetcher.Email {
	seen := make(map[uint32]struct{})
	for _, e := range existing {
		seen[e.UID] = struct{}{}
	}
	var unique []fetcher.Email
	for _, e := range incoming {
		if _, ok := seen[e.UID]; !ok {
			unique = append(unique, e)
		}
	}
	return unique
}

func main() {
	// If invoked with version flag, print version and exit
	if len(os.Args) > 1 && (os.Args[1] == "-v" || os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("matcha version %s", version)
		if commit != "" {
			fmt.Printf(" (%s)", commit)
		}
		if date != "" {
			fmt.Printf(" built on %s", date)
		}
		fmt.Println()
		os.Exit(0)
	}

	// If invoked as CLI update command, run updater and exit.
	if len(os.Args) > 1 && os.Args[1] == "update" {
		if err := runUpdateCLI(); err != nil {
			fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Gmail OAuth2 CLI subcommand: matcha gmail <auth|token|revoke> <email> [flags]
	if len(os.Args) > 1 && os.Args[1] == "gmail" {
		runGmailOAuthCLI(os.Args[2:])
		os.Exit(0)
	}

	// Send email CLI subcommand: matcha send --to <email> --subject <subject> [flags]
	if len(os.Args) > 1 && os.Args[1] == "send" {
		runSendCLI(os.Args[2:])
		os.Exit(0)
	}

	cfg, err := config.LoadConfig()
	if err == nil && cfg.Theme != "" {
		theme.SetTheme(cfg.Theme)
	}
	tui.RebuildStyles()

	// Ensure PGP keys directory exists
	_ = config.EnsurePGPDir()

	var initialModel *mainModel
	if err != nil {
		initialModel = newInitialModel(nil)
	} else {
		initialModel = newInitialModel(cfg)
	}

	// Initialize plugin system
	plugins := plugin.NewManager()
	plugins.LoadPlugins()
	initialModel.plugins = plugins
	plugins.CallHook(plugin.HookStartup)

	p := tea.NewProgram(initialModel)

	if _, err := p.Run(); err != nil {
		plugins.Close()
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}

	plugins.CallHook(plugin.HookShutdown)
	plugins.Close()
}
