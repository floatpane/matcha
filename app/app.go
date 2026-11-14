package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/mail"
	"net/url"
	"os"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	overlay "github.com/floatpane/bubble-overlay"
	"github.com/floatpane/matcha/backend"
	_ "github.com/floatpane/matcha/backend/imap"    // register the IMAP backend
	_ "github.com/floatpane/matcha/backend/jmap"    // register the JMAP backend
	_ "github.com/floatpane/matcha/backend/maildir" // register the Maildir backend
	_ "github.com/floatpane/matcha/backend/pop3"    // register the POP3 backend
	matchaCli "github.com/floatpane/matcha/cli"
	"github.com/floatpane/matcha/clib/macos"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
	"github.com/floatpane/matcha/daemonrpc"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/i18n"
	"github.com/floatpane/matcha/internal/editor"
	"github.com/floatpane/matcha/internal/emailactions"
	"github.com/floatpane/matcha/internal/emailstore"
	"github.com/floatpane/matcha/internal/exportcmd"
	"github.com/floatpane/matcha/internal/fetchcmd"
	"github.com/floatpane/matcha/internal/idledaemon"
	"github.com/floatpane/matcha/internal/logging"
	"github.com/floatpane/matcha/internal/loglevel"
	"github.com/floatpane/matcha/internal/notifications"
	"github.com/floatpane/matcha/internal/palette"
	"github.com/floatpane/matcha/internal/pluginbridge"
	"github.com/floatpane/matcha/internal/send"
	"github.com/floatpane/matcha/notify"
	"github.com/floatpane/matcha/plugin"
	"github.com/floatpane/matcha/theme"
	"github.com/floatpane/matcha/tui"
	"github.com/google/uuid"
)

// goosDarwin is the runtime.GOOS value for macOS.
const goosDarwin = "darwin"

// goosLinux is the runtime.GOOS value for Linux.
const goosLinux = "linux"

type logEntryMsg struct {
	entry logging.Entry
}

// clearErrorNotifMsg hides the error/info toast that was shown as gen; a
// newer toast is not cleared by an older toast's timer.
type clearErrorNotifMsg struct{ gen int }
type clearPluginNotifMsg struct{ gen int }

// badgeSyncMsg triggers a debounced unread-badge update.
type badgeSyncMsg struct{ gen int }

const maxBodyFetchRetries = 3

// Model is the main Bubble Tea application model that orchestrates the TUI.
type Model struct {
	current       tea.Model
	previousModel tea.Model
	config        *config.Config
	plugins       *plugin.Manager
	store         *emailstore.Store
	folderInbox   *tui.FolderInbox
	width         int
	height        int

	idleWatcher *fetcher.IdleWatcher
	idleUpdates chan fetcher.IdleUpdate

	providers   map[string]backend.Provider
	providersMu sync.RWMutex
	service     daemonclient.Service

	pendingPrompt *plugin.PendingPrompt
	mailtoURL     *url.URL

	showLogPanel bool
	logCh        <-chan logging.Entry
	oauthCh      chan tea.Msg
	logPanel     *tui.LogPanel

	palette *palette.Palette

	pendingJobID  string
	sendNotice    string
	pendingExport *emailstore.PendingExport

	actionManager *emailactions.Manager
	pluginBridge  *pluginbridge.Manager

	errorNotification overlay.Notification
	showErrorNotif    bool
	errorNotifGen     int

	pluginNotification overlay.Notification
	showPluginNotif    bool
	pluginNotifGen     int

	// graphics keeps out-of-band (Kitty/Sixel) images in sync with the view.
	graphics *tui.GraphicsPainter

	// pendingDraftID is the draft to delete once the email being sent is
	// confirmed; a failed send keeps it.
	pendingDraftID string

	badgeGen   int
	badgeDirty bool

	// Track body fetch retries to prevent infinite loops
	pendingBodyFetchUID   uint32
	pendingBodyFetchCount int
}

// NewModel creates the initial TUI model.
func NewModel(cfg *config.Config, mailtoURL *url.URL, plugins *plugin.Manager, showLogPanel bool, logCh <-chan logging.Entry, logPanel *tui.LogPanel) *Model {
	idleUpdates := make(chan fetcher.IdleUpdate, 16)
	store := emailstore.NewStore()
	m := &Model{
		current:       tui.NewChoice(),
		config:        cfg,
		plugins:       plugins,
		store:         store,
		idleUpdates:   idleUpdates,
		idleWatcher:   fetcher.NewIdleWatcher(idleUpdates),
		providers:     make(map[string]backend.Provider),
		mailtoURL:     mailtoURL,
		showLogPanel:  showLogPanel,
		logCh:         logCh,
		logPanel:      logPanel,
		palette:       palette.New(),
		graphics:      tui.NewGraphicsPainter(),
		actionManager: emailactions.NewManager(emailactions.Dependencies{Config: cfg}, store, nil),
		pluginBridge:  pluginbridge.NewManager(plugins, store, cfg, nil),
	}

	m.updateCurrentFromConfig(cfg, mailtoURL)
	m.updateSubordinates()
	return m
}

func (m *Model) updateCurrentFromConfig(cfg *config.Config, mailtoURL *url.URL) {
	if cfg == nil || !cfg.HasAccounts() {
		hideTips := false
		if cfg != nil {
			hideTips = cfg.HideTips
		}
		m.current = tui.NewLogin(hideTips)
		m.config = nil
		return
	}

	m.config = cfg
	config.MouseEnabled = cfg.MouseEnabled

	switch {
	case mailtoURL != nil:
		to, subject, body := parseMailto(mailtoURL)
		composer := tui.NewComposerWithAccounts(cfg.Accounts, cfg.Accounts[0].ID, to, subject, body, cfg.HideTips)
		composer.SetSpellcheckOptions(cfg.DisableSpellcheck, cfg.DisableSpellSuggestions)
		m.current = composer
	case !cfg.HasSeenSetupGuide:
		m.current = newSetupGuide()
	default:
		m.current = tui.NewChoice()
	}
}

func parseMailto(u *url.URL) (to, subject, body string) {
	to = u.Opaque
	if to == "" {
		to = u.Path
	}
	if to == "" {
		to = u.Query().Get("to")
	}
	return to, u.Query().Get("subject"), u.Query().Get("body")
}

func newSetupGuide() *tui.SetupGuide {
	isMac := runtime.GOOS == goosDarwin
	isLinux := runtime.GOOS == goosLinux

	// The helpers print progress meant for the CLI; while the TUI owns the
	// screen that output is discarded instead of drawn over the interface.
	var installHelper func() error
	if isMac {
		installHelper = func() error { return matchaCli.RunHelperTo(io.Discard, []string{"install"}) }
	}
	var setupMailto func() error
	if isMac || isLinux {
		setupMailto = func() error { return matchaCli.SetupMailtoTo(io.Discard) }
	}

	return tui.NewSetupGuide(isMac, isLinux, installHelper, setupMailto)
}

func (m *Model) newSettings() *tui.Settings {
	s := tui.NewSettings(m.config)
	if m.plugins != nil {
		s.SetPlugins(m.plugins)
	}
	return s
}

func getCurrentComposer(current tea.Model) *tui.Composer {
	if c, ok := current.(*tui.Composer); ok {
		return c
	}
	if rs, ok := current.(*tui.ReplySplitView); ok {
		return rs.Composer()
	}
	return nil
}

func (m *Model) applySpellcheckOptions(c *tui.Composer) {
	if c == nil || m.config == nil {
		return
	}
	c.SetSpellcheckOptions(m.config.DisableSpellcheck, m.config.DisableSpellSuggestions)
}

// resolveProvider runs inside the search command, so creating a provider
// here (JMAP authenticates over the network) never blocks the UI goroutine.
func (m *Model) resolveProvider(acct *config.Account) backend.Provider {
	if acct == nil {
		return nil
	}
	m.providersMu.RLock()
	p := m.providers[acct.ID]
	m.providersMu.RUnlock()
	if p != nil {
		return p
	}
	cp := *acct
	created, err := backend.New(&cp)
	if err != nil {
		log.Printf("backend: failed to create provider for %s: %v", acct.Email, err)
		return nil
	}
	m.providersMu.Lock()
	defer m.providersMu.Unlock()
	if existing := m.providers[acct.ID]; existing != nil {
		_ = created.Close()
		return existing
	}
	m.providers[acct.ID] = created
	return created
}

func (m *Model) Init() tea.Cmd {
	cmd := m.current.Init()
	if m.showLogPanel && m.logCh != nil {
		return tea.Batch(cmd, waitForLogEntry(m.logCh))
	}
	return cmd
}

func waitForLogEntry(ch <-chan logging.Entry) tea.Cmd {
	return func() tea.Msg {
		entry := <-ch
		return logEntryMsg{entry: entry}
	}
}

// syncUnreadBadge marks the dock badge as stale. The update itself is
// debounced and runs off the UI goroutine (see Update).
func (m *Model) syncUnreadBadge() {
	m.badgeDirty = true
}

// unreadBadgeCmd computes the unread count and updates the badge in the
// background.
func (m *Model) unreadBadgeCmd() tea.Cmd {
	count := notifications.CountUnread(m.store.EmailsByAcct, m.store.FolderEmails)
	loglevel.Debugf("unread badge count: %d", count)
	if runtime.GOOS != goosDarwin {
		return nil
	}
	return func() tea.Msg {
		if err := macos.SetBadge(count); err != nil {
			loglevel.Debugf("set badge: %v", err)
		}
		return nil
	}
}

func (m *Model) currentWindowSize() tea.WindowSizeMsg {
	return tea.WindowSizeMsg{
		Width:  m.width,
		Height: m.contentHeight(),
	}
}

func (m *Model) contentHeight() int {
	height := m.height - m.logPanelHeight()
	if height < 1 {
		return 1
	}
	return height
}

func (m *Model) renderWithLogPanel(content string) string {
	panelHeight := m.logPanelHeight()
	if panelHeight == 0 || m.logPanel == nil {
		return content
	}
	contentHeight := m.contentHeight()
	mainContent := lipgloss.NewStyle().
		MaxHeight(contentHeight).
		Height(contentHeight).
		Render(content)
	m.logPanel.SetSize(m.width, panelHeight)
	return lipgloss.JoinVertical(lipgloss.Left, mainContent, m.logPanel.View())
}

func (m *Model) logPanelHeight() int {
	if !m.showLogPanel || m.height < 12 || m.width < 20 {
		return 0
	}
	if m.height < 20 {
		return 4
	}
	return 7
}

// notificationWidth is the width of toast notifications, border included.
const notificationWidth = 42

// notificationOptions positions a toast in the top-right corner and keeps it
// inside the screen on narrow terminals.
func (m *Model) notificationOptions(opts ...overlay.Option) []overlay.Option {
	width := notificationWidth
	if m.width > 0 && m.width < width+2 {
		width = max(10, m.width-2)
	}
	col := max(0, m.width-width-2)
	return append([]overlay.Option{
		overlay.WithPosition(0, col),
		overlay.WithWidth(width),
	}, opts...)
}

func (m *Model) showErrorCmd(msg string) tea.Cmd {
	m.errorNotification = overlay.NewError(m.notificationOptions(
		overlay.WithMessage(msg),
		overlay.WithKey(config.Keybinds.Global.DismissNotification),
	)...)
	return m.armErrorNotif(8 * time.Second)
}

func (m *Model) showInfoCmd(msg string) tea.Cmd {
	m.errorNotification = overlay.NewInfo(m.notificationOptions(
		overlay.WithMessage(msg),
		overlay.WithKey(config.Keybinds.Global.DismissNotification),
	)...)
	return m.armErrorNotif(4 * time.Second)
}

// armErrorNotif shows the error/info toast and schedules its removal.
func (m *Model) armErrorNotif(d time.Duration) tea.Cmd {
	m.showErrorNotif = true
	m.errorNotifGen++
	gen := m.errorNotifGen
	return tea.Tick(d, func(time.Time) tea.Msg { return clearErrorNotifMsg{gen: gen} })
}

// notificationRects returns the screen areas covered by toasts, so images
// drawn out-of-band can be kept from covering them.
func (m *Model) notificationRects() [][4]int {
	var rects [][4]int
	add := func(box string) {
		if box == "" {
			return
		}
		w, h := lipgloss.Width(box), lipgloss.Height(box)
		rects = append(rects, [4]int{0, max(0, m.width-w-2), h, w + 2})
	}
	if m.showErrorNotif {
		add(m.errorNotification.Render())
	}
	if m.showPluginNotif {
		add(m.pluginNotification.Render())
	}
	if m.sendNotice != "" {
		add(m.noticeBox(m.sendNotice))
	}
	if m.actionManager.ActionNotice != "" {
		add(m.noticeBox(m.actionManager.ActionNotice))
	}
	return rects
}

// visibleImages returns the out-of-band images the current screen should
// show. Images are hidden behind modal overlays and under toasts.
func (m *Model) visibleImages() []tui.ScreenImage {
	if m.palette.IsOpen() {
		return nil
	}
	provider, ok := m.current.(tui.ImageProvider)
	if !ok {
		return nil
	}
	images := provider.VisibleImages()
	maxRow := m.contentHeight()
	kept := images[:0:0]
	for _, img := range images {
		if img.Row >= 0 && img.Row+img.Rows <= maxRow {
			kept = append(kept, img)
		}
	}
	for _, r := range m.notificationRects() {
		kept = tui.FilterImagesOutside(kept, r[0], r[1], r[2], r[3])
	}
	return kept
}

// isTypingText reports whether the current view is editing text, in which
// case global single-letter shortcuts (undo, dismiss) must not fire.
func (m *Model) isTypingText() bool {
	switch current := m.current.(type) {
	case *tui.Composer, *tui.ReplySplitView, *tui.Login, *tui.PasswordPrompt:
		return true
	case *tui.FolderInbox:
		if inbox := current.GetInbox(); inbox != nil {
			return inbox.IsSearchOverlayOpen() || inbox.IsFiltering()
		}
	case *tui.Inbox:
		return current.IsSearchOverlayOpen() || current.IsFiltering()
	}
	return false
}

// syncUnreadBadgeCmd debounces unread-badge updates; the badge helper is slow
// and must never run on the UI goroutine.
func (m *Model) syncUnreadBadgeCmd() tea.Cmd {
	if runtime.GOOS != goosDarwin && loglevel.Get() < loglevel.LevelDebug {
		return nil
	}
	m.badgeGen++
	gen := m.badgeGen
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return badgeSyncMsg{gen: gen} })
}

// noticeBox renders a small bordered notice that fits the screen width.
func (m *Model) noticeBox(text string) string {
	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(theme.ActiveTheme.Accent).
		Padding(0, 1)
	if m.width > 0 && lipgloss.Width(style.Render(text)) > m.width {
		style = style.Width(m.width)
	}
	return style.Render(text)
}

func (m *Model) renderSendNoticeOverlay(content string) string {
	lines := strings.Split(m.noticeBox(m.sendNotice), "\n")
	col := max(0, m.width-lipgloss.Width(lines[0]))
	return overlay.Block(content, lines, 0, col)
}

func (m *Model) renderActionNoticeOverlay(content string) string {
	lines := strings.Split(m.noticeBox(m.actionManager.ActionNotice), "\n")
	col := max(0, m.width-lipgloss.Width(lines[0]))
	return overlay.Block(content, lines, 0, col)
}

func (m *Model) folderName() string {
	if m.folderInbox != nil {
		return m.folderInbox.GetCurrentFolder()
	}
	return emailstore.FolderInbox
}

func (m *Model) updateSubordinates() {
	m.actionManager.UpdateDependencies(emailactions.Dependencies{Service: m.service, Config: m.config})
	m.actionManager.UpdateStore(m.store)
	m.actionManager.SetFolderInbox(m.folderInbox)
	m.pluginBridge.SetConfig(m.config)
	m.pluginBridge.SetFolderInbox(m.folderInbox)
}

func (m *Model) notifyNewMail(accountID, folderName string) {
	if m.config != nil && !m.config.DisableNotifications {
		accountName := accountID
		if acc := m.config.GetAccountByID(accountID); acc != nil {
			accountName = acc.Email
		}
		go notify.Send("Matcha", fmt.Sprintf("New mail in %s (%s)", folderName, accountName)) //nolint:errcheck
	}
}

func (m *Model) unsubscribeFolder(accountID, folderName string) {
	if m.service != nil && m.service.IsDaemon() {
		_ = m.service.Unsubscribe(accountID, folderName)
	} else {
		m.idleWatcher.Stop(accountID)
	}
}

func (m *Model) subscribeFolder(accountID, folderName string) {
	if m.service != nil && m.service.IsDaemon() {
		_ = m.service.Subscribe(accountID, folderName)
	} else {
		acc := m.config.GetAccountByID(accountID)
		if acc != nil {
			m.idleWatcher.Watch(acc, folderName)
		}
	}
}

func (m *Model) cachedAttachmentsToFetcher(cached []config.CachedAttachment) []fetcher.Attachment {
	attachments := make([]fetcher.Attachment, 0, len(cached))
	for _, ca := range cached {
		att := fetcher.Attachment{
			Filename:         ca.Filename,
			PartID:           ca.PartID,
			Encoding:         ca.Encoding,
			MIMEType:         ca.MIMEType,
			ContentID:        ca.ContentID,
			Inline:           ca.Inline,
			IsSMIMESignature: ca.IsSMIMESignature,
			SMIMEVerified:    ca.SMIMEVerified,
			IsSMIMEEncrypted: ca.IsSMIMEEncrypted,
			IsCalendarInvite: ca.IsCalendarInvite,
		}
		if ca.IsCalendarInvite && len(ca.CalendarData) > 0 {
			att.Data = ca.CalendarData
		}
		if ca.Inline && len(ca.Data) > 0 {
			att.Data = ca.Data
		}
		attachments = append(attachments, att)
	}
	return attachments
}

func cachedAttachmentToConfig(a fetcher.Attachment) config.CachedAttachment {
	ca := config.CachedAttachment{
		Filename:         a.Filename,
		PartID:           a.PartID,
		Encoding:         a.Encoding,
		MIMEType:         a.MIMEType,
		ContentID:        a.ContentID,
		Inline:           a.Inline,
		IsSMIMESignature: a.IsSMIMESignature,
		SMIMEVerified:    a.SMIMEVerified,
		IsSMIMEEncrypted: a.IsSMIMEEncrypted,
		IsCalendarInvite: a.IsCalendarInvite,
	}
	if a.IsCalendarInvite && len(a.Data) > 0 {
		ca.CalendarData = a.Data
	}
	if a.Inline && len(a.Data) > 0 {
		ca.Data = a.Data
	}
	return ca
}

func (m *Model) cacheEmailBody(folderName string, uid uint32, accountID, body, bodyMIMEType string, attachments []fetcher.Attachment) {
	cachedAttachments := make([]config.CachedAttachment, 0, len(attachments))
	for _, a := range attachments {
		cachedAttachments = append(cachedAttachments, cachedAttachmentToConfig(a))
	}
	threshold := m.config.GetBodyCacheThreshold()
	go func() {
		if err := config.SaveEmailBody(folderName, config.CachedEmailBody{
			UID:          uid,
			AccountID:    accountID,
			Body:         body,
			BodyMIMEType: bodyMIMEType,
			Attachments:  cachedAttachments,
		}, threshold); err != nil {
			loglevel.Debugf("error caching email body for UID %d: %v", uid, err)
		}
	}()
}

func (m *Model) pluginFlagCmds() []tea.Cmd {
	return m.pluginBridge.FlagCmds()
}

func (m *Model) pluginNotifyCmd() tea.Cmd {
	return m.pluginBridge.NotifyCmd()
}

func (m *Model) syncPluginStatus() {
	m.pluginBridge.SyncStatus(m.current)
}

func (m *Model) handlePluginKeyBinding(msg tea.KeyPressMsg) tea.Cmd {
	return m.pluginBridge.HandleKeyBinding(msg, m.current, &m.pendingPrompt)
}

func (m *Model) isSearchOverlayOpen() bool {
	return m.pluginBridge.IsSearchOverlayOpen(m.current)
}

func (m *Model) syncPluginKeyBindings() {
	m.pluginBridge.SyncKeyBindings(m.current)
}

func (m *Model) applyPluginFields(composer *tui.Composer) {
	m.pluginBridge.ApplyFields(composer)
}

func (m *Model) startComposerHooks(composer *tui.Composer) {
	m.plugins.CallComposerHook(plugin.HookComposerUpdated, composer.GetBody(), composer.GetSubject(), composer.GetTo(), composer.GetCc(), composer.GetBcc())
	m.syncPluginStatus()
	m.applyPluginFields(composer)
}

func (m *Model) saveDraftOnQuit(composer *tui.Composer) {
	if err := config.SaveDraft(composer.ToDraft()); err != nil {
		log.Printf("Error saving draft on quit: %v", err)
	}
}

func (m *Model) deleteDraftAfterSend(draftID string) {
	if draftID != "" {
		if err := config.DeleteDraft(draftID); err != nil {
			log.Printf("Error deleting draft after send: %v", err)
		}
	}
}

// finishDraftAfterSend deletes the draft of a message once it is sent.
func (m *Model) finishDraftAfterSend() {
	if id := m.pendingDraftID; id != "" {
		m.pendingDraftID = ""
		go m.deleteDraftAfterSend(id)
	}
}

func (m *Model) saveContactsAfterSend(msg tui.SendEmailMsg) {
	if msg.To == "" {
		return
	}
	for _, r := range strings.Split(msg.To, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		name, email := send.ParseEmailAddress(r)
		if err := config.AddContactForAccount(name, email, msg.AccountID); err != nil {
			log.Printf("Error saving contact: %v", err)
		}
	}
}

func (m *Model) sendEmailNotice(msg tui.SendEmailMsg) string {
	if msg.SignPGP {
		if account := m.config.GetAccountByID(msg.AccountID); account != nil && account.PGPKeySource == "yubikey" {
			return "Touch your YubiKey to sign..."
		}
	}
	return "Sending email..."
}

func (m *Model) shutdownService() {
	m.idleWatcher.StopAll()
	if m.service != nil {
		m.service.Close() //nolint:errcheck,gosec
		// Drop the closed service so a later shutdown (e.g. ctrl+c after
		// esc-ing back to the menu) doesn't close it twice and panic, and
		// re-entering the inbox builds a fresh one.
		m.service = nil
	}
}

func (m *Model) saveConfigAfterCredentials() bool {
	if err := config.SaveConfig(m.config); err != nil {
		log.Printf("could not save config: %v", err)
		return false
	}
	return true
}

func (m *Model) runOAuth2Flow(account config.Account) tea.Cmd {
	ch := make(chan tea.Msg, 16)
	m.oauthCh = ch
	go func() {
		err := config.RunOAuth2Flow(account.Email, account.ServiceProvider, "", "", func(line string) {
			ch <- tui.OAuth2ProgressMsg{Line: line}
		})
		ch <- tui.OAuth2CompleteMsg{Email: account.Email, Err: err}
	}()
	return waitForOAuthMsg(ch)
}

func waitForOAuthMsg(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// reloadService makes the mail service (daemon or direct) reload the config
// from disk, off the UI goroutine, and drops providers built from the old
// account settings.
func (m *Model) reloadService() {
	m.providersMu.Lock()
	m.providers = make(map[string]backend.Provider)
	m.providersMu.Unlock()
	if svc := m.service; svc != nil {
		go func() {
			if err := svc.ReloadConfig(); err != nil {
				log.Printf("config reload: %v", err)
			}
		}()
	}
}

func (m *Model) afterConfigSaved() {
	m.reloadService()
	if m.folderInbox != nil {
		m.folderInbox.SetDateFormat(m.config.GetDateFormat())
		m.folderInbox.SetDetailedDates(m.config.EnableDetailedDates)
		m.folderInbox.SetDefaultThreaded(m.config.EnableThreaded)
		m.folderInbox.SetDisableImages(m.config.DisableImages)
		m.folderInbox.SetSplitOrientation(m.config.GetSplitPaneOrientation())
	}
	m.updateSubordinates()
}

func (m *Model) afterLanguageChanged() {
	switch current := m.current.(type) {
	case *tui.Composer, *tui.Inbox, *tui.FolderInbox:
		m.current = tui.NewChoice()
	case *tui.Settings:
		// Rebuild with the new language but stay on the same page.
		state := current.GetState()
		s := m.newSettings()
		s.RestoreState(state)
		m.current = s
	default:
		m.current = m.newSettings()
	}
	m.current, _ = m.current.Update(m.currentWindowSize())
}

func (m *Model) deleteAccount(accountID string) error {
	if m.config == nil {
		return nil
	}
	var saveErr error
	if m.config.RemoveAccount(accountID) {
		if err := config.CleanupAccountCache(accountID); err != nil {
			log.Printf("could not clean account cache: %v", err)
		}
		if err := config.SaveConfig(m.config); err != nil {
			log.Printf("could not save config: %v", err)
			saveErr = err
		}
	}
	m.idleWatcher.Stop(accountID)
	// Providers are bound to account settings; rebuild them so actions on
	// the remaining accounts never reach the wrong server.
	m.reloadService()
	m.store.RemoveAccount(accountID)
	s := m.newSettings()
	s.RestoreState(tui.SettingsState{ActivePane: tui.PaneContent, ActiveCategory: tui.CategoryAccounts})
	m.current = s
	m.current, _ = m.current.Update(m.currentWindowSize())
	m.updateSubordinates()
	return saveErr
}

func (m *Model) loadFolderInboxCache(folderName string) {
	if cached := m.store.FolderEmails[folderName]; len(cached) > 0 {
		m.folderInbox.SetEmails(cached, m.config.Accounts)
	} else if diskCached := emailstore.LoadFolderEmailsFromCache(folderName); len(diskCached) > 0 {
		m.store.FolderEmails[folderName] = diskCached
		m.store.SetFolder(folderName, diskCached)
		m.folderInbox.SetEmails(diskCached, m.config.Accounts)
	}
}

func (m *Model) setFolderInboxDefaults() {
	m.folderInbox.SetDateFormat(m.config.GetDateFormat())
	m.folderInbox.SetDetailedDates(m.config.EnableDetailedDates)
	m.folderInbox.SetDefaultThreaded(m.config.EnableThreaded)
	m.folderInbox.SetDisableImages(m.config.DisableImages)
	m.folderInbox.SetSplitOrientation(m.config.GetSplitPaneOrientation())
}

func (m *Model) setFolderInboxEmails(folderName string, emails []fetcher.Email) {
	m.store.SetFolder(folderName, emails)
	m.folderInbox.SetEmails(emails, m.config.Accounts)
	m.folderInbox.GetInbox().SetFolderName(folderName)
	m.folderInbox.SetLoadingEmails(false)
	m.syncPluginStatus()
	m.syncPluginKeyBindings()
}

// cacheFolderEmailBodies saves the folder list and prunes cached bodies of
// mail that is gone. Accounts whose fetch failed are left untouched.
func (m *Model) cacheFolderEmailBodies(folderName string, emails []fetcher.Email, failed map[string]error) {
	emailstore.CacheFolderSnapshot(folderName, emails, failed, m.config.GetAccountIDs(), m.config.GetBodyCacheThreshold())
}

func (m *Model) refreshFolderEmails(folderName string) tea.Cmd {
	delete(m.store.FolderEmails, folderName)
	m.folderInbox.SetRefreshing(true)
	return fetchcmd.FetchFolderEmailsCmd(m.config, folderName)
}

func (m *Model) fetchCurrentFolderEmails(folderName string) tea.Cmd {
	if cached, ok := m.store.FolderEmails[folderName]; ok {
		m.setFolderInboxEmails(folderName, cached)
		return m.pluginNotifyCmd()
	}
	if diskCached := emailstore.LoadFolderEmailsFromCache(folderName); len(diskCached) > 0 {
		m.store.FolderEmails[folderName] = diskCached
		m.setFolderInboxEmails(folderName, diskCached)
		return tea.Batch(fetchcmd.FetchFolderEmailsCmd(m.config, folderName), m.pluginNotifyCmd())
	}
	m.folderInbox.SetLoadingEmails(true)
	return tea.Batch(fetchcmd.FetchFolderEmailsCmd(m.config, folderName), m.pluginNotifyCmd())
}

func (m *Model) updateDaemonSubscriptions(previousFolder, newFolder string) {
	for i := range m.config.Accounts {
		accID := m.config.Accounts[i].ID
		folders, _ := config.GetCachedFolders(accID)
		if !slices.Contains(folders, newFolder) {
			m.unsubscribeFolder(accID, previousFolder)
			continue
		}
		if previousFolder != "" {
			m.unsubscribeFolder(accID, previousFolder)
		}
		m.subscribeFolder(accID, newFolder)
	}
}

func (m *Model) markEmailReadAndQueue(accountID string, uid uint32, suppress bool) tea.Cmd {
	if suppress {
		return nil
	}
	m.store.MarkEmailAsReadInStores(uid, accountID)
	account := m.config.GetAccountByID(accountID)
	if account != nil {
		return fetchcmd.MarkEmailAsReadCmd(account, uid, accountID, m.folderName())
	}
	return nil
}

func (m *Model) maybeYubiKeyNotification(accountID string, attachments []fetcher.Attachment) tea.Cmd {
	if acct := m.config.GetAccountByID(accountID); acct != nil && acct.PGPKeySource == "yubikey" {
		for _, att := range attachments {
			if att.Filename == "pgp-status.internal" && att.IsPGPEncrypted {
				return m.showErrorCmd("Email decrypted with YubiKey")
			}
		}
	}
	return nil
}

func (m *Model) updateConfigFromCredentials(msg tui.Credentials) {
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

	if login, ok := m.current.(*tui.Login); ok && login.IsEditMode() {
		existingID := login.GetAccountID()
		account := buildAccount(existingID, msg, fetchEmails[0])
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
	} else {
		for _, fe := range fetchEmails {
			account := buildAccount(uuid.New().String(), msg, fe)
			m.config.AddAccount(account)
		}
	}
}

func buildAccount(id string, msg tui.Credentials, fetchEmail string) config.Account {
	account := config.Account{
		ID:              id,
		Name:            msg.Name,
		Email:           msg.Host,
		Password:        msg.Password,
		ServiceProvider: msg.Provider,
		FetchEmail:      fetchEmail,
		SendAsEmail:     msg.SendAsEmail,
		CatchAll:        msg.CatchAll,
		AuthMethod:      msg.AuthMethod,
		Protocol:        msg.Protocol,
		Insecure:        msg.Insecure,
		JMAPEndpoint:    msg.JMAPEndpoint,
		POP3Server:      msg.POP3Server,
		POP3Port:        msg.POP3Port,
		MaildirPath:     msg.MaildirPath,
		SC:              &config.SessionCache{},
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
	return account
}

func (m *Model) SetPasswordPrompt() {
	m.current = tui.NewPasswordPrompt()
}

func (m *Model) SetPlugins(plugins *plugin.Manager) {
	m.plugins = plugins
	m.pluginBridge.SetPlugins(plugins)
}

func (m *Model) SetLogPanel(logger *logging.Buffer) {
	m.showLogPanel = true
	m.logCh = logger.Subscribe()
	m.logPanel = tui.NewLogPanel(logger)
}

func (m *Model) Config() *config.Config {
	return m.config
}

func (m *Model) FolderInbox() *tui.FolderInbox {
	return m.folderInbox
}

func (m *Model) sendEmailDependencies() *send.Dependencies {
	return &send.Dependencies{Service: m.service, Config: m.config}
}

func (m *Model) batchActionCmd() tea.Cmd {
	if m.actionManager.IsPending() {
		return m.actionManager.FlushPendingAction()
	}
	return nil
}

func (m *Model) updateCurrentWindowSize() {
	m.current, _ = m.current.Update(m.currentWindowSize())
}

func (m *Model) setChoiceMenu() tea.Cmd {
	m.current = tui.NewChoice()
	m.updateCurrentWindowSize()
	return m.current.Init()
}

func (m *Model) restoreView() {
	if m.previousModel != nil {
		m.current = m.previousModel
		m.previousModel = nil
	}
}

// Update handles all incoming Bubble Tea messages and orchestrates the TUI.
// After every message it reconciles out-of-band images with the new screen
// and schedules background chores that the handlers flagged.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var pre tea.Cmd
	switch msg := msg.(type) {
	case tui.GraphicsPaintMsg:
		return m, m.graphics.Paint(msg)
	case badgeSyncMsg:
		if msg.gen != m.badgeGen {
			return m, nil
		}
		return m, m.unreadBadgeCmd()
	case tea.WindowSizeMsg:
		// The renderer repaints everything after a resize.
		pre = m.graphics.Reset(false)
	case tui.EditorFinishedMsg, tui.PatchApplyResultMsg:
		// An external program had the terminal; uploaded images may be gone.
		pre = m.graphics.Reset(true)
	}

	previous := m.current
	model, cmd := m.update(msg)

	cmds := []tea.Cmd{pre, cmd}
	if m.width > 0 && viewChanged(previous, m.current) {
		// Views only receive resize events while they are active; one that
		// comes back (e.g. the folder list after viewing an email) may
		// still be laid out for an older terminal size.
		var sizeCmd tea.Cmd
		m.current, sizeCmd = m.current.Update(m.currentWindowSize())
		cmds = append(cmds, sizeCmd)
	}
	if m.badgeDirty {
		m.badgeDirty = false
		cmds = append(cmds, m.syncUnreadBadgeCmd())
	}
	cmds = append(cmds, m.graphics.Sync(m.visibleImages()))
	return model, tea.Batch(cmds...)
}

// quitCmd quits the program. Kitty images on screen are removed first and
// the quit waits one frame so the removal reaches the terminal (the program
// does not flush pending output when it shuts down).
func (m *Model) quitCmd() tea.Cmd {
	clearImages := m.graphics.Reset(false)
	if clearImages == nil {
		return tea.Quit
	}
	return tea.Sequence(clearImages, tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg { return tea.QuitMsg{} }))
}

// viewChanged reports whether the active view is a different model. Views
// are compared by identity; value-type views (like the status screen) are
// treated as unchanged, as comparing them could panic and they do not lay
// themselves out.
func viewChanged(a, b tea.Model) bool {
	if a == nil || b == nil {
		return (a == nil) != (b == nil)
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return true
	}
	if ta.Kind() != reflect.Pointer {
		return false
	}
	return reflect.ValueOf(a).Pointer() != reflect.ValueOf(b).Pointer()
}

// update is the message handler behind Update.
func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) { //nolint:gocyclo
	var cmd tea.Cmd
	var cmds []tea.Cmd
	searchWasActive := false
	filterWasActive := false
	splitWasOpen := false

	if _, ok := msg.(logEntryMsg); ok {
		return m, waitForLogEntry(m.logCh)
	}

	// Drive the plugin notification's countdown (its progress bar).
	if m.showPluginNotif {
		if _, isKey := msg.(tea.KeyPressMsg); !isKey {
			if updated, notifCmd := m.pluginNotification.Update(msg); notifCmd != nil {
				if n, ok := updated.(overlay.Notification); ok {
					m.pluginNotification = n
					if n.Done() {
						m.showPluginNotif = false
						return m, nil
					}
					return m, notifCmd
				}
			} else if n, ok := updated.(overlay.Notification); ok {
				m.pluginNotification = n
				if n.Done() {
					m.showPluginNotif = false
				}
			}
		}
	}

	if msg, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = msg.Width
		m.height = msg.Height
		m.palette.UpdateSize(m.width, m.contentHeight())
		m.current, cmd = m.current.Update(m.currentWindowSize())
		return m, cmd
	}

	if m.palette.IsOpen() {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			switch keyMsg.String() {
			case "ctrl+c":
				m.shutdownService()
				return m, m.quitCmd()
			case config.Keybinds.Global.Cancel:
				m.palette.Close()
				return m, nil
			case "enter":
				actionCmd := m.palette.HandleKey(keyMsg)
				m.palette.Close()
				return m, actionCmd
			default:
				return m, m.palette.HandleKey(keyMsg)
			}
		}
		// The palette is modal: mouse events never reach the view below.
		if mouse, ok := msg.(tea.MouseMsg); ok {
			return m, m.palette.HandleMouse(mouse)
		}
	} else if keyMsg, ok := msg.(tea.KeyPressMsg); ok &&
		config.Keybinds.Global.CommandPalette != "" &&
		keyMsg.String() == config.Keybinds.Global.CommandPalette &&
		palette.Allowed(m.current) {
		cmd := m.palette.Open(palette.BuildCommands(m.current, m.folderInbox, m.config), m.width, m.contentHeight())
		return m, cmd
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == config.Keybinds.Global.Cancel {
		switch current := m.current.(type) {
		case *tui.Inbox:
			searchWasActive = current.IsSearchActive()
			filterWasActive = current.IsFilterActive()
		case *tui.FolderInbox:
			if inbox := current.GetInbox(); inbox != nil {
				searchWasActive = inbox.IsSearchActive()
				filterWasActive = inbox.IsFilterActive()
			}
			splitWasOpen = current.HasSplitPreview()
		}
	}

	m.current, cmd = m.current.Update(msg)
	cmds = append(cmds, cmd)

	if keyMsg, isKey := msg.(tea.KeyPressMsg); isKey {
		if composer := getCurrentComposer(m.current); composer != nil && m.plugins != nil {
			m.startComposerHooks(composer)
		}
		if m.plugins != nil && !m.isSearchOverlayOpen() {
			if bindingCmd := m.handlePluginKeyBinding(keyMsg); bindingCmd != nil {
				cmds = append(cmds, bindingCmd)
			}
		}
	}

	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		typing := m.isTypingText()
		if !typing && m.showErrorNotif && msg.String() == config.Keybinds.Global.DismissNotification {
			m.showErrorNotif = false
			return m, tea.Batch(cmds...)
		}
		if !typing && m.showPluginNotif && msg.String() == config.Keybinds.Global.DismissNotification {
			m.showPluginNotif = false
			return m, tea.Batch(cmds...)
		}
		if !typing && msg.String() == config.Keybinds.Composer.UndoSend {
			if m.actionManager.IsPending() {
				m.actionManager.RestorePendingAction()
				return m, tea.Batch(cmds...)
			}
			if m.pendingJobID != "" {
				jobID := m.pendingJobID
				svc := m.service // read on the UI goroutine; shutdownService may nil it
				m.pendingJobID = ""
				if svc == nil {
					return m, m.showErrorCmd("Could not undo: not connected to the mail service")
				}
				return m, func() tea.Msg {
					if err := svc.CancelEmail(jobID); err != nil {
						return tui.EmailResultMsg{Err: fmt.Errorf("could not undo: email may have already been sent")}
					}
					return tui.UndoSendMsg{JobID: jobID}
				}
			}
		}
		if msg.String() == "ctrl+c" {
			if composer := getCurrentComposer(m.current); composer != nil && composer.HasContent() {
				m.saveDraftOnQuit(composer)
			}
			// Commit a delete/archive/move that is still in its undo window
			// instead of silently dropping it.
			flush := m.batchActionCmd()
			m.shutdownService()
			if flush != nil {
				return m, tea.Sequence(flush, m.quitCmd())
			}
			return m, m.quitCmd()
		}
		if msg.String() == "esc" {
			switch m.current.(type) {
			case *tui.FolderInbox, *tui.Inbox, *tui.Login:
				if searchWasActive || filterWasActive || splitWasOpen {
					return m, tea.Batch(cmds...)
				}
				m.shutdownService()
				return m, m.setChoiceMenu()
			}
		}

	case tui.BackToInboxMsg:
		if m.folderInbox != nil {
			m.current = m.folderInbox
		} else {
			return m, m.setChoiceMenu()
		}
		return m, nil

	case tui.BackToMailboxMsg:
		if m.folderInbox != nil {
			m.current = m.folderInbox
			return m, nil
		}
		return m, m.setChoiceMenu()

	case tui.DiscardDraftMsg:
		if msg.ComposerState != nil {
			draft := msg.ComposerState.ToDraft()
			if err := config.SaveDraft(draft); err != nil {
				log.Printf("Error saving draft: %v", err)
				// Stay in the composer so the text is not lost.
				return m, m.showErrorCmd(fmt.Sprintf("Could not save draft: %v", err))
			}
		}
		return m, m.setChoiceMenu()

	case tui.OAuth2ProgressMsg:
		if login, ok := m.current.(*tui.Login); ok {
			login.AddOAuthStatus(msg.Line)
		}
		return m, waitForOAuthMsg(m.oauthCh)

	case tui.OAuth2CompleteMsg:
		m.oauthCh = nil
		if msg.Err != nil {
			log.Printf("OAuth2 authorization failed: %v", msg.Err)
			return m, tea.Batch(m.setChoiceMenu(), m.showErrorCmd(fmt.Sprintf("Authorization failed: %v", msg.Err)))
		}
		return m, m.setChoiceMenu()

	case tui.Credentials:
		m.updateConfigFromCredentials(msg)
		if !m.saveConfigAfterCredentials() {
			return m, m.showErrorCmd("Could not save the account configuration; see the log for details")
		}
		m.reloadService()
		lastAccount := m.config.Accounts[len(m.config.Accounts)-1]
		isEdit := false
		if login, ok := m.current.(*tui.Login); ok {
			isEdit = login.IsEditMode()
			if acc := m.config.GetAccountByID(login.GetAccountID()); acc != nil {
				lastAccount = *acc
			}
		}
		if lastAccount.IsOAuth2() {
			return m, m.runOAuth2Flow(lastAccount)
		}
		if isEdit {
			m.current = m.newSettings()
		} else {
			m.current = tui.NewChoice()
		}
		m.updateCurrentWindowSize()
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
		seen := make(map[string]bool)
		var cachedFolders []string
		unread := make(map[string]int)
		for _, acc := range m.config.Accounts {
			folders, counters := config.GetCachedFolders(acc.ID)
			for _, f := range folders {
				if !seen[f] {
					seen[f] = true
					cachedFolders = append(cachedFolders, f)
				}
				if count, ok := counters[f]; ok {
					unread[f] += count
				}
			}
		}
		if !seen[emailstore.FolderInbox] {
			cachedFolders = append([]string{emailstore.FolderInbox}, cachedFolders...)
		}
		m.folderInbox = tui.NewFolderInbox(cachedFolders, m.config.Accounts)
		m.folderInbox.SetUnreadCounts(unread)
		m.setFolderInboxDefaults()
		m.loadFolderInboxCache(emailstore.FolderInbox)
		m.current = m.folderInbox
		m.updateCurrentWindowSize()
		if m.service == nil {
			m.service = daemonclient.NewService(m.config)
			m.updateSubordinates()
		}
		batchCmds := []tea.Cmd{
			m.current.Init(),
			fetchcmd.FetchFoldersCmd(m.config),
			fetchcmd.FetchFolderEmailsCmd(m.config, emailstore.FolderInbox),
			idledaemon.ListenForIdleUpdates(m.idleUpdates),
		}
		if m.service.IsDaemon() {
			for _, acct := range m.config.Accounts {
				_ = m.service.Subscribe(acct.ID, emailstore.FolderInbox)
			}
			batchCmds = append(batchCmds, idledaemon.ListenForDaemonEvents(m.service.Events()))
		} else {
			for i := range m.config.Accounts {
				m.idleWatcher.Watch(&m.config.Accounts[i], emailstore.FolderInbox)
			}
		}
		return m, tea.Batch(batchCmds...)

	case tui.FoldersFetchedMsg:
		if m.folderInbox == nil {
			return m, nil
		}
		var folderNames []string
		unread := make(map[string]int)
		for _, f := range msg.MergedFolders {
			folderNames = append(folderNames, f.Name)
			if f.Unread > 0 {
				unread[f.Name] = int(f.Unread)
			}
		}
		m.folderInbox.SetFolders(folderNames)
		m.folderInbox.SetUnreadCounts(unread)
		type accountFolders struct {
			id     string
			names  []string
			unread map[string]int
		}
		toSave := make([]accountFolders, 0, len(msg.FoldersByAccount))
		for accID, folders := range msg.FoldersByAccount {
			af := accountFolders{id: accID, unread: make(map[string]int)}
			for _, f := range folders {
				af.names = append(af.names, f.Name)
				if f.Unread > 0 {
					af.unread[f.Name] = int(f.Unread)
				}
			}
			toSave = append(toSave, af)
		}
		// The folder cache is one shared file: save sequentially, never in
		// concurrent goroutines that overwrite each other.
		go func() {
			for _, af := range toSave {
				if err := config.SaveAccountFolders(af.id, af.names, af.unread); err != nil {
					log.Printf("could not save folder cache for %s: %v", af.id, err)
				}
			}
		}()
		if len(msg.Errors) > 0 {
			lookup := map[string]string{}
			for _, acc := range m.config.Accounts {
				name := acc.Email
				if name == "" {
					name = acc.Name
				}
				if name == "" {
					name = acc.ID
				}
				lookup[acc.ID] = name
			}
			parts := make([]string, 0, len(msg.Errors))
			for accID, err := range msg.Errors {
				name := lookup[accID]
				if name == "" {
					name = accID
				}
				parts = append(parts, fmt.Sprintf("%s: %v", name, err))
			}
			sort.Strings(parts)
			return m, m.showErrorCmd(fmt.Sprintf(
				"Folder fetch failed for %d account(s): %s",
				len(parts), strings.Join(parts, "; "),
			))
		}
		return m, nil

	case tui.SwitchFolderMsg:
		if m.config == nil {
			return m, nil
		}
		m.updateDaemonSubscriptions(msg.PreviousFolder, msg.FolderName)
		if m.plugins != nil {
			m.plugins.CallFolderHook(plugin.HookFolderChanged, msg.FolderName)
			m.syncPluginStatus()
			m.syncPluginKeyBindings()
		}
		return m, m.fetchCurrentFolderEmails(msg.FolderName)

	case tui.PluginNotifyMsg:
		// Build a non-blocking overlay notification instead of swapping the
		// full-screen view. The notification is composited on top of the
		// current view via PlaceOn() in View().
		dur := time.Duration(msg.Duration * float64(time.Second))
		if dur <= 0 {
			dur = 2 * time.Second
		}
		opts := m.notificationOptions(
			overlay.WithMessage(msg.Message),
			overlay.WithDuration(dur),
		)
		if msg.Title != "" {
			opts = append(opts, overlay.WithTitle(msg.Title))
		}
		if msg.Closable {
			opts = append(opts,
				overlay.WithKey(config.Keybinds.Global.DismissNotification),
				overlay.WithDismissMode(overlay.DismissEither),
			)
		} else {
			opts = append(opts, overlay.WithDismissMode(overlay.DismissAfterTimer))
		}
		switch msg.Kind {
		case "warning":
			m.pluginNotification = overlay.NewWarning(opts...)
		case "error":
			m.pluginNotification = overlay.NewError(opts...)
		default:
			m.pluginNotification = overlay.NewInfo(opts...)
		}
		m.showPluginNotif = true
		m.pluginNotifGen++
		gen := m.pluginNotifGen
		// Init starts the notification's own countdown so its progress bar moves.
		return m, tea.Batch(
			m.pluginNotification.Init(),
			tea.Tick(dur, func(time.Time) tea.Msg { return clearPluginNotifMsg{gen: gen} }),
		)

	case tui.PluginPromptSubmitMsg:
		if m.pendingPrompt != nil {
			if composer := getCurrentComposer(m.current); composer != nil {
				composer.HidePluginPrompt()
				m.plugins.ResolvePrompt(m.pendingPrompt, msg.Value)
				m.applyPluginFields(composer)
				m.syncPluginStatus()
			}
			m.pendingPrompt = nil
		}
		return m, nil

	case tui.PluginPromptCancelMsg:
		if composer := getCurrentComposer(m.current); composer != nil {
			composer.HidePluginPrompt()
		}
		m.pendingPrompt = nil
		return m, nil

	case tui.FolderEmailsFetchedMsg:
		if m.folderInbox == nil {
			return m, nil
		}
		previous := m.store.FolderEmails[msg.FolderName]
		if len(previous) == 0 {
			previous = emailstore.LoadFolderEmailsFromCache(msg.FolderName)
		}
		// Accounts whose fetch failed keep their known mail instead of vanishing.
		emails := emailstore.MergeFolderFetch(previous, msg.Emails, msg.Errors)
		if m.plugins != nil {
			// Only mail that actually arrived, not the whole folder on every refresh.
			for _, email := range emailstore.NewEmails(previous, msg.Emails) {
				t := m.plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, msg.FolderName)
				m.plugins.CallHook(plugin.HookEmailReceived, t)
			}
		}
		m.store.FolderEmails[msg.FolderName] = emails
		m.cacheFolderEmailBodies(msg.FolderName, emails, msg.Errors)
		var errCmd tea.Cmd
		if len(msg.Errors) > 0 {
			errCmd = m.showErrorCmd(fmt.Sprintf("Could not refresh %s for %d account(s); showing cached mail", msg.FolderName, len(msg.Errors)))
		}
		if m.folderInbox.GetCurrentFolder() != msg.FolderName {
			return m, errCmd
		}
		m.setFolderInboxEmails(msg.FolderName, emails)
		return m, tea.Batch(append(m.pluginFlagCmds(), m.pluginNotifyCmd(), errCmd)...)

	case tui.FetchFolderMoreEmailsMsg:
		if msg.AccountID == "" || m.config == nil {
			return m, nil
		}
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			return m, nil
		}
		limit := uint32(emailstore.PaginationLimit)
		if msg.Limit > 0 {
			limit = msg.Limit
		}
		return m, tea.Batch(
			func() tea.Msg { return tui.FetchingMoreEmailsMsg{} },
			fetchcmd.FetchFolderEmailsPaginatedCmd(account, msg.FolderName, limit, msg.Offset),
		)

	case tui.FolderEmailsAppendedMsg:
		if m.folderInbox == nil || m.folderInbox.GetCurrentFolder() != msg.FolderName {
			return m, nil
		}
		m.folderInbox.Update(msg)
		m.store.AppendToFolder(msg.FolderName, msg.Emails)
		m.cacheFolderEmailBodies(msg.FolderName, m.store.FolderEmails[msg.FolderName], nil)
		m.syncUnreadBadge()
		return m, nil

	case tui.MoveEmailToFolderMsg:
		if m.config == nil {
			return m, nil
		}
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			return m, nil
		}
		folderName := m.folderName()
		if m.folderInbox != nil {
			m.folderInbox.GetInbox().RemoveEmail(msg.UID, msg.AccountID)
		}
		flushCmd := m.batchActionCmd()
		pa, notice := m.actionManager.HandleMoveEmailMsg(msg, folderName)
		return m, tea.Batch(flushCmd, m.actionManager.StartActionGracePeriod(pa, notice))

	case tui.UpdatePreviewMsg:
		if m.folderInbox == nil {
			return m, nil
		}
		folderName := m.folderName()
		if cached := config.GetCachedEmailBody(folderName, msg.UID, msg.AccountID, m.config.GetBodyCacheThreshold()); cached != nil {
			attachments := m.cachedAttachmentsToFetcher(cached.Attachments)
			return m, func() tea.Msg {
				return tui.PreviewBodyFetchedMsg{
					UID:          msg.UID,
					Body:         cached.Body,
					BodyMIMEType: cached.BodyMIMEType,
					Attachments:  attachments,
					AccountID:    msg.AccountID,
					Folder:       folderName,
				}
			}
		}
		return m, fetchcmd.FetchPreviewBodyCmd(m.config, msg.UID, msg.AccountID, folderName)

	case tui.PreviewBodyFetchedMsg:
		if msg.Err != nil {
			log.Printf("preview body fetch error for UID %d: %v", msg.UID, msg.Err)
			m.pendingBodyFetchUID = 0
			m.pendingBodyFetchCount = 0
			return m, m.showErrorCmd(msg.Err.Error())
		}
		if m.folderInbox == nil {
			m.pendingBodyFetchUID = 0
			m.pendingBodyFetchCount = 0
			return m, nil
		}
		previewFolder := msg.Folder
		if previewFolder == "" {
			previewFolder = m.folderName()
		}
		if previewFolder != m.folderName() {
			// The user switched folders while this loaded: UIDs are per
			// folder, so it must not be shown (or cached) as the new folder's.
			if strings.TrimSpace(msg.Body) != "" {
				m.cacheEmailBody(previewFolder, msg.UID, msg.AccountID, msg.Body, msg.BodyMIMEType, msg.Attachments)
			}
			return m, nil
		}
		// If body is empty or whitespace-only, retry the fetch (up to max retries)
		if strings.TrimSpace(msg.Body) == "" {
			if m.pendingBodyFetchUID == msg.UID && m.pendingBodyFetchCount >= maxBodyFetchRetries {
				log.Printf("preview body still empty after %d retries for UID %d, showing error", maxBodyFetchRetries, msg.UID)
				m.pendingBodyFetchUID = 0
				m.pendingBodyFetchCount = 0
				return m, m.showErrorCmd("Email body could not be loaded after multiple attempts")
			}
			m.pendingBodyFetchUID = msg.UID
			m.pendingBodyFetchCount++
			log.Printf("preview body empty for UID %d, retrying fetch (%d/%d)", msg.UID, m.pendingBodyFetchCount, maxBodyFetchRetries)
			return m, tea.Batch(
				m.current.Init(),
				fetchcmd.FetchPreviewBodyCmd(m.config, msg.UID, msg.AccountID, previewFolder),
			)
		}
		// Reset retry counter on successful fetch
		m.pendingBodyFetchUID = 0
		m.pendingBodyFetchCount = 0
		// Cache the valid body
		m.cacheEmailBody(previewFolder, msg.UID, msg.AccountID, msg.Body, msg.BodyMIMEType, msg.Attachments)
		m.current, cmd = m.current.Update(msg)
		return m, cmd

	case tui.EmailMovedMsg:
		if msg.Err != nil {
			log.Printf("Move failed: %v", msg.Err)
			return m, m.showErrorCmd(msg.Err.Error())
		}
		return m, nil

	case tui.CachedEmailsLoadedMsg:
		if m.folderInbox == nil {
			return m, nil
		}
		return m, fetchcmd.FetchFolderEmailsCmd(m.config, m.folderInbox.GetCurrentFolder())

	case tui.IdleNewMailMsg:
		m.notifyNewMail(msg.AccountID, msg.FolderName)
		if m.folderInbox != nil && m.folderInbox.GetCurrentFolder() == msg.FolderName {
			return m, tea.Batch(
				fetchcmd.FetchFolderEmailsCmd(m.config, msg.FolderName),
				idledaemon.ListenForIdleUpdates(m.idleUpdates),
			)
		}
		return m, idledaemon.ListenForIdleUpdates(m.idleUpdates)

	case tui.DaemonEventMsg:
		if msg.Event == nil {
			// Events channel closed — daemon may have reconnected with a
			// fresh channel. Re-subscribe if still in daemon mode.
			if m.service != nil && m.service.IsDaemon() {
				return m, idledaemon.ListenForDaemonEvents(m.service.Events())
			}
			return m, nil
		}
		var daemonCmds []tea.Cmd
		if m.service != nil && m.service.IsDaemon() {
			daemonCmds = append(daemonCmds, idledaemon.ListenForDaemonEvents(m.service.Events()))
		}
		switch msg.Event.Type {
		case daemonrpc.EventNewMail:
			var ev daemonrpc.NewMailEvent
			if err := json.Unmarshal(msg.Event.Data, &ev); err == nil {
				m.notifyNewMail(ev.AccountID, ev.Folder)
				if m.folderInbox != nil && m.folderInbox.GetCurrentFolder() == ev.Folder {
					daemonCmds = append(daemonCmds, fetchcmd.FetchFolderEmailsCmd(m.config, ev.Folder))
				}
			}
		case daemonrpc.EventSendFailed:
			var ev daemonrpc.SendFailedEvent
			if err := json.Unmarshal(msg.Event.Data, &ev); err == nil {
				daemonCmds = append(daemonCmds, m.showErrorCmd(fmt.Sprintf("Email %q was not sent: %s", ev.Subject, ev.Error)))
			}
		case daemonrpc.EventSyncComplete:
			var ev daemonrpc.SyncCompleteEvent
			if err := json.Unmarshal(msg.Event.Data, &ev); err == nil {
				if m.folderInbox != nil && m.folderInbox.GetCurrentFolder() == ev.Folder {
					daemonCmds = append(daemonCmds, fetchcmd.FetchFolderEmailsCmd(m.config, ev.Folder))
				}
			}
		}
		return m, tea.Batch(daemonCmds...)

	case tui.RequestRefreshMsg:
		if msg.FolderName != "" && m.config != nil {
			return m, m.refreshFolderEmails(msg.FolderName)
		}
		return m, tea.Batch(
			func() tea.Msg { return tui.RefreshingEmailsMsg{Mailbox: msg.Mailbox} },
			fetchcmd.RefreshEmails(m.config, msg.Mailbox, msg.Counts),
		)

	case tui.EmailsRefreshedMsg:
		m.store.MergeRefreshed(msg.EmailsByAccount)
		m.syncUnreadBadge()
		if m.folderInbox != nil {
			m.folderInbox.SetEmails(m.store.Emails, m.config.Accounts)
			m.folderInbox.GetInbox().Update(msg)
		}
		return m, nil

	case tui.AllEmailsFetchedMsg:
		m.store.EmailsByAcct = msg.EmailsByAccount
		m.store.Emails = emailstore.FlattenAndSort(msg.EmailsByAccount)
		m.syncUnreadBadge()
		if m.folderInbox != nil {
			m.folderInbox.SetEmails(m.store.Emails, m.config.Accounts)
			m.folderInbox.SetLoadingEmails(false)
		}
		return m, nil

	case tui.EmailsFetchedMsg:
		if m.store.EmailsByAcct == nil {
			m.store.EmailsByAcct = make(map[string][]fetcher.Email)
		}
		m.store.EmailsByAcct[msg.AccountID] = msg.Emails
		m.store.Emails = emailstore.FlattenAndSort(m.store.EmailsByAcct)
		m.syncUnreadBadge()
		if m.folderInbox != nil {
			m.folderInbox.SetEmails(m.store.Emails, m.config.Accounts)
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
		limit := uint32(emailstore.PaginationLimit)
		if msg.Limit > 0 {
			limit = msg.Limit
		}
		folderName := m.folderName()
		return m, tea.Batch(
			func() tea.Msg { return tui.FetchingMoreEmailsMsg{} },
			fetchcmd.FetchFolderEmailsPaginatedCmd(account, folderName, limit, msg.Offset),
		)

	case tui.SearchRequestedMsg:
		folderName := msg.FolderName
		if folderName == "" {
			folderName = emailstore.FolderInbox
		}
		return m, fetchcmd.SearchEmailsCmd(m.config, m.resolveProvider, msg.Query, folderName, msg.AccountID)

	case tui.EmailsAppendedMsg:
		if m.store.EmailsByAcct == nil {
			m.store.EmailsByAcct = make(map[string][]fetcher.Email)
		}
		unique := filterUnique(m.store.EmailsByAcct[msg.AccountID], msg.Emails)
		m.store.EmailsByAcct[msg.AccountID] = append(m.store.EmailsByAcct[msg.AccountID], unique...)
		m.store.Emails = append(m.store.Emails, unique...)
		m.syncUnreadBadge()
		return m, nil

	case tui.GoToSendMsg:
		hideTips := false
		if m.config != nil {
			hideTips = m.config.HideTips
		}
		var composer *tui.Composer
		if m.config != nil && len(m.config.Accounts) > 0 {
			firstAccount := m.config.GetFirstAccount()
			composer = tui.NewComposerWithAccounts(m.config.Accounts, firstAccount.ID, msg.To, msg.Subject, msg.Body, hideTips)
		} else {
			composer = tui.NewComposer("", msg.To, msg.Subject, msg.Body, hideTips)
		}
		if m.config != nil {
			composer.SetShowCcBcc(m.config.ShowCcBccByDefault)
		}
		m.applySpellcheckOptions(composer)
		m.current = composer
		m.updateCurrentWindowSize()
		m.syncPluginKeyBindings()
		return m, m.current.Init()

	case tui.GoToDraftsMsg:
		drafts := config.GetAllDrafts()
		m.current = tui.NewDrafts(drafts)
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.OpenDraftMsg:
		var accounts []config.Account
		hideTips := false
		if m.config != nil {
			accounts = m.config.Accounts
			hideTips = m.config.HideTips
		}
		composer := tui.NewComposerFromDraft(msg.Draft, accounts, hideTips)
		if m.config != nil {
			composer.SetShowCcBcc(m.config.ShowCcBccByDefault)
		}
		m.applySpellcheckOptions(composer)
		m.current = composer
		m.updateCurrentWindowSize()
		m.syncPluginKeyBindings()
		return m, m.current.Init()

	case tui.DeleteSavedDraftMsg:
		go func() {
			if err := config.DeleteDraft(msg.DraftID); err != nil {
				log.Printf("Error deleting draft: %v", err)
			}
		}()
		m.current, cmd = m.current.Update(tui.DraftDeletedMsg{DraftID: msg.DraftID})
		return m, cmd

	case tui.GoToMarketplaceMsg:
		m.current = tui.NewMarketplace(false)
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.ConfigSavedMsg:
		m.afterConfigSaved()
		return m, nil

	case tui.ToggleSplitOrientationMsg:
		if m.config.EnableSplitPane && m.folderInbox != nil && m.folderInbox.HasSplitPreview() {
			if m.config.GetSplitPaneOrientation() == config.SplitPaneVertical {
				m.config.SplitPaneOrientation = config.SplitPaneHorizontal
			} else {
				m.config.SplitPaneOrientation = config.SplitPaneVertical
			}
			m.folderInbox.SetSplitOrientation(m.config.GetSplitPaneOrientation())
			m.current, cmd = m.current.Update(m.currentWindowSize())
			return m, cmd
		}
		return m, nil

	case tui.OpenFullscreenFromSplitMsg:
		if m.folderInbox != nil && m.folderInbox.HasSplitPreview() {
			previewPane := m.folderInbox.GetPreviewPane()
			if previewPane == nil {
				return m, nil
			}
			email := previewPane.GetEmail()
			m.folderInbox.CloseSplitPreview()
			mailbox := tui.MailboxInbox
			if inbox := m.folderInbox.GetInbox(); inbox != nil {
				mailbox = inbox.GetMailbox()
			}
			emailIndex := m.store.GetEmailIndex(email.UID, email.AccountID)
			sz := m.currentWindowSize()
			emailView := tui.NewEmailView(email, emailIndex, sz.Width, sz.Height, mailbox, m.config.DisableImages)
			m.current = emailView
			m.syncPluginStatus()
			m.syncPluginKeyBindings()
			m.updateCurrentWindowSize()
			return m, tea.Batch(append(m.pluginFlagCmds(), m.current.Init())...)
		}
		return m, nil

	case tui.OpenSplitFromFullscreenMsg:
		if ev, ok := m.current.(*tui.EmailView); ok && m.config.EnableSplitPane && m.folderInbox != nil {
			email := ev.GetEmail()
			m.folderInbox.OpenSplitPreview(email.UID, email.AccountID, &email)
			m.current = m.folderInbox
			m.updateCurrentWindowSize()
			markReadCmd := m.markEmailReadAndQueue(email.AccountID, email.UID, false)
			return m, tea.Batch(append(m.pluginFlagCmds(), markReadCmd, func() tea.Msg {
				return tui.UpdatePreviewMsg{UID: email.UID, AccountID: email.AccountID}
			})...)
		}
		return m, nil

	case tui.LanguageChangedMsg:
		m.afterLanguageChanged()
		return m, m.current.Init()

	case tui.GoToSettingsMsg:
		m.current = m.newSettings()
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.GoToAddAccountMsg:
		hideTips := false
		if m.config != nil {
			hideTips = m.config.HideTips
		}
		m.current = tui.NewLogin(hideTips)
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.GoToAddMailingListMsg:
		m.current = tui.NewMailingListEditor()
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.GoToEditAccountMsg:
		hideTips := false
		if m.config != nil {
			hideTips = m.config.HideTips
		}
		login := tui.NewLogin(hideTips)
		login.SetEditMode(msg.AccountID, msg.Protocol, msg.Provider, msg.Name, msg.Email, msg.FetchEmail, msg.SendAsEmail, msg.IMAPServer, msg.IMAPPort, msg.SMTPServer, msg.SMTPPort, msg.Insecure, msg.JMAPEndpoint, msg.POP3Server, msg.POP3Port, msg.CatchAll, msg.MaildirPath)
		if msg.AuthMethod != "" {
			login.SetAuthMethod(msg.AuthMethod)
		}
		m.current = login
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.GoToEditMailingListMsg:
		editor := tui.NewMailingListEditor()
		editor.SetEditMode(msg.Index, msg.Name, msg.Addresses)
		m.current = editor
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.GoToAddContactMsg:
		m.current = tui.NewContactEditor()
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.GoToEditContactMsg:
		editor := tui.NewContactEditor()
		editor.SetEditMode(msg.OriginalEmail, msg.Name, msg.Email)
		m.current = editor
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.SaveContactMsg:
		var contactErr error
		if msg.IsEdit {
			contactErr = config.UpdateContact(msg.OriginalEmail, msg.Name, msg.Email)
		} else {
			contactErr = config.AddContact(msg.Name, msg.Email)
		}
		s := m.newSettings()
		s.RestoreState(tui.SettingsState{
			ActivePane:     tui.PaneContent,
			ActiveCategory: tui.CategoryContacts,
		})
		m.current = s
		m.updateCurrentWindowSize()
		if contactErr != nil {
			return m, tea.Batch(m.current.Init(), m.showErrorCmd(fmt.Sprintf("Could not save contact: %v", contactErr)))
		}
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
				return m, m.showErrorCmd(fmt.Sprintf("Could not save mailing list: %v", err))
			}
		}
		s := m.newSettings()
		s.RestoreState(tui.SettingsState{
			ActivePane:     tui.PaneContent,
			ActiveCategory: tui.CategoryMailingLists,
		})
		m.current = s
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.GoToSignatureEditorMsg:
		m.current = tui.NewSignatureEditor(msg.AccountID)
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.PasswordVerifiedMsg:
		if msg.Err != nil {
			return m, nil
		}
		config.SetSessionKey(msg.Key)
		cfg, err := config.LoadConfig()
		if err == nil {
			if migrateErr := config.MigrateContactsCacheUsage(cfg.GetAccountIDs()); migrateErr != nil {
				log.Printf("warning: contacts migration failed: %v", migrateErr)
			}
			if cfg.Theme != "" {
				theme.SetTheme(cfg.Theme)
				tui.RebuildStyles()
			}
			lang := i18n.DetectLanguage(cfg)
			loglevel.Verbosef("Detected language: %s", lang)
			if err := i18n.GetManager().SetLanguage(lang); err != nil {
				log.Printf("Failed to set language %s: %v", lang, err)
			} else {
				loglevel.Verbosef("Language set to: %s", i18n.GetManager().GetLanguage())
				loglevel.Verbosef("Test translation: %s", i18n.GetManager().T("composer.title"))
			}
		}
		_ = config.EnsurePGPDir()
		if err != nil {
			m.config = nil
			m.current = tui.NewLogin(false)
		} else {
			m.config = cfg
			if m.mailtoURL != nil && len(cfg.Accounts) > 0 {
				to, subject, body := parseMailto(m.mailtoURL)
				composer := tui.NewComposerWithAccounts(cfg.Accounts, cfg.Accounts[0].ID, to, subject, body, cfg.HideTips)
				m.applySpellcheckOptions(composer)
				m.current = composer
			} else {
				m.current = tui.NewChoice()
			}
			m.updateSubordinates()
		}
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.SecureModeEnabledMsg:
		if msg.Err != nil {
			log.Printf("Failed to enable encryption: %v", msg.Err)
			return m, m.showErrorCmd(fmt.Sprintf("Failed to enable encryption: %v", msg.Err))
		}
		return m, nil

	case tui.SecureModeDisabledMsg:
		if msg.Err != nil {
			log.Printf("Failed to disable encryption: %v", msg.Err)
			return m, m.showErrorCmd(fmt.Sprintf("Failed to disable encryption: %v", msg.Err))
		}
		return m, nil

	case tui.GoToChoiceMenuMsg:
		return m, m.setChoiceMenu()

	case tui.MouseSupportChosenMsg:
		if m.config != nil {
			enabled := msg.Enabled
			m.config.MouseEnabled = &enabled
			config.MouseEnabled = &enabled
			if err := config.SaveConfig(m.config); err != nil {
				log.Printf("could not save mouse-enabled flag: %v", err)
				return m, m.showErrorCmd(fmt.Sprintf("Could not save settings: %v", err))
			}
		}
		return m, nil

	case tui.SetupGuideDoneMsg:
		if m.config != nil && !m.config.HasSeenSetupGuide {
			m.config.HasSeenSetupGuide = true
			if err := config.SaveConfig(m.config); err != nil {
				log.Printf("could not save setup-guide flag: %v", err)
				return m, tea.Batch(m.setChoiceMenu(), m.showErrorCmd(fmt.Sprintf("Could not save settings: %v", err)))
			}
		}
		return m, m.setChoiceMenu()

	case tui.DeleteAccountMsg:
		if err := m.deleteAccount(msg.AccountID); err != nil {
			return m, tea.Batch(m.current.Init(), m.showErrorCmd(fmt.Sprintf("Could not save settings: %v", err)))
		}
		return m, m.current.Init()

	case tui.ViewEmailMsg:
		email := msg.Email
		if email == nil {
			email = m.store.GetEmailByUIDAndAccount(msg.UID, msg.AccountID)
		} else {
			m.store.AddEmailToStoresIfMissing(*email, msg.Mailbox)
		}
		if email == nil {
			return m, nil
		}
		folderName := m.folderName()
		suppressRead := false
		if m.plugins != nil {
			t := m.plugins.EmailToTable(email.UID, email.From, email.To, email.Subject, email.Date, email.IsRead, email.AccountID, folderName)
			m.plugins.CallHook(plugin.HookEmailViewed, t)
			suppressRead = m.plugins.TakeAutoReadSuppressed()
		}
		if m.config.EnableSplitPane && m.folderInbox != nil {
			m.folderInbox.OpenSplitPreview(msg.UID, msg.AccountID, email)
			m.current = m.folderInbox
			cmd = m.markEmailReadAndQueue(msg.AccountID, msg.UID, suppressRead)
			return m, tea.Batch(append(m.pluginFlagCmds(), cmd, func() tea.Msg {
				return tui.UpdatePreviewMsg{UID: msg.UID, AccountID: msg.AccountID}
			})...)
		}
		if cached := config.GetCachedEmailBody(folderName, msg.UID, msg.AccountID, m.config.GetBodyCacheThreshold()); cached != nil {
			attachments := m.cachedAttachmentsToFetcher(cached.Attachments)
			return m, func() tea.Msg {
				return tui.EmailBodyFetchedMsg{
					UID:          msg.UID,
					Body:         cached.Body,
					BodyMIMEType: cached.BodyMIMEType,
					Attachments:  attachments,
					AccountID:    msg.AccountID,
					Mailbox:      msg.Mailbox,
					Folder:       folderName,
				}
			}
		}
		m.current = tui.NewStatus("Fetching email content...")
		return m, tea.Batch(append(m.pluginFlagCmds(), m.current.Init(), fetchcmd.FetchFolderEmailBodyCmd(m.config, msg.UID, msg.AccountID, folderName, msg.Mailbox), m.pluginNotifyCmd())...)

	case tui.FetchErr:
		log.Printf("paginated fetch error: %v", error(msg))
		return m, m.showErrorCmd(error(msg).Error())

	case tui.EmailBodyFetchedMsg:
		if msg.Err != nil {
			log.Printf("could not fetch email body: %v", msg.Err)
			m.pendingBodyFetchUID = 0
			m.pendingBodyFetchCount = 0
			if m.folderInbox != nil {
				m.current = m.folderInbox
			}
			return m, m.showErrorCmd(msg.Err.Error())
		}
		bodyFolder := msg.Folder
		if bodyFolder == "" {
			bodyFolder = m.folderName()
		}
		// If body is empty or whitespace-only, retry the fetch instead of showing incomplete content
		if strings.TrimSpace(msg.Body) == "" {
			if m.pendingBodyFetchUID == msg.UID && m.pendingBodyFetchCount >= maxBodyFetchRetries {
				log.Printf("email body still empty after %d retries for UID %d, showing error", maxBodyFetchRetries, msg.UID)
				m.pendingBodyFetchUID = 0
				m.pendingBodyFetchCount = 0
				if m.folderInbox != nil {
					m.current = m.folderInbox
				}
				return m, m.showErrorCmd("Email body could not be loaded after multiple attempts")
			}
			m.pendingBodyFetchUID = msg.UID
			m.pendingBodyFetchCount++
			log.Printf("email body empty for UID %d, retrying fetch (%d/%d)", msg.UID, m.pendingBodyFetchCount, maxBodyFetchRetries)
			m.current = tui.NewStatus("Fetching email content...")
			return m, tea.Batch(
				append(m.pluginFlagCmds(), m.current.Init(), fetchcmd.FetchFolderEmailBodyCmd(m.config, msg.UID, msg.AccountID, bodyFolder, msg.Mailbox), m.pluginNotifyCmd())...,
			)
		}
		// Reset retry counter on successful fetch
		m.pendingBodyFetchUID = 0
		m.pendingBodyFetchCount = 0
		m.store.UpdateEmailBodyByUID(msg.UID, msg.AccountID, msg.Body, msg.BodyMIMEType, msg.Attachments)
		m.cacheEmailBody(bodyFolder, msg.UID, msg.AccountID, msg.Body, msg.BodyMIMEType, msg.Attachments)
		email := m.store.GetEmailByUIDAndAccount(msg.UID, msg.AccountID)
		if email == nil {
			if m.folderInbox != nil {
				m.current = m.folderInbox
			}
			return m, nil
		}
		var markReadCmd tea.Cmd
		pluginSuppressed := m.plugins != nil && m.plugins.TakeAutoReadSuppressed()
		if !email.IsRead && !pluginSuppressed {
			m.store.MarkEmailAsReadInStores(msg.UID, msg.AccountID)
			account := m.config.GetAccountByID(msg.AccountID)
			if account != nil {
				markReadCmd = fetchcmd.MarkEmailAsReadCmd(account, msg.UID, msg.AccountID, bodyFolder)
			}
		}
		emailIndex := m.store.GetEmailIndex(msg.UID, msg.AccountID)
		sz := m.currentWindowSize()
		emailView := tui.NewEmailView(*email, emailIndex, sz.Width, sz.Height, msg.Mailbox, m.config.DisableImages)
		m.current = emailView
		m.syncPluginStatus()
		m.syncPluginKeyBindings()
		cmds := []tea.Cmd{m.current.Init()}
		if markReadCmd != nil {
			cmds = append(cmds, markReadCmd)
		}
		cmds = append(cmds, m.pluginFlagCmds()...)
		if yubiCmd := m.maybeYubiKeyNotification(msg.AccountID, msg.Attachments); yubiCmd != nil {
			cmds = append(cmds, yubiCmd)
		}
		return m, tea.Batch(cmds...)

	case tui.ReplyToEmailMsg:
		var to string
		if len(msg.Email.ReplyTo) > 0 {
			to = strings.Join(msg.Email.ReplyTo, ", ")
		} else {
			to = msg.Email.From
		}
		subject := msg.Email.Subject
		normalizedSubject := strings.ToLower(strings.TrimSpace(subject))
		if !strings.HasPrefix(normalizedSubject, "re:") {
			subject = "Re: " + subject
		}
		quotedText := fmt.Sprintf("\n\nOn %s, %s wrote:\n> %s", msg.Email.Date.Local().Format("Jan 2, 2006 at 3:04 PM"), msg.Email.From, strings.ReplaceAll(msg.Email.Body, "\n", "\n> "))
		var composer *tui.Composer
		hideTips := false
		if m.config != nil {
			hideTips = m.config.HideTips
		}
		if m.config != nil && len(m.config.Accounts) > 0 {
			accountID := msg.Email.AccountID
			if accountID == "" {
				accountID = m.config.GetFirstAccount().ID
			}
			composer = tui.NewComposerWithAccounts(m.config.Accounts, accountID, to, subject, "", hideTips)
			if len(msg.Email.To) > 0 {
				for i := range m.config.Accounts {
					if m.config.Accounts[i].ID == accountID && m.config.Accounts[i].CatchAll {
						acc := &m.config.Accounts[i]
						deliveryAddr := msg.Email.To[0]
						if addr, err := mail.ParseAddress(deliveryAddr); err == nil {
							deliveryAddr = addr.Address
						}
						fromVal := deliveryAddr
						if acc.Name != "" {
							fromVal = fmt.Sprintf("%s <%s>", acc.Name, deliveryAddr)
						}
						composer.SetFromOverride(fromVal)
						break
					}
				}
			}
		} else {
			composer = tui.NewComposer("", to, subject, "", hideTips)
		}
		if m.config != nil {
			composer.SetShowCcBcc(m.config.ShowCcBccByDefault)
		}
		composer.SetQuotedText(quotedText)
		inReplyTo := msg.Email.MessageID
		references := append(msg.Email.References, msg.Email.MessageID) //nolint:gocritic
		composer.SetReplyContext(inReplyTo, references)
		m.applySpellcheckOptions(composer)
		if m.config != nil && m.config.ShowOriginalOnReply {
			sz := m.currentWindowSize()
			replySplit := tui.NewReplySplitView(
				msg.Email,
				composer,
				m.config.GetSplitPaneOrientation(),
				m.config.DisableImages,
				sz.Width,
				sz.Height,
			)
			m.current = replySplit
		} else {
			m.current = composer
		}
		m.updateCurrentWindowSize()
		m.syncPluginKeyBindings()
		return m, m.current.Init()

	case tui.ReplyAllEmailMsg:
		// Determine the original sender (Reply-To if present, else From)
		var sender string
		if len(msg.Email.ReplyTo) > 0 {
			sender = strings.Join(msg.Email.ReplyTo, ", ")
		} else {
			sender = msg.Email.From
		}
		subject := msg.Email.Subject
		normalizedSubject := strings.ToLower(strings.TrimSpace(subject))
		if !strings.HasPrefix(normalizedSubject, "re:") {
			subject = "Re: " + subject
		}
		quotedText := fmt.Sprintf("\n\nOn %s, %s wrote:\n> %s", msg.Email.Date.Local().Format("Jan 2, 2006 at 3:04 PM"), msg.Email.From, strings.ReplaceAll(msg.Email.Body, "\n", "\n> "))

		// Build set of the user's own addresses to exclude from reply-all
		selfAddrs := make(map[string]bool)
		if m.config != nil {
			for _, acc := range m.config.Accounts {
				selfAddrs[strings.ToLower(acc.Email)] = true
				selfAddrs[strings.ToLower(acc.GetFetchEmail())] = true
				selfAddrs[strings.ToLower(acc.GetSendAsEmail())] = true
			}
			// For catch-all accounts, also exclude the delivery address
			accountID := msg.Email.AccountID
			if accountID == "" && len(m.config.Accounts) > 0 {
				accountID = m.config.GetFirstAccount().ID
			}
			for _, acc := range m.config.Accounts {
				if acc.ID == accountID && acc.CatchAll && len(msg.Email.To) > 0 {
					if addr, err := mail.ParseAddress(msg.Email.To[0]); err == nil {
						selfAddrs[strings.ToLower(addr.Address)] = true
					} else {
						selfAddrs[strings.ToLower(strings.TrimSpace(msg.Email.To[0]))] = true
					}
					break
				}
			}
		}

		// Collect Cc recipients from original To list, excluding self and the sender
		seen := make(map[string]bool)
		var ccRecipients []string
		for _, addr := range msg.Email.To {
			parsed, err := mail.ParseAddress(addr)
			var addrLower string
			displayAddr := addr
			if err == nil {
				addrLower = strings.ToLower(parsed.Address)
				displayAddr = parsed.Address
			} else {
				addrLower = strings.ToLower(strings.TrimSpace(addr))
			}
			if selfAddrs[addrLower] || seen[addrLower] {
				continue
			}
			seen[addrLower] = true
			ccRecipients = append(ccRecipients, displayAddr)
		}
		cc := strings.Join(ccRecipients, ", ")

		var composer *tui.Composer
		hideTips := false
		if m.config != nil {
			hideTips = m.config.HideTips
		}
		if m.config != nil && len(m.config.Accounts) > 0 {
			accountID := msg.Email.AccountID
			if accountID == "" {
				accountID = m.config.GetFirstAccount().ID
			}
			composer = tui.NewComposerWithAccounts(m.config.Accounts, accountID, sender, subject, "", hideTips)
			// For catch-all accounts, pre-fill From with the specific address the email was delivered to.
			if len(msg.Email.To) > 0 {
				for i := range m.config.Accounts {
					if m.config.Accounts[i].ID == accountID && m.config.Accounts[i].CatchAll {
						acc := &m.config.Accounts[i]
						deliveryAddr := msg.Email.To[0]
						if addr, err := mail.ParseAddress(deliveryAddr); err == nil {
							deliveryAddr = addr.Address
						}
						fromVal := deliveryAddr
						if acc.Name != "" {
							fromVal = fmt.Sprintf("%s <%s>", acc.Name, deliveryAddr)
						}
						composer.SetFromOverride(fromVal)
						break
					}
				}
			}
		} else {
			composer = tui.NewComposer("", sender, subject, "", hideTips)
		}
		if m.config != nil {
			composer.SetShowCcBcc(m.config.ShowCcBccByDefault)
		}
		if cc != "" {
			composer.SetCc(cc)
		}
		composer.SetQuotedText(quotedText)

		// Set reply headers
		inReplyTo := msg.Email.MessageID
		references := append(msg.Email.References, msg.Email.MessageID) //nolint:gocritic
		composer.SetReplyContext(inReplyTo, references)

		m.applySpellcheckOptions(composer)
		if m.config != nil && m.config.ShowOriginalOnReply {
			sz := m.currentWindowSize()
			replySplit := tui.NewReplySplitView(
				msg.Email,
				composer,
				m.config.GetSplitPaneOrientation(),
				m.config.DisableImages,
				sz.Width,
				sz.Height,
			)
			m.current = replySplit
		} else {
			m.current = composer
		}
		m.updateCurrentWindowSize()
		m.syncPluginKeyBindings()
		return m, m.current.Init()

	case tui.ForwardEmailMsg:
		subject := msg.Email.Subject
		if !strings.HasPrefix(strings.ToLower(subject), "fwd:") {
			subject = "Fwd: " + subject
		}
		forwardHeader := fmt.Sprintf("\n\n---------- Forwarded message ----------\nFrom: %s\nDate: %s\nSubject: %s\nTo: %s\n\n",
			msg.Email.From,
			msg.Email.Date.Local().Format("Mon, Jan 2, 2006 at 3:04 PM"),
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
			accountID := msg.Email.AccountID
			if accountID == "" {
				accountID = m.config.GetFirstAccount().ID
			}
			composer = tui.NewComposerWithAccounts(m.config.Accounts, accountID, "", subject, body, hideTips)
		} else {
			composer = tui.NewComposer("", "", subject, body, hideTips)
		}
		if m.config != nil {
			composer.SetShowCcBcc(m.config.ShowCcBccByDefault)
		}
		m.applySpellcheckOptions(composer)
		m.current = composer
		m.updateCurrentWindowSize()
		m.syncPluginKeyBindings()
		return m, m.current.Init()

	case tui.OpenEditorMsg:
		composer := getCurrentComposer(m.current)
		if composer == nil {
			return m, nil
		}
		return m, editor.OpenExternalEditor(composer.GetBody())

	case tui.EditorFinishedMsg:
		if msg.Err != nil {
			log.Printf("Editor error: %v", msg.Err)
			return m, m.showErrorCmd(fmt.Sprintf("External editor failed: %v", msg.Err))
		}
		if composer := getCurrentComposer(m.current); composer != nil {
			composer.SetBody(msg.Body)
		}
		return m, nil

	case tui.GoToFilePickerMsg:
		if runtime.GOOS == "darwin" {
			return m, func() tea.Msg {
				wd, _ := os.Getwd()
				paths, err := macos.OpenFilePicker(wd)
				if err != nil || len(paths) == 0 {
					return tui.CancelFilePickerMsg{}
				}
				return tui.FileSelectedMsg{Paths: paths}
			}
		}
		m.previousModel = m.current
		wd, _ := os.Getwd()
		m.current = tui.NewFilePicker(wd)
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.FileSelectedMsg:
		m.pendingExport = nil
		m.restoreView()
		// Forward the selected paths to the restored composer so its
		// FileSelectedMsg handler can append them to attachmentPaths.
		// Without this, attachments selected in the picker are dropped.
		if m.current != nil {
			newModel, cmd := m.current.Update(msg)
			m.current = newModel
			return m, cmd
		}
		return m, nil

	case tui.CancelFilePickerMsg:
		m.pendingExport = nil
		m.restoreView()
		return m, nil

	case tui.OpenPaletteMsg:
		return m, m.palette.Open(msg.Commands, m.width, m.contentHeight())

	case tui.GoToSaveFilePickerMsg:
		if runtime.GOOS == "darwin" {
			return m, func() tea.Msg {
				home, _ := os.UserHomeDir()
				savePath, err := macos.SaveFilePicker(home, msg.SuggestedName)
				if err != nil || savePath == "" {
					return tui.CancelFilePickerMsg{}
				}
				return tui.ExportEmailMsg{
					Email:    msg.Email,
					Account:  msg.Account,
					Folder:   msg.Folder,
					Mailbox:  msg.Mailbox,
					Format:   msg.Format,
					SavePath: savePath,
				}
			}
		}
		m.previousModel = m.current
		home, _ := os.UserHomeDir()
		m.current = tui.NewSaveFilePicker(home, msg.SuggestedName)
		m.updateCurrentWindowSize()
		m.pendingExport = &emailstore.PendingExport{
			Email:   msg.Email,
			Account: msg.Account,
			Folder:  msg.Folder,
			Mailbox: msg.Mailbox,
			Format:  msg.Format,
		}
		return m, m.current.Init()

	case tui.SaveFileSelectedMsg:
		pe := m.pendingExport
		m.pendingExport = nil
		m.restoreView()
		if pe == nil {
			return m, nil
		}
		return m, exportcmd.ExportEmailCmd(m.config, pe.Email, pe.Account, pe.Folder, pe.Format, msg.Path)

	case tui.ExportEmailMsg:
		return m, exportcmd.ExportEmailCmd(m.config, msg.Email, msg.Account, msg.Folder, msg.Format, msg.SavePath)

	case tui.EmailExportedMsg:
		if msg.Err != nil {
			return m, m.showErrorCmd(fmt.Sprintf("Export failed: %v", msg.Err))
		}
		return m, m.showInfoCmd(fmt.Sprintf("Exported to %s", msg.Path))

	case tui.OpenEmailInBrowserMsg:
		return m, exportcmd.OpenEmailInBrowserCmd(m.config, msg.Email, msg.Account, msg.Folder)

	case tui.EmailOpenedInBrowserMsg:
		if msg.Err != nil {
			return m, m.showErrorCmd(fmt.Sprintf("Failed to open in browser: %v", msg.Err))
		}
		return m, m.showInfoCmd("Opened in browser")

	case tui.SendEmailMsg:
		if m.plugins != nil {
			m.plugins.CallSendHook(plugin.HookEmailSendBefore, msg.To, msg.Cc, msg.Subject, msg.AccountID)
		}
		m.previousModel = m.current
		var draftID string
		if composer := getCurrentComposer(m.current); composer != nil {
			draftID = composer.GetDraftID()
		}
		if m.service == nil && m.config != nil {
			m.service = daemonclient.NewService(m.config)
			m.updateSubordinates()
		}
		m.sendNotice = m.sendEmailNotice(msg)
		m.pendingDraftID = draftID
		m.current = tui.NewChoice()
		m.updateCurrentWindowSize()
		go m.saveContactsAfterSend(msg)
		return m, tea.Batch(m.current.Init(), send.SendEmailCmd(m.sendEmailDependencies(), msg))

	case tui.EmailQueuedMsg:
		m.pendingJobID = msg.JobID
		m.sendNotice = fmt.Sprintf("Message sent (%s to undo)", config.Keybinds.Composer.UndoSend)
		return m, tea.Tick(
			time.Duration(msg.DelaySeconds)*time.Second, func(t time.Time) tea.Msg {
				return tui.EmailDelayExpiredMsg{JobID: msg.JobID}
			})

	case tui.EmailDelayExpiredMsg:
		if m.pendingJobID == msg.JobID {
			m.pendingJobID = ""
			m.sendNotice = ""
			m.previousModel = nil
			m.finishDraftAfterSend()
			if m.plugins != nil {
				m.plugins.CallHook(plugin.HookEmailSendAfter)
			}
			// Stay wherever the user went while the undo window was open.
			return m, nil
		}
		return m, nil

	case tui.UndoSendMsg:
		m.sendNotice = ""
		m.pendingDraftID = ""
		if m.previousModel != nil {
			m.current = m.previousModel
			m.previousModel = nil
			m.updateCurrentWindowSize()
			return m, m.current.Init()
		}
		return m, m.setChoiceMenu()

	case tui.ActionGracePeriodExpiredMsg:
		return m, m.actionManager.OnGracePeriodExpired(msg)

	case clearErrorNotifMsg:
		if msg.gen == m.errorNotifGen {
			m.showErrorNotif = false
		}
		return m, nil

	case clearPluginNotifMsg:
		if msg.gen == m.pluginNotifGen {
			m.showPluginNotif = false
		}
		return m, nil

	case tui.InfoNotifyMsg:
		dur := time.Duration(msg.Duration * float64(time.Second))
		if dur <= 0 {
			dur = 2 * time.Second
		}
		m.errorNotification = overlay.NewInfo(m.notificationOptions(
			overlay.WithMessage(msg.Message),
			overlay.WithKey(config.Keybinds.Global.DismissNotification),
		)...)
		return m, m.armErrorNotif(dur)

	case tui.NotifyMsg:
		return m, m.showErrorCmd(msg.Message)

	case tui.SendRSVPMsg:
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			return m, m.showErrorCmd("account not found")
		}
		return m, tea.Batch(m.showInfoCmd("Sending RSVP…"), send.SendRSVP(account, msg))

	case tui.RSVPResultMsg:
		if msg.Err != nil {
			log.Printf("Failed to send RSVP: %v", msg.Err)
			return m, m.showErrorCmd(fmt.Sprintf("Failed to send RSVP: %v", msg.Err))
		}
		status := fmt.Sprintf("RSVP sent: %s", msg.Response)
		if strings.HasSuffix(strings.ToLower(msg.Organizer), "@gmail.com") || strings.HasSuffix(strings.ToLower(msg.Organizer), "@googlemail.com") {
			status += " (Google Calendar may not auto-update — use Gmail buttons for Google events)"
		}
		return m, m.showInfoCmd(status)

	case tui.EmailResultMsg:
		m.sendNotice = ""
		if msg.Err != nil {
			log.Printf("Failed to send email: %v", msg.Err)
			// Bring the message back so nothing typed is lost.
			_, wasPatchForm := m.previousModel.(*tui.PatchSend)
			if getCurrentComposer(m.previousModel) != nil || wasPatchForm {
				m.current = m.previousModel
				m.previousModel = nil
				m.pendingDraftID = ""
				m.updateCurrentWindowSize()
				return m, m.showErrorCmd(fmt.Sprintf("Failed to send: %v", msg.Err))
			}
			m.previousModel = nil
			m.current = tui.NewChoice()
			m.updateCurrentWindowSize()
			return m, tea.Batch(m.current.Init(), m.showErrorCmd(msg.Err.Error()))
		}
		if m.pendingJobID == "" {
			// Sent directly (no undo window): the draft is no longer needed.
			m.previousModel = nil
			m.finishDraftAfterSend()
		}
		if msg.Warning != "" {
			log.Printf("Email send warning: %s", msg.Warning)
			m.current = tui.NewChoice()
			m.updateCurrentWindowSize()
			return m, tea.Batch(m.current.Init(), m.showInfoCmd(msg.Warning))
		}
		if m.plugins != nil {
			m.plugins.CallHook(plugin.HookEmailSendAfter)
		}
		if _, onMenu := m.current.(*tui.Choice); onMenu {
			return m, nil
		}
		return m, m.setChoiceMenu()

	case tui.ApplyPatchMsg:
		// Apply patch to current directory by default; user can change via config.
		repoDir := "."
		if cwd, err := os.Getwd(); err == nil {
			repoDir = cwd
		}
		return m, send.ApplyPatchCmd(repoDir, msg)

	case tui.PatchStagedMsg:
		// Patch has been applied to disk and staged. Run git commit via
		// tea.ExecProcess so the terminal is released for GPG pinentry.
		repoDir := "."
		if cwd, err := os.Getwd(); err == nil {
			repoDir = cwd
		}
		return m, send.CommitPatchCmd(repoDir, msg)

	case tui.PatchApplyResultMsg:
		if msg.Err != nil {
			errMsg := fmt.Sprintf("Patch apply failed: %v", msg.Err)
			return m, tea.Batch(m.showErrorCmd(errMsg))
		}
		verb := "applied"
		if msg.DryRun {
			verb = "validated"
		}
		status := fmt.Sprintf("Patch %s: %s", verb, msg.Subject)
		if len(msg.Files) > 0 {
			status += fmt.Sprintf(" (%d files)", len(msg.Files))
		}
		if len(msg.Warnings) > 0 {
			status += "\n" + strings.Join(msg.Warnings, "\n")
		}
		return m, tea.Batch(m.showInfoCmd(status))

	case tui.GoToSendPatchMsg:
		if m.config == nil || len(m.config.Accounts) == 0 {
			return m, m.showErrorCmd("no accounts configured")
		}
		m.previousModel = m.current
		m.current = tui.NewPatchSend(m.config.Accounts)
		m.updateCurrentWindowSize()
		return m, m.current.Init()

	case tui.SendPatchMsg:
		m.previousModel = m.current
		m.current = tui.NewStatus("Generating and sending patch...")
		return m, tea.Batch(m.current.Init(), send.SendPatchCmd(m.sendEmailDependencies(), msg))

	case tui.PatchGeneratedMsg:
		if msg.Err != nil {
			errMsg := fmt.Sprintf("Patch generation failed: %v", msg.Err)
			if m.previousModel != nil {
				m.current = m.previousModel
				m.previousModel = nil
				m.updateCurrentWindowSize()
			}
			return m, tea.Batch(m.current.Init(), m.showErrorCmd(errMsg))
		}
		// Patch generated — now send the raw patch email via SMTP.
		// We need the original SendPatchMsg (with To/Cc) to inject the
		// recipients into the raw RFC 5322 message. The patch-send form
		// stores it in the PatchGeneratedMsg.
		return m, send.SendRawPatchCmd(m.sendEmailDependencies(), msg.SendPatchMsg, msg.RawPatch)

	case tui.DeleteEmailMsg:
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			if m.folderInbox != nil {
				m.current = m.folderInbox
			}
			return m, nil
		}
		folderName := m.folderName()
		if m.folderInbox != nil {
			m.current = m.folderInbox
			m.folderInbox.GetInbox().RemoveEmail(msg.UID, msg.AccountID)
		}
		flushCmd := m.batchActionCmd()
		pa, notice := m.actionManager.HandleDeleteEmailMsg(msg, folderName)
		return m, tea.Batch(flushCmd, m.actionManager.StartActionGracePeriod(pa, notice))

	case tui.ArchiveEmailMsg:
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			if m.folderInbox != nil {
				m.current = m.folderInbox
			}
			return m, nil
		}
		folderName := m.folderName()
		if m.folderInbox != nil {
			m.current = m.folderInbox
			m.folderInbox.GetInbox().RemoveEmail(msg.UID, msg.AccountID)
		}
		flushCmd := m.batchActionCmd()
		pa, notice := m.actionManager.HandleArchiveEmailMsg(msg, folderName)
		return m, tea.Batch(flushCmd, m.actionManager.StartActionGracePeriod(pa, notice))

	case tui.EmailMarkedReadMsg:
		if msg.Err != nil {
			log.Printf("Error marking email as read: %v", msg.Err)
		}
		m.syncUnreadBadge()
		return m, nil

	case tui.EmailMarkedUnreadMsg:
		if msg.Err != nil {
			log.Printf("Error marking email as unread: %v", msg.Err)
		}
		m.syncUnreadBadge()
		return m, nil

	case tui.EmailActionDoneMsg:
		if msg.Err != nil {
			log.Printf("Action failed: %v", msg.Err)
			return m, m.showErrorCmd(msg.Err.Error())
		}
		return m, nil

	case tui.BatchDeleteEmailsMsg:
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			if m.folderInbox != nil {
				m.current = m.folderInbox
			}
			return m, nil
		}
		folderName := m.folderName()
		if m.folderInbox != nil {
			m.folderInbox.GetInbox().RemoveEmails(msg.UIDs, msg.AccountID)
		}
		flushCmd := m.batchActionCmd()
		pa, notice := m.actionManager.HandleBatchDeleteEmailsMsg(msg, folderName)
		return m, tea.Batch(flushCmd, m.actionManager.StartActionGracePeriod(pa, notice))

	case tui.BatchArchiveEmailsMsg:
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			if m.folderInbox != nil {
				m.current = m.folderInbox
			}
			return m, nil
		}
		folderName := m.folderName()
		if m.folderInbox != nil {
			m.folderInbox.GetInbox().RemoveEmails(msg.UIDs, msg.AccountID)
		}
		flushCmd := m.batchActionCmd()
		pa, notice := m.actionManager.HandleBatchArchiveEmailsMsg(msg, folderName)
		return m, tea.Batch(flushCmd, m.actionManager.StartActionGracePeriod(pa, notice))

	case tui.BatchMoveEmailsMsg:
		if m.config == nil {
			return m, nil
		}
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			return m, nil
		}
		folderName := m.folderName()
		if m.folderInbox != nil {
			m.folderInbox.GetInbox().RemoveEmails(msg.UIDs, msg.AccountID)
		}
		flushCmd := m.batchActionCmd()
		pa, notice := m.actionManager.HandleBatchMoveEmailsMsg(msg, folderName)
		return m, tea.Batch(flushCmd, m.actionManager.StartActionGracePeriod(pa, notice))

	case tui.BatchEmailActionDoneMsg:
		if msg.Err != nil {
			log.Printf("Batch %s failed: %v", msg.Action, msg.Err)
			return m, m.showErrorCmd(msg.Err.Error())
		}
		return m, nil

	case tui.DownloadAttachmentMsg:
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			return m, m.showErrorCmd("Account not found for this email")
		}
		var email *fetcher.Email
		if msg.UID != 0 {
			email = m.store.GetEmailByUIDAndAccount(msg.UID, msg.AccountID)
		} else {
			email = m.store.GetEmailByIndex(msg.Index) // legacy: the index can be stale
		}
		if email == nil {
			return m, m.showErrorCmd("Email not found; refresh and try again")
		}
		newMsg := msg
		newMsg.UID = email.UID
		if newMsg.Folder == "" {
			newMsg.Folder = m.folderName()
		}
		if newMsg.Encoding == "" {
			for _, att := range email.Attachments {
				if att.PartID == msg.PartID {
					newMsg.Encoding = att.Encoding
					break
				}
			}
		}
		return m, tea.Batch(
			m.showInfoCmd(fmt.Sprintf("Downloading %s…", msg.Filename)),
			exportcmd.DownloadAttachmentCmd(account, email.UID, newMsg),
		)

	case tui.AttachmentDownloadedMsg:
		if msg.Err != nil {
			return m, m.showErrorCmd(fmt.Sprintf("Error downloading: %v", msg.Err))
		}
		return m, m.showInfoCmd(fmt.Sprintf("Saved to %s", msg.Path))

	case tui.RestoreViewMsg:
		m.restoreView()
		return m, nil
	}

	if cmd := m.pluginNotifyCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// View renders the current TUI view with overlays.
func (m *Model) View() tea.View {
	v := m.current.View()
	if m.showLogPanel {
		v.Content = m.renderWithLogPanel(v.Content)
	}
	if m.palette.IsOpen() && m.palette.CommandPalette != nil {
		v.Content = m.palette.Render(v.Content, m.width, m.contentHeight())
	}
	if m.sendNotice != "" {
		v.Content = m.renderSendNoticeOverlay(v.Content)
	}
	if m.actionManager.ActionNotice != "" {
		v.Content = m.renderActionNoticeOverlay(v.Content)
	}
	if m.showErrorNotif {
		v.Content = m.errorNotification.PlaceOn(v.Content)
	}
	if m.showPluginNotif {
		v.Content = m.pluginNotification.PlaceOn(v.Content)
	}
	v.AltScreen = true
	// Mouse reporting is a global setting; applying it here keeps every
	// screen consistent (individual views used to forget it).
	if config.MouseEnabled != nil && *config.MouseEnabled {
		v.MouseMode = tea.MouseModeCellMotion
	} else {
		v.MouseMode = tea.MouseModeNone
	}
	return v
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
