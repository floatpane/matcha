package emailstore

import (
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/tui"
)

const (
	InitialEmailLimit = 0
	PaginationLimit   = 50
	MaxCacheEmails    = 100
	FolderInbox       = "INBOX"
)

const (
	ActionKindDelete  = "delete"
	ActionKindArchive = "archive"
	ActionKindMove    = "move"
)

// PendingAction holds context for an undoable email action (delete, archive,
// move) while it sits in its grace period.
type PendingAction struct {
	JobID      string
	Kind       string
	UIDs       []uint32
	AccountID  string
	FolderName string
	DestFolder string
	Mailbox    tui.MailboxKind
	EmailsSnap []fetcher.Email
	AcctSnap   []fetcher.Email
	FolderSnap []fetcher.Email
}

// PendingExport holds context for an in-progress email export while the save
// file picker is open (non-macOS platforms).
type PendingExport struct {
	Email   fetcher.Email
	Account string
	Folder  string
	Mailbox tui.MailboxKind
	Format  string
}

// Store centralizes all in-memory email storage and helper operations.
type Store struct {
	Emails       []fetcher.Email
	EmailsByAcct map[string][]fetcher.Email
	FolderEmails map[string][]fetcher.Email
}

func NewStore() *Store {
	return &Store{
		Emails:       []fetcher.Email{},
		EmailsByAcct: make(map[string][]fetcher.Email),
		FolderEmails: make(map[string][]fetcher.Email),
	}
}

func (s *Store) CloneAccount(accountID string) []fetcher.Email {
	if s.EmailsByAcct == nil {
		return nil
	}
	return slices.Clone(s.EmailsByAcct[accountID])
}

func (s *Store) CloneFolder(folderName string) []fetcher.Email {
	if s.FolderEmails == nil {
		return nil
	}
	return slices.Clone(s.FolderEmails[folderName])
}

func (s *Store) CloneEmails() []fetcher.Email {
	return slices.Clone(s.Emails)
}

func (s *Store) SetFolder(folderName string, emails []fetcher.Email) {
	if s.FolderEmails == nil {
		s.FolderEmails = make(map[string][]fetcher.Email)
	}
	s.FolderEmails[folderName] = emails
	s.EmailsByAcct = make(map[string][]fetcher.Email)
	for _, e := range emails {
		s.EmailsByAcct[e.AccountID] = append(s.EmailsByAcct[e.AccountID], e)
	}
	s.Emails = FlattenAndSort(s.EmailsByAcct)
}

func (s *Store) AppendToFolder(folderName string, emails []fetcher.Email) {
	if s.FolderEmails == nil {
		s.FolderEmails = make(map[string][]fetcher.Email)
	}
	s.FolderEmails[folderName] = append(s.FolderEmails[folderName], emails...)
	for _, e := range emails {
		s.Emails = append(s.Emails, e)
		s.EmailsByAcct[e.AccountID] = append(s.EmailsByAcct[e.AccountID], e)
	}
}

func (s *Store) MergeRefreshed(refreshed map[string][]fetcher.Email) {
	for accID, fresh := range refreshed {
		refreshedUIDs := make(map[uint32]struct{}, len(fresh))
		for _, e := range fresh {
			refreshedUIDs[e.UID] = struct{}{}
		}
		if existing, ok := s.EmailsByAcct[accID]; ok {
			for _, e := range existing {
				if _, found := refreshedUIDs[e.UID]; !found {
					fresh = append(fresh, e)
				}
			}
		}
		s.EmailsByAcct[accID] = fresh
	}
	s.Emails = FlattenAndSort(s.EmailsByAcct)
}

func (s *Store) RemoveAccount(accountID string) {
	delete(s.EmailsByAcct, accountID)
	s.Emails = FlattenAndSort(s.EmailsByAcct)
}

func (s *Store) GetEmailByIndex(index int) *fetcher.Email {
	if index >= 0 && index < len(s.Emails) {
		return &s.Emails[index]
	}
	return nil
}

func (s *Store) GetEmailByUIDAndAccount(uid uint32, accountID string) *fetcher.Email {
	for i := range s.Emails {
		if s.Emails[i].UID == uid && s.Emails[i].AccountID == accountID {
			return &s.Emails[i]
		}
	}
	return nil
}

func (s *Store) GetEmailIndex(uid uint32, accountID string) int {
	for i := range s.Emails {
		if s.Emails[i].UID == uid && s.Emails[i].AccountID == accountID {
			return i
		}
	}
	return -1
}

func (s *Store) UpdateEmailBodyByUID(uid uint32, accountID string, body, bodyMIMEType string, attachments []fetcher.Attachment) {
	for i := range s.Emails {
		if s.Emails[i].UID == uid && s.Emails[i].AccountID == accountID {
			s.Emails[i].Body = body
			s.Emails[i].BodyMIMEType = bodyMIMEType
			s.Emails[i].Attachments = attachments
			break
		}
	}
	if emails, ok := s.EmailsByAcct[accountID]; ok {
		for i := range emails {
			if emails[i].UID == uid {
				emails[i].Body = body
				emails[i].BodyMIMEType = bodyMIMEType
				emails[i].Attachments = attachments
				break
			}
		}
	}
}

func (s *Store) AddEmailToStoresIfMissing(email fetcher.Email, _ tui.MailboxKind) {
	if s.GetEmailByUIDAndAccount(email.UID, email.AccountID) != nil {
		return
	}
	if s.EmailsByAcct == nil {
		s.EmailsByAcct = make(map[string][]fetcher.Email)
	}
	s.EmailsByAcct[email.AccountID] = append(s.EmailsByAcct[email.AccountID], email)
	s.Emails = FlattenAndSort(s.EmailsByAcct)
}

func (s *Store) MarkEmailAsReadInStores(uid uint32, accountID string) {
	for i := range s.Emails {
		if s.Emails[i].UID == uid && s.Emails[i].AccountID == accountID {
			s.Emails[i].IsRead = true
			break
		}
	}
	if emails, ok := s.EmailsByAcct[accountID]; ok {
		for i := range emails {
			if emails[i].UID == uid {
				emails[i].IsRead = true
				break
			}
		}
	}
	for folderName, folderEmails := range s.FolderEmails {
		for i := range folderEmails {
			if folderEmails[i].UID == uid && folderEmails[i].AccountID == accountID {
				folderEmails[i].IsRead = true
				s.FolderEmails[folderName] = folderEmails
				go SaveFolderEmailsToCache(folderName, folderEmails)
				break
			}
		}
	}
}

func (s *Store) MarkEmailAsUnreadInStores(uid uint32, accountID string) {
	for i := range s.Emails {
		if s.Emails[i].UID == uid && s.Emails[i].AccountID == accountID {
			s.Emails[i].IsRead = false
			break
		}
	}
	if emails, ok := s.EmailsByAcct[accountID]; ok {
		for i := range emails {
			if emails[i].UID == uid {
				emails[i].IsRead = false
				break
			}
		}
	}
	for folderName, folderEmails := range s.FolderEmails {
		for i := range folderEmails {
			if folderEmails[i].UID == uid && folderEmails[i].AccountID == accountID {
				folderEmails[i].IsRead = false
				s.FolderEmails[folderName] = folderEmails
				go SaveFolderEmailsToCache(folderName, folderEmails)
				break
			}
		}
	}
}

func (s *Store) RemoveEmailFromStores(uid uint32, accountID string) {
	var filtered []fetcher.Email
	for _, e := range s.Emails {
		if e.UID != uid || e.AccountID != accountID {
			filtered = append(filtered, e)
		}
	}
	s.Emails = filtered
	if emails, ok := s.EmailsByAcct[accountID]; ok {
		var filteredAcct []fetcher.Email
		for _, e := range emails {
			if e.UID != uid {
				filteredAcct = append(filteredAcct, e)
			}
		}
		s.EmailsByAcct[accountID] = filteredAcct
	}
}

// AddGmailLabel adds a Gmail label to the email with the given UID and account,
// updating all in-memory stores. No-op if the label already exists.
func (s *Store) AddGmailLabel(uid uint32, accountID, label string) {
	s.updateEmailLabels(uid, accountID, func(labels []string) []string {
		for _, l := range labels {
			if strings.EqualFold(l, label) {
				return labels
			}
		}
		return append(labels, label)
	})
}

// RemoveGmailLabel removes a Gmail label from the email with the given UID and
// account, updating all in-memory stores. No-op if the label doesn't exist.
func (s *Store) RemoveGmailLabel(uid uint32, accountID, label string) {
	s.updateEmailLabels(uid, accountID, func(labels []string) []string {
		var filtered []string
		for _, l := range labels {
			if !strings.EqualFold(l, label) {
				filtered = append(filtered, l)
			}
		}
		return filtered
	})
}

func (s *Store) updateEmailLabels(uid uint32, accountID string, fn func([]string) []string) {
	for i := range s.Emails {
		if s.Emails[i].UID == uid && s.Emails[i].AccountID == accountID {
			s.Emails[i].Labels = fn(s.Emails[i].Labels)
			break
		}
	}
	if emails, ok := s.EmailsByAcct[accountID]; ok {
		for i := range emails {
			if emails[i].UID == uid {
				emails[i].Labels = fn(emails[i].Labels)
				break
			}
		}
	}
	for folderName, folderEmails := range s.FolderEmails {
		for i := range folderEmails {
			if folderEmails[i].UID == uid && folderEmails[i].AccountID == accountID {
				folderEmails[i].Labels = fn(folderEmails[i].Labels)
				s.FolderEmails[folderName] = folderEmails
				go SaveFolderEmailsToCache(folderName, folderEmails)
				break
			}
		}
	}
}

func (s *Store) RemoveFolderEmail(folderName string, uid uint32, accountID string) []fetcher.Email {
	emails, ok := s.FolderEmails[folderName]
	if !ok {
		return nil
	}
	var filtered []fetcher.Email
	for _, e := range emails {
		if e.UID != uid || e.AccountID != accountID {
			filtered = append(filtered, e)
		}
	}
	s.FolderEmails[folderName] = filtered
	go SaveFolderEmailsToCache(folderName, filtered)
	return filtered
}

func (s *Store) RemoveFolderEmails(folderName, accountID string, uids []uint32) []fetcher.Email {
	emails, ok := s.FolderEmails[folderName]
	if !ok {
		return nil
	}
	var filtered []fetcher.Email
	for _, e := range emails {
		if e.AccountID != accountID || !slices.Contains(uids, e.UID) {
			filtered = append(filtered, e)
		}
	}
	s.FolderEmails[folderName] = filtered
	go SaveFolderEmailsToCache(folderName, filtered)
	return filtered
}

func FlattenAndSort(emailsByAccount map[string][]fetcher.Email) []fetcher.Email {
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

func FilterUnique(existing, incoming []fetcher.Email) []fetcher.Email {
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

func EmailsToCache(emails []fetcher.Email) []config.CachedEmail {
	cached := make([]config.CachedEmail, 0, len(emails))
	for _, email := range emails {
		cached = append(cached, config.CachedEmail{
			UID:        email.UID,
			From:       email.From,
			To:         email.To,
			Subject:    email.Subject,
			Date:       email.Date,
			MessageID:  email.MessageID,
			InReplyTo:  email.InReplyTo,
			References: email.References,
			AccountID:  email.AccountID,
			IsRead:     email.IsRead,
			Labels:     email.Labels,
		})
	}
	return cached
}

func CacheToEmails(cached []config.CachedEmail) []fetcher.Email {
	emails := make([]fetcher.Email, 0, len(cached))
	for _, c := range cached {
		emails = append(emails, fetcher.Email{
			UID:        c.UID,
			From:       c.From,
			To:         c.To,
			Subject:    c.Subject,
			Date:       c.Date,
			MessageID:  c.MessageID,
			InReplyTo:  c.InReplyTo,
			References: c.References,
			AccountID:  c.AccountID,
			IsRead:     c.IsRead,
			Labels:     c.Labels,
		})
	}
	return emails
}

func SaveFolderEmailsToCache(folderName string, emails []fetcher.Email) {
	cached := EmailsToCache(emails)
	if err := config.SaveFolderEmailCache(folderName, cached); err != nil {
		log.Printf("Error saving folder email cache for %s: %v", folderName, err)
	}
}

func LoadFolderEmailsFromCache(folderName string) []fetcher.Email {
	cached, err := config.LoadFolderEmailCache(folderName)
	if err != nil {
		return nil
	}
	return CacheToEmails(cached)
}

func NewPendingAction(kind, accountID, folderName, destFolder string, uids []uint32, mailbox tui.MailboxKind, emailsSnap, acctSnap, folderSnap []fetcher.Email) *PendingAction {
	return &PendingAction{
		JobID:      fmt.Sprintf("action-%d", time.Now().UnixNano()),
		Kind:       kind,
		UIDs:       uids,
		AccountID:  accountID,
		FolderName: folderName,
		DestFolder: destFolder,
		Mailbox:    mailbox,
		EmailsSnap: emailsSnap,
		AcctSnap:   acctSnap,
		FolderSnap: folderSnap,
	}
}
