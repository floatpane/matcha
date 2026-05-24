package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CachedEmail stores essential email data for caching.
type CachedEmail struct {
	UID        uint32    `json:"uid"`
	From       string    `json:"from"`
	To         []string  `json:"to"`
	Subject    string    `json:"subject"`
	Date       time.Time `json:"date"`
	MessageID  string    `json:"message_id"`
	InReplyTo  string    `json:"in_reply_to,omitempty"`
	References []string  `json:"references,omitempty"`
	AccountID  string    `json:"account_id"`
	IsRead     bool      `json:"is_read"`
}

// EmailCache stores cached emails for all accounts.
type EmailCache struct {
	Emails    []CachedEmail `json:"emails"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// cacheFile returns the full path to the email cache file.
func cacheFile() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "email_cache.json"), nil
}

// SaveEmailCache saves emails to the cache file.
func SaveEmailCache(cache *EmailCache) error {
	path, err := cacheFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	cache.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return SecureWriteFile(path, data, 0600)
}

// LoadEmailCache loads emails from the cache file.
func LoadEmailCache() (*EmailCache, error) {
	path, err := cacheFile()
	if err != nil {
		return nil, err
	}
	data, err := SecureReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache EmailCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

// HasEmailCache checks if a cache file exists.
func HasEmailCache() bool {
	path, err := cacheFile()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// ClearEmailCache removes the cache file.
func ClearEmailCache() error {
	path, err := cacheFile()
	if err != nil {
		return err
	}
	return os.Remove(path)
}

func removeAccountFromEmailCache(accountID string) error {
	cache, err := LoadEmailCache()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	filtered := cache.Emails[:0]
	for _, email := range cache.Emails {
		if email.AccountID != accountID {
			filtered = append(filtered, email)
		}
	}
	if len(filtered) == len(cache.Emails) {
		return nil
	}
	cache.Emails = filtered
	return SaveEmailCache(cache)
}

// --- Contacts Cache ---

const legacyContactUsageKey = "__legacy__"

// ContactUsage stores per-account contact usage metadata.
type ContactUsage struct {
	LastUsed time.Time `json:"last_used"`
	UseCount int       `json:"use_count"`
}

// Contact stores a contact's name, email address, and per-account usage.
//
// For regular contacts, Email holds a single address and Addresses is empty.
// For mailing-list virtual contacts emitted by SearchContacts, Email is empty
// and Addresses holds the expanded list of recipients. Callers that need to
// distinguish the two cases should check len(Addresses) > 0.
type Contact struct {
	Name      string                  `json:"name"`
	Email     string                  `json:"email"`
	Addresses []string                `json:"addresses,omitempty"`
	Usage     map[string]ContactUsage `json:"usage_by_account"`
}

// UnmarshalJSON accepts both the current usage_by_account format and the
// legacy last_used/use_count fields so old contacts can be migrated.
func (c *Contact) UnmarshalJSON(data []byte) error {
	type contactAlias Contact
	aux := struct {
		*contactAlias
		LastUsed time.Time `json:"last_used"`
		UseCount int       `json:"use_count"`
	}{
		contactAlias: (*contactAlias)(c),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if c.Usage == nil {
		c.Usage = make(map[string]ContactUsage)
	}
	if len(c.Usage) == 0 && (!aux.LastUsed.IsZero() || aux.UseCount > 0) {
		c.Usage[legacyContactUsageKey] = ContactUsage{
			LastUsed: aux.LastUsed,
			UseCount: aux.UseCount,
		}
	}
	return nil
}

// ContactsCache stores all known contacts.
type ContactsCache struct {
	Contacts  []Contact `json:"contacts"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetContactsCachePath returns the full path to the contacts cache file.
func GetContactsCachePath() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "contacts.json"), nil
}

// SaveContactsCache saves contacts to the cache file.
func SaveContactsCache(cache *ContactsCache) error {
	path, err := GetContactsCachePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	for i := range cache.Contacts {
		if cache.Contacts[i].Usage == nil {
			cache.Contacts[i].Usage = make(map[string]ContactUsage)
		}
	}
	cache.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return SecureWriteFile(path, data, 0600)
}

// LoadContactsCache loads contacts from the cache file.
func LoadContactsCache() (*ContactsCache, error) {
	path, err := GetContactsCachePath()
	if err != nil {
		return nil, err
	}
	data, err := SecureReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache ContactsCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

func normalizeContactEmail(email string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(email), ",<>"))
}

// AddContact adds or updates a global contact in the cache.
func AddContact(name, email string) error {
	return AddContactForAccount(name, email, "")
}

// AddContactForAccount adds or updates a contact in the cache for an account.
func AddContactForAccount(name, email, accountID string) error {
	if email == "" {
		return nil
	}

	email = normalizeContactEmail(email)
	name = strings.TrimSpace(name)

	cache, err := LoadContactsCache()
	if err != nil {
		cache = &ContactsCache{Contacts: []Contact{}}
	}

	// Check if contact exists
	found := false
	for i, c := range cache.Contacts {
		if strings.EqualFold(c.Email, email) {
			// Normalize the stored email to a canonical lowercase form.
			cache.Contacts[i].Email = email
			if cache.Contacts[i].Usage == nil {
				cache.Contacts[i].Usage = make(map[string]ContactUsage)
			}
			usage := cache.Contacts[i].Usage[accountID]
			usage.UseCount++
			usage.LastUsed = time.Now()
			cache.Contacts[i].Usage[accountID] = usage
			// Update name if we have a better one
			if name != "" && (c.Name == "" || c.Name == email) {
				cache.Contacts[i].Name = name
			}
			found = true
			break
		}
	}

	if !found {
		cache.Contacts = append(cache.Contacts, Contact{
			Name:  name,
			Email: email,
			Usage: map[string]ContactUsage{
				accountID: {
					LastUsed: time.Now(),
					UseCount: 1,
				},
			},
		})
	}

	return SaveContactsCache(cache)
}

func contactUsageForAccount(c Contact, accountID string) (ContactUsage, bool) {
	if len(c.Usage) == 0 {
		return ContactUsage{}, accountID == ""
	}
	if accountID != "" {
		if usage, ok := c.Usage[legacyContactUsageKey]; ok {
			return usage, true
		}
		usage, ok := c.Usage[accountID]
		return usage, ok
	}
	var aggregate ContactUsage
	for _, usage := range c.Usage {
		aggregate.UseCount += usage.UseCount
		if usage.LastUsed.After(aggregate.LastUsed) {
			aggregate.LastUsed = usage.LastUsed
		}
	}
	return aggregate, true
}

// ContactAggregateUsage returns a contact's total usage across accounts.
func ContactAggregateUsage(c Contact) ContactUsage {
	usage, _ := contactUsageForAccount(c, "")
	return usage
}

// SearchContacts searches for contacts matching the query across all accounts.
func SearchContacts(query string) []Contact {
	return SearchContactsForAccount(query, "")
}

// SearchContactsForAccount searches for contacts matching the query for an account.
func SearchContactsForAccount(query, accountID string) []Contact {
	cache, err := LoadContactsCache()
	if err != nil {
		return nil
	}

	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return nil
	}

	var matches []Contact

	// Add mailing lists to matches if they match the query
	cfg, err := LoadConfig()
	if err == nil {
		for _, list := range cfg.MailingLists {
			if strings.Contains(strings.ToLower(list.Name), query) {
				// Convert mailing list to a virtual contact. Addresses are
				// stored in a dedicated slice so the Email field keeps its
				// single-address invariant -- avoids corruption by
				// normalizeContactEmail and exact-match lookups in callers.
				addresses := append([]string(nil), list.Addresses...)
				matches = append(matches, Contact{
					Name:      list.Name,
					Addresses: addresses,
					Usage: map[string]ContactUsage{
						accountID: {
							UseCount: 9999, // Ensure lists appear at the top
							LastUsed: time.Now(),
						},
					},
				})
			}
		}
	}

	for _, c := range cache.Contacts {
		if strings.Contains(strings.ToLower(c.Email), query) ||
			strings.Contains(strings.ToLower(c.Name), query) {
			if _, ok := contactUsageForAccount(c, accountID); ok {
				matches = append(matches, c)
			}
		}
	}

	// Sort by use count (most used first), then by last used
	sort.Slice(matches, func(i, j int) bool {
		left, _ := contactUsageForAccount(matches[i], accountID)
		right, _ := contactUsageForAccount(matches[j], accountID)
		if left.UseCount != right.UseCount {
			return left.UseCount > right.UseCount
		}
		return left.LastUsed.After(right.LastUsed)
	})

	// Limit to 5 suggestions
	if len(matches) > 5 {
		matches = matches[:5]
	}

	return matches
}

// MigrateContactsCacheUsage expands legacy global contact usage to all accounts.
func MigrateContactsCacheUsage(accountIDs []string) error {
	cache, err := LoadContactsCache()
	if err != nil {
		return err
	}

	changed := false
	for i := range cache.Contacts {
		if cache.Contacts[i].Usage == nil {
			cache.Contacts[i].Usage = make(map[string]ContactUsage)
			changed = true
		}
		legacyUsage, hasLegacy := cache.Contacts[i].Usage[legacyContactUsageKey]
		if !hasLegacy {
			continue
		}
		delete(cache.Contacts[i].Usage, legacyContactUsageKey)
		for _, accountID := range accountIDs {
			if accountID == "" {
				continue
			}
			if _, ok := cache.Contacts[i].Usage[accountID]; !ok {
				cache.Contacts[i].Usage[accountID] = legacyUsage
			}
		}
		changed = true
	}
	if !changed {
		return nil
	}
	return SaveContactsCache(cache)
}

func removeAccountFromContactsCache(accountID string) error {
	cache, err := LoadContactsCache()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	changed := false
	filtered := cache.Contacts[:0]
	for _, contact := range cache.Contacts {
		if _, ok := contact.Usage[accountID]; ok {
			delete(contact.Usage, accountID)
			changed = true
		}
		if len(contact.Usage) > 0 {
			filtered = append(filtered, contact)
		} else {
			changed = true
		}
	}
	if !changed {
		return nil
	}
	cache.Contacts = filtered
	return SaveContactsCache(cache)
}

// --- Drafts Cache ---

// Draft stores a saved email draft.
type Draft struct {
	ID              string    `json:"id"`
	To              string    `json:"to"`
	Cc              string    `json:"cc,omitempty"`
	Bcc             string    `json:"bcc,omitempty"`
	Subject         string    `json:"subject"`
	Body            string    `json:"body"`
	AttachmentPaths []string  `json:"attachment_paths,omitempty"`
	AccountID       string    `json:"account_id"`
	FromOverride    string    `json:"from_override,omitempty"`
	InReplyTo       string    `json:"in_reply_to,omitempty"`
	References      []string  `json:"references,omitempty"`
	QuotedText      string    `json:"quoted_text,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DraftsCache stores all saved drafts.
type DraftsCache struct {
	Drafts    []Draft   `json:"drafts"`
	UpdatedAt time.Time `json:"updated_at"`
}

// draftsFile returns the full path to the drafts cache file.
func draftsFile() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "drafts.json"), nil
}

// SaveDraftsCache saves drafts to the cache file.
func SaveDraftsCache(cache *DraftsCache) error {
	path, err := draftsFile()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	cache.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return SecureWriteFile(path, data, 0600)
}

// LoadDraftsCache loads drafts from the cache file.
func LoadDraftsCache() (*DraftsCache, error) {
	path, err := draftsFile()
	if err != nil {
		return nil, err
	}
	data, err := SecureReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache DraftsCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

// SaveDraft saves or updates a draft.
func SaveDraft(draft Draft) error {
	cache, err := LoadDraftsCache()
	if err != nil {
		cache = &DraftsCache{Drafts: []Draft{}}
	}

	draft.UpdatedAt = time.Now()

	// Check if draft exists (update) or is new
	found := false
	for i, d := range cache.Drafts {
		if d.ID == draft.ID {
			cache.Drafts[i] = draft
			found = true
			break
		}
	}

	if !found {
		if draft.CreatedAt.IsZero() {
			draft.CreatedAt = time.Now()
		}
		cache.Drafts = append(cache.Drafts, draft)
	}

	return SaveDraftsCache(cache)
}

// DeleteDraft removes a draft by ID.
func DeleteDraft(id string) error {
	cache, err := LoadDraftsCache()
	if err != nil {
		return err
	}

	var filtered []Draft
	for _, d := range cache.Drafts {
		if d.ID != id {
			filtered = append(filtered, d)
		}
	}
	cache.Drafts = filtered

	return SaveDraftsCache(cache)
}

// GetDraft retrieves a draft by ID.
func GetDraft(id string) *Draft {
	cache, err := LoadDraftsCache()
	if err != nil {
		return nil
	}

	for _, d := range cache.Drafts {
		if d.ID == id {
			return &d
		}
	}
	return nil
}

// GetAllDrafts retrieves all drafts sorted by update time (newest first).
func GetAllDrafts() []Draft {
	cache, err := LoadDraftsCache()
	if err != nil {
		return nil
	}

	drafts := cache.Drafts
	sort.Slice(drafts, func(i, j int) bool {
		return drafts[i].UpdatedAt.After(drafts[j].UpdatedAt)
	})

	return drafts
}

// HasDrafts checks if there are any saved drafts.
func HasDrafts() bool {
	cache, err := LoadDraftsCache()
	if err != nil {
		return false
	}
	return len(cache.Drafts) > 0
}

func removeAccountFromDraftsCache(accountID string) error {
	cache, err := LoadDraftsCache()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	filtered := cache.Drafts[:0]
	for _, draft := range cache.Drafts {
		if draft.AccountID != accountID {
			filtered = append(filtered, draft)
		}
	}
	if len(filtered) == len(cache.Drafts) {
		return nil
	}
	cache.Drafts = filtered
	return SaveDraftsCache(cache)
}

// --- Email Body Cache ---

// CachedAttachment stores attachment metadata (not the binary data).
type CachedAttachment struct {
	Filename         string `json:"filename"`
	PartID           string `json:"part_id"`
	Encoding         string `json:"encoding,omitempty"`
	MIMEType         string `json:"mime_type,omitempty"`
	ContentID        string `json:"content_id,omitempty"`
	Inline           bool   `json:"inline,omitempty"`
	IsSMIMESignature bool   `json:"is_smime_signature,omitempty"`
	SMIMEVerified    bool   `json:"smime_verified,omitempty"`
	IsSMIMEEncrypted bool   `json:"is_smime_encrypted,omitempty"`
	IsCalendarInvite bool   `json:"is_calendar_invite,omitempty"`
	CalendarData     []byte `json:"calendar_data,omitempty"` // Raw .ics data for calendar invites
}

// CachedEmailBody stores the body and attachment metadata for a single email.
type CachedEmailBody struct {
	UID            uint32             `json:"uid"`
	AccountID      string             `json:"account_id"`
	Body           string             `json:"body"`
	BodyMIMEType   string             `json:"body_mime_type,omitempty"` // empty for cache rows written before MIME-type tracking; renderer falls back to legacy markdown→HTML pre-pass
	Attachments    []CachedAttachment `json:"attachments,omitempty"`
	CachedAt       time.Time          `json:"cached_at"`
	LastAccessedAt time.Time          `json:"last_accessed_at"`
	SizeBytes      int                `json:"size_bytes"`
}

// EmailBodyCache stores cached email bodies for a folder.
type EmailBodyCache struct {
	FolderName string            `json:"folder_name"`
	Bodies     []CachedEmailBody `json:"bodies"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// bodyCacheDir returns the directory for body cache files.
func bodyCacheDir() (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "email_bodies"), nil
}

// bodyBacheFile returns the file path for a folder's body cache.
func bodyCacheFile(folderName string) (string, error) {
	dir, err := bodyCacheDir()
	if err != nil {
		return "", err
	}
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_").Replace(folderName)
	return filepath.Join(dir, safe+".json"), nil
}

// LoadEmailBodyCache loads the body cache for a folder.
func LoadEmailBodyCache(folderName string) (*EmailBodyCache, error) {
	path, err := bodyCacheFile(folderName)
	if err != nil {
		return nil, err
	}
	data, err := SecureReadFile(path)
	if err != nil {
		return nil, err
	}
	var cache EmailBodyCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}
	return &cache, nil
}

// saveEmailBodyCache writes the body cache for a folder.
func saveEmailBodyCache(cache *EmailBodyCache) error {
	path, err := bodyCacheFile(cache.FolderName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	cache.UpdatedAt = time.Now()
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}
	return SecureWriteFile(path, data, 0600)
}

// GetCachedEmailBody returns the cached body for a specific email, or nil if not cached.
// LastAccessedAt is updated by SaveEmailBody, not here -- a read should not
// mutate cache state.
func GetCachedEmailBody(folderName string, uid uint32, accountID string, threshold int) *CachedEmailBody {
	lru := GetLRUInstance(threshold)

	if node := lru.Get(folderName, uid, accountID); node != nil {
		return node.Body
	}

	return nil
}

func calculateEmailBodySize(body *CachedEmailBody) int {
	size := len(body.Body)
	for _, att := range body.Attachments {
		size += len(att.Filename)
		size += len(att.PartID)
		size += len(att.Encoding)
		size += len(att.MIMEType)
		size += len(att.ContentID)
		size += len(att.CalendarData)
	}
	return size
}

// SaveEmailBody saves or updates a cached email body for a folder.
func SaveEmailBody(folderName string, body CachedEmailBody, threshold int) error {
	body.CachedAt = time.Now()
	body.SizeBytes = calculateEmailBodySize(&body)

	lru := GetLRUInstance(threshold)
	lru.Put(folderName, body.UID, body.AccountID, &body)

	return nil
}

// PruneEmailBodyCache removes cached bodies for emails that are no longer in the folder.
// validUIDs is a map of UID -> AccountID for emails still present.
func PruneEmailBodyCache(folderName string, validUIDs map[uint32]string, threshold int) error {
	cache, err := LoadEmailBodyCache(folderName)

	if err != nil {
		return err
	}

	lru := GetLRUInstance(threshold)

	var kept []CachedEmailBody
	for _, b := range cache.Bodies {
		if accID, ok := validUIDs[b.UID]; ok && accID == b.AccountID {
			kept = append(kept, b)
		} else {
			lru.Delete(folderName, b.UID, b.AccountID)
		}
	}

	if len(kept) == len(cache.Bodies) {
		return nil
	}

	cache.Bodies = kept
	return saveEmailBodyCache(cache)
}

func removeAccountFromEmailBodyCaches(accountID string) error {
	dir, err := bodyCacheDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var errs []error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := SecureReadFile(path)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var cache EmailBodyCache
		if err := json.Unmarshal(data, &cache); err != nil {
			errs = append(errs, err)
			continue
		}

		filtered := cache.Bodies[:0]
		for _, body := range cache.Bodies {
			if body.AccountID != accountID {
				filtered = append(filtered, body)
			}
		}
		if len(filtered) == len(cache.Bodies) {
			continue
		}
		if len(filtered) == 0 {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				errs = append(errs, err)
			}
			continue
		}
		cache.Bodies = filtered
		cache.UpdatedAt = time.Now()
		data, err = json.Marshal(cache)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := SecureWriteFile(path, data, 0600); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CleanupAccountCache removes cached data associated with an account.
func CleanupAccountCache(accountID string) error {
	if accountID == "" {
		return nil
	}

	return errors.Join(
		removeAccountFromEmailCache(accountID),
		removeAccountFromFolderCache(accountID),
		removeAccountFromFolderEmailCaches(accountID),
		removeAccountFromEmailBodyCaches(accountID),
		removeAccountFromContactsCache(accountID),
		removeAccountFromDraftsCache(accountID),
	)
}
