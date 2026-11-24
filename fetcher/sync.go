package fetcher

import (
	"cmp"
	"log"
	"slices"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/floatpane/matcha/config"
)

// fullSyncInterval is how often a folder is still fetched in full. The
// incremental sync is complete on its own; the daily full fetch only repairs
// drift, such as a read flag another writer of the cache overwrote.
const fullSyncInterval = 24 * time.Hour

// syncChunkSize is how many UIDs one header fetch of new mail asks for.
const syncChunkSize = 1000

var syncNow = time.Now

// SyncFolderEmails returns all of an account's emails in a folder, the same
// list FetchFolderEmails(account, folder, 0, 0) returns, but downloads only
// what changed since the last sync: headers of new mail, the flags that
// changed (CONDSTORE) and the folder's UID list (ESEARCH, a few KB). A full
// fetch of every header (16 MB for a 44k-message Gmail INBOX) runs into
// Gmail's IMAP bandwidth limit when it repeats, after which Gmail delays
// every command by seconds. The full fetch now happens on the first sync,
// when the folder's UIDVALIDITY changes, without CONDSTORE, and once a day.
//
// The result is also written to the folder's email cache and sync state.
func SyncFolderEmails(account *config.Account, folder string) ([]Email, error) {
	if hasBackendProvider(account) {
		emails, err := FetchFolderEmails(account, folder, 0, 0)
		if err != nil {
			return nil, err
		}
		if err := config.ReplaceAccountFolderEmails(folder, account.ID, ToCachedEmails(emails)); err != nil {
			log.Printf("sync: could not cache %s: %v", folder, err)
		}
		return emails, nil
	}

	prev, hasPrev := config.LoadFolderSyncState(folder, account.ID)
	var known []Email
	if hasPrev {
		cached, _ := config.LoadFolderEmailCache(folder)
		for _, e := range FromCachedEmails(cached) {
			if e.AccountID == account.ID {
				known = append(known, e)
			}
		}
	}

	emails, state, err := syncIMAPFolder(account, folder, known, prev, hasPrev)
	if err != nil {
		return nil, err
	}

	// The list is written before the state: if the state write is lost, the
	// next sync redoes some work but never misses mail.
	if err := config.ReplaceAccountFolderEmails(folder, account.ID, ToCachedEmails(emails)); err != nil {
		log.Printf("sync: could not cache %s: %v", folder, err)
		return emails, nil
	}
	if err := config.SaveFolderSyncState(folder, account.ID, state); err != nil {
		log.Printf("sync: could not save sync state of %s: %v", folder, err)
	}
	return emails, nil
}

func syncIMAPFolder(account *config.Account, mailbox string, known []Email, prev config.FolderSyncState, hasPrev bool) ([]Email, config.FolderSyncState, error) {
	c, err := connect(account)
	if err != nil {
		return nil, config.FolderSyncState{}, err
	}
	defer c.Close() //nolint:errcheck

	caps := c.Caps()
	condStore := caps.Has(imap.CapCondStore)
	var selectOpts *imap.SelectOptions
	if condStore {
		selectOpts = &imap.SelectOptions{CondStore: true}
	}
	sel, err := c.Select(mailbox, selectOpts).Wait()
	if err != nil {
		return nil, config.FolderSyncState{}, err
	}

	now := syncNow()
	state := config.FolderSyncState{
		UIDValidity:   sel.UIDValidity,
		HighestModSeq: sel.HighestModSeq,
		FullAt:        prev.FullAt,
	}

	if needsFullSync(prev, hasPrev, sel, condStore, now) {
		emails, skipped, err := fetchSelectedEmails(c, account, mailbox, sel.NumMessages, 0, 0)
		if err != nil {
			return nil, config.FolderSyncState{}, err
		}
		state.Skipped = skipped
		state.FullAt = now
		return emails, state, nil
	}

	// Every UID in the folder: what is gone and what is new.
	var searchOpts *imap.SearchOptions
	if caps.Has(imap.CapESearch) {
		searchOpts = &imap.SearchOptions{ReturnAll: true}
	}
	search, err := c.UIDSearch(&imap.SearchCriteria{}, searchOpts).Wait()
	if err != nil {
		return nil, config.FolderSyncState{}, err
	}
	present := make(map[uint32]struct{}, len(known))
	for _, uid := range search.AllUIDs() {
		present[uint32(uid)] = struct{}{}
	}

	// New mail is any UID not known yet, rather than UIDs past the last
	// UIDNEXT: mail lost from the cache by another writer is fetched again.
	seen := make(map[uint32]struct{}, len(known)+len(prev.Skipped))
	for _, e := range known {
		seen[e.UID] = struct{}{}
	}
	for _, uid := range prev.Skipped {
		if _, ok := present[uid]; ok {
			seen[uid] = struct{}{}
			state.Skipped = append(state.Skipped, uid)
		}
	}
	var newUIDs []uint32
	for uid := range present {
		if _, ok := seen[uid]; !ok {
			newUIDs = append(newUIDs, uid)
		}
	}
	slices.Sort(newUIDs)

	fresh, skipped, err := fetchEmailsByUID(c, account, mailbox, newUIDs)
	if err != nil {
		return nil, config.FolderSyncState{}, err
	}
	state.Skipped = append(state.Skipped, skipped...)

	var readFlags map[uint32]bool
	if sel.HighestModSeq > prev.HighestModSeq {
		readFlags, err = fetchChangedReadFlags(c, prev.HighestModSeq)
		if err != nil {
			return nil, config.FolderSyncState{}, err
		}
	}

	return mergeSync(known, present, readFlags, fresh), state, nil
}

func needsFullSync(prev config.FolderSyncState, hasPrev bool, sel *imap.SelectData, condStore bool, now time.Time) bool {
	return !hasPrev ||
		!condStore ||
		sel.HighestModSeq == 0 ||
		prev.UIDValidity != sel.UIDValidity ||
		now.Sub(prev.FullAt) >= fullSyncInterval
}

// fetchEmailsByUID fetches headers of the given UIDs in chunks.
func fetchEmailsByUID(c *imapclient.Client, account *config.Account, mailbox string, uids []uint32) ([]Email, []uint32, error) {
	if len(uids) == 0 {
		return nil, nil, nil
	}
	filter := newHeaderFilter(c, account, mailbox)
	var emails []Email
	var skipped []uint32
	for start := 0; start < len(uids); start += syncChunkSize {
		chunk := uids[start:min(start+syncChunkSize, len(uids))]
		msgs, err := c.Fetch(uidsToUIDSet(chunk), filter.fetchOptions()).Collect()
		if err != nil {
			return nil, nil, err
		}
		kept, left := filter.build(msgs)
		emails = append(emails, kept...)
		skipped = append(skipped, left...)
	}
	return emails, skipped, nil
}

// fetchChangedReadFlags returns the read state of every message whose flags
// changed after modSeq.
func fetchChangedReadFlags(c *imapclient.Client, modSeq uint64) (map[uint32]bool, error) {
	var all imap.UIDSet
	all.AddRange(1, 0) // 1:*
	msgs, err := c.Fetch(all, &imap.FetchOptions{
		UID:          true,
		Flags:        true,
		ChangedSince: modSeq,
	}).Collect()
	if err != nil {
		return nil, err
	}
	flags := make(map[uint32]bool, len(msgs))
	for _, msg := range msgs {
		flags[uint32(msg.UID)] = hasSeenFlag(msg.Flags)
	}
	return flags, nil
}

// mergeSync applies an incremental sync to the known emails: those no longer
// present are dropped, changed read flags are applied, and fresh emails are
// added (replacing a known copy). The result is sorted newest first.
func mergeSync(known []Email, present map[uint32]struct{}, readFlags map[uint32]bool, fresh []Email) []Email {
	freshUIDs := make(map[uint32]struct{}, len(fresh))
	for _, e := range fresh {
		freshUIDs[e.UID] = struct{}{}
	}
	merged := make([]Email, 0, len(known)+len(fresh))
	for _, e := range known {
		if _, ok := present[e.UID]; !ok {
			continue
		}
		if _, ok := freshUIDs[e.UID]; ok {
			continue
		}
		if read, ok := readFlags[e.UID]; ok {
			e.IsRead = read
		}
		merged = append(merged, e)
	}
	merged = append(merged, fresh...)
	slices.SortStableFunc(merged, func(a, b Email) int {
		if c := b.Date.Compare(a.Date); c != 0 {
			return c
		}
		return cmp.Compare(b.UID, a.UID)
	})
	return merged
}

// ToCachedEmails converts emails to their folder-cache form.
func ToCachedEmails(emails []Email) []config.CachedEmail {
	cached := make([]config.CachedEmail, 0, len(emails))
	for _, e := range emails {
		cached = append(cached, config.CachedEmail{
			UID:        e.UID,
			From:       e.From,
			To:         e.To,
			ReplyTo:    e.ReplyTo,
			Subject:    e.Subject,
			Date:       e.Date,
			MessageID:  e.MessageID,
			InReplyTo:  e.InReplyTo,
			References: e.References,
			AccountID:  e.AccountID,
			IsRead:     e.IsRead,
		})
	}
	return cached
}

// FromCachedEmails converts folder-cache entries back to emails.
func FromCachedEmails(cached []config.CachedEmail) []Email {
	emails := make([]Email, 0, len(cached))
	for _, c := range cached {
		emails = append(emails, Email{
			UID:        c.UID,
			From:       c.From,
			To:         c.To,
			ReplyTo:    c.ReplyTo,
			Subject:    c.Subject,
			Date:       c.Date,
			MessageID:  c.MessageID,
			InReplyTo:  c.InReplyTo,
			References: c.References,
			AccountID:  c.AccountID,
			IsRead:     c.IsRead,
		})
	}
	return emails
}
