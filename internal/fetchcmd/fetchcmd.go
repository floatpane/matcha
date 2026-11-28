package fetchcmd

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"sync"

	tea "charm.land/bubbletea/v2"
	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/httpclient"
	"github.com/floatpane/matcha/tui"
)

const (
	InitialEmailLimit = 0
	PaginationLimit   = 50
)

// Seams for tests; production code always uses the fetcher functions.
var (
	fetchFolderEmailBody = fetcher.FetchFolderEmailBody
	fetchFolderEmails    = fetcher.FetchFolderEmails
	syncFolderEmails     = fetcher.SyncFolderEmails
)

// snapshotAccounts copies the account list so a command running on another
// goroutine never reads cfg.Accounts while the UI goroutine mutates it
// (account add/edit/delete). Must be called when the command is built, i.e.
// on the UI goroutine.
func snapshotAccounts(cfg *config.Config) []config.Account {
	if cfg == nil {
		return nil
	}
	return slices.Clone(cfg.Accounts)
}

// snapshotAccount copies a single account (see snapshotAccounts).
func snapshotAccount(cfg *config.Config, accountID string) *config.Account {
	if cfg == nil {
		return nil
	}
	acct := cfg.GetAccountByID(accountID)
	if acct == nil {
		return nil
	}
	cp := *acct
	return &cp
}

// recoverMsg converts a panic raised while fetching or parsing a message into
// an error so a malformed email cannot kill the program (Bubble Tea treats a
// panicking command as fatal).
func recoverMsg(r any) error {
	return fmt.Errorf("could not load email: %v", r)
}

// RefreshEmails fetches the latest emails for all configured accounts.
// Accounts that fail are reported in EmailsRefreshedMsg.Errors.
func RefreshEmails(cfg *config.Config, mailbox tui.MailboxKind, counts map[string]int) tea.Cmd {
	accounts := snapshotAccounts(cfg)
	return func() tea.Msg {
		emailsByAccount := make(map[string][]fetcher.Email)
		errs := make(map[string]error)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, account := range accounts {
			wg.Add(1)
			go func(acc config.Account) {
				defer wg.Done()
				var emails []fetcher.Email
				var err error

				limit := uint32(InitialEmailLimit)
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
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					log.Printf("Error fetching from %s: %v", acc.Email, err)
					errs[acc.ID] = err
					return
				}
				emailsByAccount[acc.ID] = emails
			}(account)
		}

		wg.Wait()
		if len(errs) == 0 {
			errs = nil
		}
		return tui.EmailsRefreshedMsg{EmailsByAccount: emailsByAccount, Mailbox: mailbox, Errors: errs}
	}
}

// SearchEmailsCmd searches backend providers across all matching accounts and
// returns the merged results. resolveProvider is the caller-supplied callback
// used to obtain a backend.Provider for each account.
func SearchEmailsCmd(cfg *config.Config, resolveProvider ProviderResolver, query backend.SearchQuery, folderName, accountID string) tea.Cmd {
	var accounts []config.Account
	for _, acc := range snapshotAccounts(cfg) {
		if accountID == "" || acc.ID == accountID {
			accounts = append(accounts, acc)
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), httpclient.IMAPSearchTimeout)
		defer cancel()

		var results []fetcher.Email
		var firstErr error
		succeeded := false
		for i := range accounts {
			acc := &accounts[i]
			p := resolveProvider(acc)
			if p == nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("provider not found for account %s", acc.ID)
				}
				continue
			}
			emails, err := p.Search(ctx, folderName, query)
			if err != nil {
				if errors.Is(err, backend.ErrNotSupported) {
					continue
				}
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			succeeded = true
			results = append(results, BackendEmailsToFetcher(emails)...)
		}
		if !succeeded && firstErr != nil {
			return tui.SearchResultsMsg{Query: query, Err: firstErr}
		}
		SortFetcherEmails(results)

		return tui.SearchResultsMsg{Query: query, Emails: results}
	}
}

// BackendEmailsToFetcher converts backend.Email values into the fetcher.Email
// type used by the TUI.
func BackendEmailsToFetcher(emails []backend.Email) []fetcher.Email {
	result := make([]fetcher.Email, len(emails))
	for i, e := range emails {
		result[i] = fetcher.Email{
			UID: e.UID, From: e.From, To: e.To, ReplyTo: e.ReplyTo,
			Subject: e.Subject, Body: e.Body, Date: e.Date, IsRead: e.IsRead,
			MessageID: e.MessageID, References: e.References, AccountID: e.AccountID,
		}
	}
	return result
}

// SortFetcherEmails sorts emails newest-first, using UID as a tie-breaker.
func SortFetcherEmails(emails []fetcher.Email) {
	slices.SortStableFunc(emails, func(a, b fetcher.Email) int {
		if c := b.Date.Compare(a.Date); c != 0 {
			return c
		}
		return cmp.Compare(b.UID, a.UID)
	})
}

// FetchFoldersCmd fetches the folder list across all configured accounts.
func FetchFoldersCmd(cfg *config.Config) tea.Cmd {
	accounts := snapshotAccounts(cfg)
	return func() tea.Msg {
		if len(accounts) == 0 {
			return nil
		}
		foldersByAccount := make(map[string][]fetcher.Folder)
		errsByAccount := make(map[string]error)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, account := range accounts {
			wg.Add(1)
			go func(acc config.Account) {
				defer wg.Done()
				folders, err := fetcher.FetchFolders(&acc)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errsByAccount[acc.ID] = err
					return
				}
				foldersByAccount[acc.ID] = folders
			}(account)
		}
		wg.Wait()

		return tui.FoldersFetchedMsg{
			FoldersByAccount: foldersByAccount,
			MergedFolders:    MergeFolders(accounts, foldersByAccount),
			Errors:           errsByAccount,
		}
	}
}

// MergeFolders merges per-account folder lists into one list of unique
// folder names. Unread counts of folders present in several accounts are
// summed, matching how the cached counts are combined at startup. Accounts are
// visited in configuration order so the result is deterministic.
func MergeFolders(accounts []config.Account, foldersByAccount map[string][]fetcher.Folder) []fetcher.Folder {
	var merged []fetcher.Folder
	index := make(map[string]int)
	for _, acc := range accounts {
		for _, f := range foldersByAccount[acc.ID] {
			if i, ok := index[f.Name]; ok {
				merged[i].Unread += f.Unread
				continue
			}
			index[f.Name] = len(merged)
			merged = append(merged, f)
		}
	}
	return merged
}

// FetchFolderEmailsCmd fetches emails for a single folder across all accounts.
// Accounts that fail are listed in FolderEmailsFetchedMsg.Errors instead of
// being silently reported as empty.
func FetchFolderEmailsCmd(cfg *config.Config, folderName string) tea.Cmd {
	accounts := snapshotAccounts(cfg)
	return func() tea.Msg {
		emailsByAccount := make(map[string][]fetcher.Email)
		errs := make(map[string]error)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, account := range accounts {
			wg.Add(1)
			go func(acc config.Account) {
				defer wg.Done()
				// The whole folder, synced incrementally rather than refetched.
				emails, err := syncFolderEmails(&acc, folderName)
				mu.Lock()
				defer mu.Unlock()
				if err != nil {
					errs[acc.ID] = err
					return
				}
				emailsByAccount[acc.ID] = emails
			}(account)
		}

		wg.Wait()

		var allEmails []fetcher.Email
		for _, acc := range accounts {
			allEmails = append(allEmails, emailsByAccount[acc.ID]...)
		}
		SortFetcherEmails(allEmails)

		if len(errs) == 0 {
			errs = nil
		}
		return tui.FolderEmailsFetchedMsg{
			Emails:     allEmails,
			FolderName: folderName,
			Errors:     errs,
		}
	}
}

// FetchFolderEmailsPaginatedCmd fetches the next page of folder emails for a single account.
func FetchFolderEmailsPaginatedCmd(account *config.Account, folderName string, limit, offset uint32) tea.Cmd {
	if account == nil {
		return nil
	}
	acct := *account
	return func() tea.Msg {
		emails, err := fetcher.FetchFolderEmails(&acct, folderName, limit, offset)
		if err != nil {
			return tui.FetchErr(err)
		}
		return tui.FolderEmailsAppendedMsg{
			Emails:     emails,
			AccountID:  acct.ID,
			FolderName: folderName,
		}
	}
}

// FetchFolderEmailBodyCmd fetches the full body of a folder email.
func FetchFolderEmailBodyCmd(cfg *config.Config, uid uint32, accountID string, folderName string, mailbox tui.MailboxKind) tea.Cmd {
	account := snapshotAccount(cfg, accountID)
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = tui.EmailBodyFetchedMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Folder: folderName, Err: recoverMsg(r)}
			}
		}()
		if account == nil {
			return tui.EmailBodyFetchedMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Folder: folderName, Err: fmt.Errorf("account not found")}
		}

		body, bodyMIMEType, attachments, err := fetchFolderEmailBody(account, folderName, uid)
		if err != nil {
			return tui.EmailBodyFetchedMsg{UID: uid, AccountID: accountID, Mailbox: mailbox, Folder: folderName, Err: err}
		}

		return tui.EmailBodyFetchedMsg{
			UID:          uid,
			Body:         body,
			BodyMIMEType: bodyMIMEType,
			Attachments:  attachments,
			AccountID:    accountID,
			Mailbox:      mailbox,
			Folder:       folderName,
		}
	}
}

// FetchPreviewBodyCmd fetches a preview body for an email without a mailbox kind.
func FetchPreviewBodyCmd(cfg *config.Config, uid uint32, accountID string, folderName string) tea.Cmd {
	account := snapshotAccount(cfg, accountID)
	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = tui.PreviewBodyFetchedMsg{UID: uid, AccountID: accountID, Folder: folderName, Err: recoverMsg(r)}
			}
		}()
		if account == nil {
			return tui.PreviewBodyFetchedMsg{UID: uid, AccountID: accountID, Folder: folderName, Err: fmt.Errorf("account not found")}
		}

		body, bodyMIMEType, attachments, err := fetchFolderEmailBody(account, folderName, uid)
		if err != nil {
			return tui.PreviewBodyFetchedMsg{UID: uid, AccountID: accountID, Folder: folderName, Err: err}
		}

		return tui.PreviewBodyFetchedMsg{
			UID:          uid,
			Body:         body,
			BodyMIMEType: bodyMIMEType,
			Attachments:  attachments,
			AccountID:    accountID,
			Folder:       folderName,
		}
	}
}

// MarkEmailAsReadCmd marks a single email as read in its folder mailbox.
func MarkEmailAsReadCmd(account *config.Account, uid uint32, accountID string, folderName string) tea.Cmd {
	if account == nil {
		return nil
	}
	acct := *account
	return func() tea.Msg {
		err := fetcher.MarkEmailAsReadInMailbox(&acct, folderName, uid)
		return tui.EmailMarkedReadMsg{UID: uid, AccountID: accountID, Err: err}
	}
}

// MarkEmailAsUnreadCmd marks a single email as unread in its folder mailbox.
func MarkEmailAsUnreadCmd(account *config.Account, uid uint32, accountID string, folderName string) tea.Cmd {
	if account == nil {
		return nil
	}
	acct := *account
	return func() tea.Msg {
		err := fetcher.MarkEmailAsUnreadInMailbox(&acct, folderName, uid)
		return tui.EmailMarkedUnreadMsg{UID: uid, AccountID: accountID, Err: err}
	}
}
