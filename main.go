package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/sender"
	"github.com/floatpane/matcha/tui"
	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer/html"
)

const (
	initialEmailLimit = 20
	paginationLimit   = 20
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
	emails        []fetcher.Email
	emailsByAcct  map[string][]fetcher.Email
	sentEmails    []fetcher.Email
	sentByAcct    map[string][]fetcher.Email
	trashEmails   []fetcher.Email
	trashByAcct   map[string][]fetcher.Email
	archiveEmails []fetcher.Email
	archiveByAcct map[string][]fetcher.Email
	inbox         *tui.Inbox
	sentInbox     *tui.Inbox
	trashArchive  *tui.TrashArchive
	width         int
	height        int
	err           error
}

func newInitialModel(cfg *config.Config) *mainModel {
	initialModel := &mainModel{
		emailsByAcct:  make(map[string][]fetcher.Email),
		sentByAcct:    make(map[string][]fetcher.Email),
		trashByAcct:   make(map[string][]fetcher.Email),
		archiveByAcct: make(map[string][]fetcher.Email),
	}

	if cfg == nil || !cfg.HasAccounts() {
		initialModel.current = tui.NewLogin()
	} else {
		initialModel.current = tui.NewChoice()
		initialModel.config = cfg
	}
	return initialModel
}

func (m *mainModel) Init() tea.Cmd {
	return tea.Batch(m.current.Init(), checkForUpdatesCmd())
}

func (m *mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	m.current, cmd = m.current.Update(msg)
	cmds = append(cmds, cmd)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if msg.String() == "esc" {
			switch m.current.(type) {
			case *tui.FilePicker:
				return m, func() tea.Msg { return tui.CancelFilePickerMsg{} }
			case *tui.Inbox, *tui.Login, *tui.TrashArchive:
				m.current = tui.NewChoice()
				return m, m.current.Init()
			}
		}

	case tui.BackToInboxMsg:
		if m.inbox != nil {
			m.current = m.inbox
		} else {
			m.current = tui.NewChoice()
		}
		return m, nil

	case tui.BackToMailboxMsg:
		switch msg.Mailbox {
		case tui.MailboxSent:
			if m.sentInbox != nil {
				m.current = m.sentInbox
				return m, nil
			}
		case tui.MailboxInbox:
			if m.inbox != nil {
				m.current = m.inbox
				return m, nil
			}
		case tui.MailboxTrash, tui.MailboxArchive:
			if m.trashArchive != nil {
				m.current = m.trashArchive
				return m, nil
			}
		}
		m.current = tui.NewChoice()
		return m, nil

	case tui.DiscardDraftMsg:
		// Save draft to disk
		if msg.ComposerState != nil {
			draft := msg.ComposerState.ToDraft()
			go func() {
				if err := config.SaveDraft(draft); err != nil {
					log.Printf("Error saving draft: %v", err)
				}
			}()
		}
		m.current = tui.NewChoice()
		return m, m.current.Init()

	case tui.Credentials:
		// Add new account or update existing
		account := config.Account{
			ID:              uuid.New().String(),
			Name:            msg.Name,
			Email:           msg.Host, // login/email used for authentication comes from Host field in the form
			Password:        msg.Password,
			ServiceProvider: msg.Provider,
			FetchEmail:      msg.FetchEmail,
		}

		if msg.Provider == "custom" {
			account.IMAPServer = msg.IMAPServer
			account.IMAPPort = msg.IMAPPort
			account.SMTPServer = msg.SMTPServer
			account.SMTPPort = msg.SMTPPort
		}

		// Ensure FetchEmail defaults to the login Email (Host) if not explicitly set
		if account.FetchEmail == "" && account.Email != "" {
			account.FetchEmail = account.Email
		}

		if m.config == nil {
			m.config = &config.Config{}
		}

		// Check if we're editing an existing account
		if login, ok := m.current.(*tui.Login); ok && login.IsEditMode() {
			// Find and update the existing account
			existingID := login.GetAccountID()
			for i, acc := range m.config.Accounts {
				if acc.ID == existingID {
					account.ID = existingID
					m.config.Accounts[i] = account
					break
				}
			}
		} else {
			m.config.AddAccount(account)
		}

		if err := config.SaveConfig(m.config); err != nil {
			log.Printf("could not save config: %v", err)
			return m, tea.Quit
		}

		m.current = tui.NewChoice()
		return m, m.current.Init()

	case tui.GoToInboxMsg:
		if m.config == nil || !m.config.HasAccounts() {
			m.current = tui.NewLogin()
			return m, m.current.Init()
		}
		// Try to load from cache first for instant display
		if config.HasEmailCache() {
			return m, loadCachedEmails()
		}
		// No cache, fetch normally
		m.current = tui.NewStatus("Fetching emails from all accounts...")
		return m, tea.Batch(m.current.Init(), fetchAllAccountsEmails(m.config, tui.MailboxInbox))

	case tui.GoToSentInboxMsg:
		if m.config == nil || !m.config.HasAccounts() {
			m.current = tui.NewLogin()
			return m, m.current.Init()
		}
		m.current = tui.NewStatus("Fetching sent emails from all accounts...")
		return m, tea.Batch(m.current.Init(), fetchAllAccountsEmails(m.config, tui.MailboxSent))

	case tui.GoToTrashArchiveMsg:
		if m.config == nil || !m.config.HasAccounts() {
			m.current = tui.NewLogin()
			return m, m.current.Init()
		}
		m.current = tui.NewStatus("Fetching trash and archive emails...")
		return m, tea.Batch(
			m.current.Init(),
			fetchAllAccountsEmails(m.config, tui.MailboxTrash),
			fetchAllAccountsEmails(m.config, tui.MailboxArchive),
		)

	case tui.CachedEmailsLoadedMsg:
		if msg.Cache == nil {
			// Cache load failed, fetch normally
			m.current = tui.NewStatus("Fetching emails from all accounts...")
			return m, tea.Batch(m.current.Init(), fetchAllAccountsEmails(m.config, tui.MailboxInbox))
		}

		// Convert cached emails to fetcher.Email
		var cachedEmails []fetcher.Email
		emailsByAcct := make(map[string][]fetcher.Email)
		for _, cached := range msg.Cache.Emails {
			email := fetcher.Email{
				UID:       cached.UID,
				From:      cached.From,
				To:        cached.To,
				Subject:   cached.Subject,
				Date:      cached.Date,
				MessageID: cached.MessageID,
				AccountID: cached.AccountID,
			}
			cachedEmails = append(cachedEmails, email)
			emailsByAcct[cached.AccountID] = append(emailsByAcct[cached.AccountID], email)
		}

		m.emails = cachedEmails
		m.emailsByAcct = emailsByAcct
		m.inbox = tui.NewInbox(m.emails, m.config.Accounts)
		m.current = m.inbox
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})

		// Start background refresh
		return m, tea.Batch(
			m.current.Init(),
			func() tea.Msg { return tui.RefreshingEmailsMsg{Mailbox: tui.MailboxInbox} },
			refreshEmails(m.config, tui.MailboxInbox),
		)

	case tui.RequestRefreshMsg:
		return m, tea.Batch(
			func() tea.Msg { return tui.RefreshingEmailsMsg{Mailbox: msg.Mailbox} },
			refreshEmails(m.config, msg.Mailbox),
		)

	case tui.EmailsRefreshedMsg:
		if msg.Mailbox == tui.MailboxSent {
			m.sentByAcct = msg.EmailsByAccount
			m.sentEmails = flattenAndSort(msg.EmailsByAccount)
			if m.sentInbox != nil {
				m.sentInbox.SetEmails(m.sentEmails, m.config.Accounts)
				m.current, _ = m.current.Update(msg)
			}
			return m, nil
		}

		m.emailsByAcct = msg.EmailsByAccount
		m.emails = flattenAndSort(msg.EmailsByAccount)

		// Save to cache (inbox only)
		go saveEmailsToCache(m.emails)

		// Update inbox if it exists
		if m.inbox != nil {
			m.inbox.SetEmails(m.emails, m.config.Accounts)
			// Forward the message to inbox to clear refreshing state
			m.current, _ = m.current.Update(msg)
		}
		return m, nil

	case tui.AllEmailsFetchedMsg:
		if msg.Mailbox == tui.MailboxSent {
			m.sentByAcct = msg.EmailsByAccount
			m.sentEmails = flattenAndSort(msg.EmailsByAccount)

			m.sentInbox = tui.NewSentInbox(m.sentEmails, m.config.Accounts)
			m.current = m.sentInbox
			m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, m.current.Init()
		}

		if msg.Mailbox == tui.MailboxTrash {
			m.trashByAcct = msg.EmailsByAccount
			m.trashEmails = flattenAndSort(msg.EmailsByAccount)

			// Create or update trash/archive view
			if m.trashArchive == nil {
				m.trashArchive = tui.NewTrashArchive(m.trashEmails, m.archiveEmails, m.config.Accounts)
			} else {
				m.trashArchive.SetTrashEmails(m.trashEmails, m.config.Accounts)
			}
			m.current = m.trashArchive
			m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, m.current.Init()
		}

		if msg.Mailbox == tui.MailboxArchive {
			m.archiveByAcct = msg.EmailsByAccount
			m.archiveEmails = flattenAndSort(msg.EmailsByAccount)

			// Create or update trash/archive view
			if m.trashArchive == nil {
				m.trashArchive = tui.NewTrashArchive(m.trashEmails, m.archiveEmails, m.config.Accounts)
			} else {
				m.trashArchive.SetArchiveEmails(m.archiveEmails, m.config.Accounts)
			}
			m.current = m.trashArchive
			m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, m.current.Init()
		}

		m.emailsByAcct = msg.EmailsByAccount
		m.emails = flattenAndSort(msg.EmailsByAccount)

		// Save to cache
		go saveEmailsToCache(m.emails)

		m.inbox = tui.NewInbox(m.emails, m.config.Accounts)
		m.current = m.inbox
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.EmailsFetchedMsg:
		if msg.Mailbox == tui.MailboxSent {
			if m.sentByAcct == nil {
				m.sentByAcct = make(map[string][]fetcher.Email)
			}
			m.sentByAcct[msg.AccountID] = msg.Emails
			m.sentEmails = flattenAndSort(m.sentByAcct)
			if m.sentInbox == nil {
				m.sentInbox = tui.NewSentInbox(m.sentEmails, m.config.Accounts)
			} else {
				m.sentInbox.SetEmails(m.sentEmails, m.config.Accounts)
			}
			m.current = m.sentInbox
			m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, m.current.Init()
		}

		// Inbox path
		if m.emailsByAcct == nil {
			m.emailsByAcct = make(map[string][]fetcher.Email)
		}
		m.emailsByAcct[msg.AccountID] = msg.Emails
		m.emails = flattenAndSort(m.emailsByAcct)
		if m.inbox == nil {
			m.inbox = tui.NewInbox(m.emails, m.config.Accounts)
		} else {
			m.inbox.SetEmails(m.emails, m.config.Accounts)
		}
		m.current = m.inbox
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.FetchMoreEmailsMsg:
		if msg.AccountID == "" {
			return m, nil // Don't fetch more for "ALL" view
		}
		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			return m, nil
		}
		return m, tea.Batch(
			func() tea.Msg { return tui.FetchingMoreEmailsMsg{} },
			fetchEmailsForMailbox(account, paginationLimit, msg.Offset, msg.Mailbox),
		)

	case tui.EmailsAppendedMsg:
		if msg.Mailbox == tui.MailboxSent {
			if m.sentByAcct == nil {
				m.sentByAcct = make(map[string][]fetcher.Email)
			}
			m.sentByAcct[msg.AccountID] = append(m.sentByAcct[msg.AccountID], msg.Emails...)
			m.sentEmails = append(m.sentEmails, msg.Emails...)
			return m, nil
		}
		if msg.Mailbox == tui.MailboxTrash {
			if m.trashByAcct == nil {
				m.trashByAcct = make(map[string][]fetcher.Email)
			}
			m.trashByAcct[msg.AccountID] = append(m.trashByAcct[msg.AccountID], msg.Emails...)
			m.trashEmails = append(m.trashEmails, msg.Emails...)
			return m, nil
		}
		if msg.Mailbox == tui.MailboxArchive {
			if m.archiveByAcct == nil {
				m.archiveByAcct = make(map[string][]fetcher.Email)
			}
			m.archiveByAcct[msg.AccountID] = append(m.archiveByAcct[msg.AccountID], msg.Emails...)
			m.archiveEmails = append(m.archiveEmails, msg.Emails...)
			return m, nil
		}
		// Inbox
		if m.emailsByAcct == nil {
			m.emailsByAcct = make(map[string][]fetcher.Email)
		}
		m.emailsByAcct[msg.AccountID] = append(m.emailsByAcct[msg.AccountID], msg.Emails...)
		m.emails = append(m.emails, msg.Emails...)
		return m, nil

	case tui.GoToSendMsg:
		if m.config != nil && len(m.config.Accounts) > 0 {
			firstAccount := m.config.GetFirstAccount()
			composer := tui.NewComposerWithAccounts(m.config.Accounts, firstAccount.ID, msg.To, msg.Subject, msg.Body)
			m.current = composer
		} else {
			m.current = tui.NewComposer("", msg.To, msg.Subject, msg.Body)
		}
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToDraftsMsg:
		drafts := config.GetAllDrafts()
		m.current = tui.NewDrafts(drafts)
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.OpenDraftMsg:
		var accounts []config.Account
		if m.config != nil {
			accounts = m.config.Accounts
		}
		composer := tui.NewComposerFromDraft(msg.Draft, accounts)
		m.current = composer
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
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
		if m.config != nil {
			m.current = tui.NewSettings(m.config.Accounts)
		} else {
			m.current = tui.NewSettings(nil)
		}
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToAddAccountMsg:
		m.current = tui.NewLogin()
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToSignatureEditorMsg:
		m.current = tui.NewSignatureEditor()
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

	case tui.GoToChoiceMenuMsg:
		m.current = tui.NewChoice()
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
			m.current = tui.NewSettings(m.config.Accounts)
			m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		}
		return m, m.current.Init()

	case tui.ViewEmailMsg:
		email := m.getEmailByUIDAndAccount(msg.UID, msg.AccountID, msg.Mailbox)
		if email == nil {
			return m, nil
		}
		m.current = tui.NewStatus("Fetching email content...")
		return m, tea.Batch(m.current.Init(), fetchEmailBodyCmd(m.config, *email, msg.UID, msg.AccountID, msg.Mailbox))

	case tui.EmailBodyFetchedMsg:
		if msg.Err != nil {
			log.Printf("could not fetch email body: %v", msg.Err)
			if msg.Mailbox == tui.MailboxSent && m.sentInbox != nil {
				m.current = m.sentInbox
			} else {
				m.current = m.inbox
			}
			return m, nil
		}

		// Update the email in our stores
		m.updateEmailBodyByUID(msg.UID, msg.AccountID, msg.Mailbox, msg.Body, msg.Attachments)

		email := m.getEmailByUIDAndAccount(msg.UID, msg.AccountID, msg.Mailbox)
		if email == nil {
			if msg.Mailbox == tui.MailboxSent && m.sentInbox != nil {
				m.current = m.sentInbox
			} else {
				m.current = m.inbox
			}
			return m, nil
		}

		// Find the index for the email view (used for display purposes)
		emailIndex := m.getEmailIndex(msg.UID, msg.AccountID, msg.Mailbox)
		emailView := tui.NewEmailView(*email, emailIndex, m.width, m.height, msg.Mailbox)
		m.current = emailView
		return m, m.current.Init()

	case tui.ReplyToEmailMsg:
		to := msg.Email.From
		subject := msg.Email.Subject
		normalizedSubject := strings.ToLower(strings.TrimSpace(subject))
		if !strings.HasPrefix(normalizedSubject, "re:") {
			subject = "Re: " + subject
		}
		quotedText := fmt.Sprintf("\n\nOn %s, %s wrote:\n> %s", msg.Email.Date.Format("Jan 2, 2006 at 3:04 PM"), msg.Email.From, strings.ReplaceAll(msg.Email.Body, "\n", "\n> "))

		var composer *tui.Composer
		if m.config != nil && len(m.config.Accounts) > 0 {
			// Use the account that received the email
			accountID := msg.Email.AccountID
			if accountID == "" {
				accountID = m.config.GetFirstAccount().ID
			}
			composer = tui.NewComposerWithAccounts(m.config.Accounts, accountID, to, subject, "")
		} else {
			composer = tui.NewComposer("", to, subject, "")
		}
		composer.SetQuotedText(quotedText)

		// Set reply headers
		inReplyTo := msg.Email.MessageID
		references := append(msg.Email.References, msg.Email.MessageID)
		composer.SetReplyContext(inReplyTo, references)

		m.current = composer
		m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		return m, m.current.Init()

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
		// Get draft ID before clearing composer (if it's a composer)
		var draftID string
		if composer, ok := m.current.(*tui.Composer); ok {
			draftID = composer.GetDraftID()
		}
		m.current = tui.NewStatus("Sending email...")

		// Get the account to send from
		var account *config.Account
		if msg.AccountID != "" && m.config != nil {
			account = m.config.GetAccountByID(msg.AccountID)
		}
		if account == nil && m.config != nil {
			account = m.config.GetFirstAccount()
		}

		// Save contact and delete draft in background
		go func() {
			// Save the recipient as a contact
			if msg.To != "" {
				// Parse "Name <email>" format
				name, email := parseEmailAddress(msg.To)
				if err := config.AddContact(name, email); err != nil {
					log.Printf("Error saving contact: %v", err)
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
			m.current = tui.NewStatus(fmt.Sprintf("Error: %v", msg.Err))
			return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
				return tui.RestoreViewMsg{}
			})
		}
		m.current = tui.NewChoice()
		return m, m.current.Init()

	case tui.DeleteEmailMsg:
		m.previousModel = m.current
		m.current = tui.NewStatus("Deleting email...")

		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			if msg.Mailbox == tui.MailboxSent && m.sentInbox != nil {
				m.current = m.sentInbox
			} else {
				m.current = m.inbox
			}
			return m, nil
		}

		return m, tea.Batch(m.current.Init(), deleteEmailCmd(account, msg.UID, msg.AccountID, msg.Mailbox))

	case tui.ArchiveEmailMsg:
		m.previousModel = m.current
		m.current = tui.NewStatus("Archiving email...")

		account := m.config.GetAccountByID(msg.AccountID)
		if account == nil {
			if msg.Mailbox == tui.MailboxSent && m.sentInbox != nil {
				m.current = m.sentInbox
			} else {
				m.current = m.inbox
			}
			return m, nil
		}

		return m, tea.Batch(m.current.Init(), archiveEmailCmd(account, msg.UID, msg.AccountID, msg.Mailbox))

	case tui.EmailActionDoneMsg:
		if msg.Err != nil {
			log.Printf("Action failed: %v", msg.Err)
			m.previousModel = m.current
			switch msg.Mailbox {
			case tui.MailboxSent:
				if m.sentInbox != nil {
					m.previousModel = m.sentInbox
				}
			case tui.MailboxTrash, tui.MailboxArchive:
				if m.trashArchive != nil {
					m.previousModel = m.trashArchive
				}
			default:
				if m.inbox != nil {
					m.previousModel = m.inbox
				}
			}
			m.current = tui.NewStatus(fmt.Sprintf("Error: %v", msg.Err))
			return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
				return tui.RestoreViewMsg{}
			})
		}

		// Remove email from stores
		m.removeEmailByMailbox(msg.UID, msg.AccountID, msg.Mailbox)

		if msg.Mailbox == tui.MailboxSent {
			if m.sentInbox != nil {
				m.sentInbox.RemoveEmail(msg.UID, msg.AccountID)
				m.current = m.sentInbox
				m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
				return m, m.current.Init()
			}
			m.current = tui.NewChoice()
			return m, m.current.Init()
		}

		if msg.Mailbox == tui.MailboxTrash || msg.Mailbox == tui.MailboxArchive {
			if m.trashArchive != nil {
				m.trashArchive.RemoveEmail(msg.UID, msg.AccountID, msg.Mailbox)
				m.current = m.trashArchive
				m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
				return m, m.current.Init()
			}
			m.current = tui.NewChoice()
			return m, m.current.Init()
		}

		if m.inbox != nil {
			m.inbox.RemoveEmail(msg.UID, msg.AccountID)
			m.current = m.inbox
			m.current, _ = m.current.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
			return m, m.current.Init()
		}
		m.current = tui.NewChoice()
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

	return m, tea.Batch(cmds...)
}

func (m *mainModel) getEmailByIndex(index int, mailbox tui.MailboxKind) *fetcher.Email {
	switch mailbox {
	case tui.MailboxSent:
		if index >= 0 && index < len(m.sentEmails) {
			return &m.sentEmails[index]
		}
	case tui.MailboxTrash:
		if index >= 0 && index < len(m.trashEmails) {
			return &m.trashEmails[index]
		}
	case tui.MailboxArchive:
		if index >= 0 && index < len(m.archiveEmails) {
			return &m.archiveEmails[index]
		}
	default:
		if index >= 0 && index < len(m.emails) {
			return &m.emails[index]
		}
	}
	return nil
}

func (m *mainModel) getEmailByUIDAndAccount(uid uint32, accountID string, mailbox tui.MailboxKind) *fetcher.Email {
	switch mailbox {
	case tui.MailboxSent:
		for i := range m.sentEmails {
			if m.sentEmails[i].UID == uid && m.sentEmails[i].AccountID == accountID {
				return &m.sentEmails[i]
			}
		}
	case tui.MailboxTrash:
		for i := range m.trashEmails {
			if m.trashEmails[i].UID == uid && m.trashEmails[i].AccountID == accountID {
				return &m.trashEmails[i]
			}
		}
	case tui.MailboxArchive:
		for i := range m.archiveEmails {
			if m.archiveEmails[i].UID == uid && m.archiveEmails[i].AccountID == accountID {
				return &m.archiveEmails[i]
			}
		}
	default:
		for i := range m.emails {
			if m.emails[i].UID == uid && m.emails[i].AccountID == accountID {
				return &m.emails[i]
			}
		}
	}
	return nil
}

func (m *mainModel) getEmailIndex(uid uint32, accountID string, mailbox tui.MailboxKind) int {
	switch mailbox {
	case tui.MailboxSent:
		for i := range m.sentEmails {
			if m.sentEmails[i].UID == uid && m.sentEmails[i].AccountID == accountID {
				return i
			}
		}
	case tui.MailboxTrash:
		for i := range m.trashEmails {
			if m.trashEmails[i].UID == uid && m.trashEmails[i].AccountID == accountID {
				return i
			}
		}
	case tui.MailboxArchive:
		for i := range m.archiveEmails {
			if m.archiveEmails[i].UID == uid && m.archiveEmails[i].AccountID == accountID {
				return i
			}
		}
	default:
		for i := range m.emails {
			if m.emails[i].UID == uid && m.emails[i].AccountID == accountID {
				return i
			}
		}
	}
	return -1
}

func (m *mainModel) updateEmailBodyByUID(uid uint32, accountID string, mailbox tui.MailboxKind, body string, attachments []fetcher.Attachment) {
	switch mailbox {
	case tui.MailboxSent:
		for i := range m.sentEmails {
			if m.sentEmails[i].UID == uid && m.sentEmails[i].AccountID == accountID {
				m.sentEmails[i].Body = body
				m.sentEmails[i].Attachments = attachments
				break
			}
		}
		if emails, ok := m.sentByAcct[accountID]; ok {
			for i := range emails {
				if emails[i].UID == uid {
					emails[i].Body = body
					emails[i].Attachments = attachments
					break
				}
			}
		}
	case tui.MailboxTrash:
		for i := range m.trashEmails {
			if m.trashEmails[i].UID == uid && m.trashEmails[i].AccountID == accountID {
				m.trashEmails[i].Body = body
				m.trashEmails[i].Attachments = attachments
				break
			}
		}
		if emails, ok := m.trashByAcct[accountID]; ok {
			for i := range emails {
				if emails[i].UID == uid {
					emails[i].Body = body
					emails[i].Attachments = attachments
					break
				}
			}
		}
	case tui.MailboxArchive:
		for i := range m.archiveEmails {
			if m.archiveEmails[i].UID == uid && m.archiveEmails[i].AccountID == accountID {
				m.archiveEmails[i].Body = body
				m.archiveEmails[i].Attachments = attachments
				break
			}
		}
		if emails, ok := m.archiveByAcct[accountID]; ok {
			for i := range emails {
				if emails[i].UID == uid {
					emails[i].Body = body
					emails[i].Attachments = attachments
					break
				}
			}
		}
	default:
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
}

func (m *mainModel) removeEmailByMailbox(uid uint32, accountID string, mailbox tui.MailboxKind) {
	switch mailbox {
	case tui.MailboxSent:
		var filtered []fetcher.Email
		for _, e := range m.sentEmails {
			if !(e.UID == uid && e.AccountID == accountID) {
				filtered = append(filtered, e)
			}
		}
		m.sentEmails = filtered
		if emails, ok := m.sentByAcct[accountID]; ok {
			var filteredAcct []fetcher.Email
			for _, e := range emails {
				if e.UID != uid {
					filteredAcct = append(filteredAcct, e)
				}
			}
			m.sentByAcct[accountID] = filteredAcct
		}
	case tui.MailboxTrash:
		var filtered []fetcher.Email
		for _, e := range m.trashEmails {
			if !(e.UID == uid && e.AccountID == accountID) {
				filtered = append(filtered, e)
			}
		}
		m.trashEmails = filtered
		if emails, ok := m.trashByAcct[accountID]; ok {
			var filteredAcct []fetcher.Email
			for _, e := range emails {
				if e.UID != uid {
					filteredAcct = append(filteredAcct, e)
				}
			}
			m.trashByAcct[accountID] = filteredAcct
		}
	case tui.MailboxArchive:
		var filtered []fetcher.Email
		for _, e := range m.archiveEmails {
			if !(e.UID == uid && e.AccountID == accountID) {
				filtered = append(filtered, e)
			}
		}
		m.archiveEmails = filtered
		if emails, ok := m.archiveByAcct[accountID]; ok {
			var filteredAcct []fetcher.Email
			for _, e := range emails {
				if e.UID != uid {
					filteredAcct = append(filteredAcct, e)
				}
			}
			m.archiveByAcct[accountID] = filteredAcct
		}
	default:
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
}

func (m *mainModel) View() string {
	return m.current.View()
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

func refreshEmails(cfg *config.Config, mailbox tui.MailboxKind) tea.Cmd {
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
				if mailbox == tui.MailboxSent {
					emails, err = fetcher.FetchSentEmails(&acc, initialEmailLimit, 0)
				} else {
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
		return tui.EmailsRefreshedMsg{EmailsByAccount: emailsByAccount, Mailbox: mailbox}
	}
}

func saveEmailsToCache(emails []fetcher.Email) {
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

func fetchEmailBodyCmd(cfg *config.Config, email fetcher.Email, uid uint32, accountID string, mailbox tui.MailboxKind) tea.Cmd {
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
	var buf bytes.Buffer
	p := goldmark.New(goldmark.WithRendererOptions(html.WithUnsafe()))
	if err := p.Convert(md, &buf); err != nil {
		return md
	}
	return buf.Bytes()
}

func sendEmail(account *config.Account, msg tui.SendEmailMsg) tea.Cmd {
	return func() tea.Msg {
		if account == nil {
			return tui.EmailResultMsg{Err: fmt.Errorf("no account configured")}
		}

		recipients := []string{msg.To}
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

		if msg.AttachmentPath != "" {
			fileData, err := os.ReadFile(msg.AttachmentPath)
			if err != nil {
				log.Printf("Could not read attachment file %s: %v", msg.AttachmentPath, err)
			} else {
				_, filename := filepath.Split(msg.AttachmentPath)
				attachments[filename] = fileData
			}
		}

		err := sender.SendEmail(account, recipients, msg.Subject, body, string(htmlBody), images, attachments, msg.InReplyTo, msg.References)
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

	// If it's a tar.gz, extract and find the `matcha` binary
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
			if name == "matcha" || strings.Contains(strings.ToLower(name), "matcha") && (hdr.Typeflag == tar.TypeReg) {
				// write out the file
				binPath = filepath.Join(tmpDir, "matcha")
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

	// Attempt to atomically replace
	if err := os.Rename(tmpNew, execPath); err != nil {
		return fmt.Errorf("could not replace executable: %w", err)
	}

	fmt.Println("Successfully updated matcha to", latestTag)
	return nil
}

func main() {
	// If invoked as CLI update command, run updater and exit.
	if len(os.Args) > 1 && os.Args[1] == "update" {
		if err := runUpdateCLI(); err != nil {
			fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	cfg, err := config.LoadConfig()
	var initialModel *mainModel
	if err != nil {
		initialModel = newInitialModel(nil)
	} else {
		initialModel = newInitialModel(cfg)
	}

	p := tea.NewProgram(initialModel, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
