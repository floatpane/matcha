package emailstore

import (
	"cmp"
	"fmt"
	"log"
	"slices"
	"strconv"
	"sync"
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
				SaveFolderEmailsAsync(folderName, folderEmails)
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
				SaveFolderEmailsAsync(folderName, folderEmails)
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
	SaveFolderEmailsAsync(folderName, filtered)
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
	SaveFolderEmailsAsync(folderName, filtered)
	return filtered
}

// FlattenAndSort merges per-account lists into one list sorted newest first.
// It runs on the UI goroutine for every folder load, so it must stay
// O(n log n): the previous exchange sort took ~0.8s for 20k emails.
func FlattenAndSort(emailsByAccount map[string][]fetcher.Email) []fetcher.Email {
	var allEmails []fetcher.Email
	for _, emails := range emailsByAccount {
		allEmails = append(allEmails, emails...)
	}
	SortNewestFirst(allEmails)
	return allEmails
}

// SortNewestFirst sorts emails by date, newest first (UID descending breaks ties).
func SortNewestFirst(emails []fetcher.Email) {
	slices.SortStableFunc(emails, func(a, b fetcher.Email) int {
		if c := b.Date.Compare(a.Date); c != 0 {
			return c
		}
		return cmp.Compare(b.UID, a.UID)
	})
}

// emailKey identifies an email across accounts (UIDs are per account/folder).
func emailKey(e fetcher.Email) string {
	return e.AccountID + ":" + strconv.FormatUint(uint64(e.UID), 10)
}

// NewEmails returns the emails of fetched that are not in previous (matched
// by account and UID). Use it to fire "email received" plugin hooks only for
// mail that actually arrived instead of for every email on every refresh.
func NewEmails(previous, fetched []fetcher.Email) []fetcher.Email {
	known := make(map[string]struct{}, len(previous))
	for _, e := range previous {
		known[emailKey(e)] = struct{}{}
	}
	var fresh []fetcher.Email
	for _, e := range fetched {
		if _, ok := known[emailKey(e)]; !ok {
			fresh = append(fresh, e)
		}
	}
	return fresh
}

// MergeFolderFetch combines a folder fetch result with the previously known
// emails of the accounts whose fetch failed, so a transient error (offline,
// expired login) does not blank those accounts' emails. The result is sorted
// newest first. With no failures it returns fetched unchanged.
func MergeFolderFetch(previous, fetched []fetcher.Email, failed map[string]error) []fetcher.Email {
	if len(failed) == 0 {
		return fetched
	}
	merged := slices.Clone(fetched)
	for _, e := range previous {
		if _, ok := failed[e.AccountID]; ok {
			merged = append(merged, e)
		}
	}
	SortNewestFirst(merged)
	return merged
}

// CacheFolderSnapshot persists a folder's email list and prunes the folder's
// body cache in the background. accountIDs are the configured accounts;
// accounts in failed are left untouched (their fetch did not tell us which
// emails still exist). The slice is copied before this returns, so the caller
// may keep mutating it.
func CacheFolderSnapshot(folderName string, emails []fetcher.Email, failed map[string]error, accountIDs []string, bodyCacheThreshold int) {
	SaveFolderEmailsAsync(folderName, emails)

	valid := make(map[string]map[uint32]bool, len(accountIDs))
	for _, id := range accountIDs {
		if _, bad := failed[id]; !bad {
			valid[id] = make(map[uint32]bool)
		}
	}
	for _, e := range emails {
		if uids, ok := valid[e.AccountID]; ok {
			uids[e.UID] = true
		}
	}
	go func() {
		if err := config.PruneEmailBodyCacheByAccount(folderName, valid, bodyCacheThreshold); err != nil {
			log.Printf("Error pruning body cache for %s: %v", folderName, err)
		}
	}()
}

// folderSaves orders background saves of the same folder: each snapshot gets
// a sequence number and a save is skipped when a newer snapshot is queued, so
// a stale snapshot can never overwrite a newer one. writeMu serializes the
// check-and-write so an older save cannot land after a newer one.
var folderSaves = struct {
	sync.Mutex
	writeMu sync.Mutex
	seq     map[string]uint64
}{seq: make(map[string]uint64)}

// SaveFolderEmailsAsync converts emails to their cache form on the calling
// goroutine (so the caller may keep mutating the slice) and writes them in
// the background.
func SaveFolderEmailsAsync(folderName string, emails []fetcher.Email) {
	cached := EmailsToCache(emails)
	folderSaves.Lock()
	folderSaves.seq[folderName]++
	n := folderSaves.seq[folderName]
	folderSaves.Unlock()

	go func() {
		folderSaves.writeMu.Lock()
		defer folderSaves.writeMu.Unlock()
		folderSaves.Lock()
		latest := folderSaves.seq[folderName] == n
		folderSaves.Unlock()
		if !latest {
			return
		}
		if err := config.SaveFolderEmailCache(folderName, cached); err != nil {
			log.Printf("Error saving folder email cache for %s: %v", folderName, err)
		}
	}()
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
	return fetcher.ToCachedEmails(emails)
}

func CacheToEmails(cached []config.CachedEmail) []fetcher.Email {
	return fetcher.FromCachedEmails(cached)
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
