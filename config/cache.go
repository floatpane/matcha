package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// CachedEmail stores essential email data for caching.
type CachedEmail struct {
	UID       uint32    `json:"uid"`
	From      string    `json:"from"`
	To        []string  `json:"to"`
	Subject   string    `json:"subject"`
	Date      time.Time `json:"date"`
	MessageID string    `json:"message_id"`
	AccountID string    `json:"account_id"`
	IsRead    bool      `json:"is_read"`
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

// --- Contacts Cache ---

// Contact stores a contact's name and email address.
type Contact struct {
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	LastUsed time.Time `json:"last_used"`
	UseCount int       `json:"use_count"`
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

// AddContact adds or updates a contact in the cache.
func AddContact(name, email string) error {
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
			cache.Contacts[i].UseCount++
			cache.Contacts[i].LastUsed = time.Now()
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
			Name:     name,
			Email:    email,
			LastUsed: time.Now(),
			UseCount: 1,
		})
	}

	return SaveContactsCache(cache)
}

// SearchContacts searches for contacts matching the query.
func SearchContacts(query string) []Contact {
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
				// Convert mailing list to a virtual contact
				matches = append(matches, Contact{
					Name:     list.Name,
					Email:    strings.Join(list.Addresses, ", "),
					UseCount: 9999, // Ensure lists appear at the top
					LastUsed: time.Now(),
				})
			}
		}
	}

	for _, c := range cache.Contacts {
		if strings.Contains(strings.ToLower(c.Email), query) ||
			strings.Contains(strings.ToLower(c.Name), query) {
			matches = append(matches, c)
		}
	}

	// Sort by use count (most used first), then by last used
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].UseCount != matches[j].UseCount {
			return matches[i].UseCount > matches[j].UseCount
		}
		return matches[i].LastUsed.After(matches[j].LastUsed)
	})

	// Limit to 5 suggestions
	if len(matches) > 5 {
		matches = matches[:5]
	}

	return matches
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
		return nil // No cache, nothing to delete
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
	UID         uint32             `json:"uid"`
	AccountID   string             `json:"account_id"`
	Body        string             `json:"body"`
	Attachments []CachedAttachment `json:"attachments,omitempty"`
	CachedAt    time.Time          `json:"cached_at"`
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
func GetCachedEmailBody(folderName string, uid uint32, accountID string) *CachedEmailBody {
	cache, err := LoadEmailBodyCache(folderName)
	if err != nil {
		return nil
	}
	for _, b := range cache.Bodies {
		if b.UID == uid && b.AccountID == accountID {
			return &b
		}
	}
	return nil
}

// SaveEmailBody saves or updates a cached email body for a folder.
func SaveEmailBody(folderName string, body CachedEmailBody) error {
	cache, err := LoadEmailBodyCache(folderName)
	if err != nil {
		cache = &EmailBodyCache{FolderName: folderName}
	}

	body.CachedAt = time.Now()

	// Replace existing or append
	found := false
	for i, b := range cache.Bodies {
		if b.UID == body.UID && b.AccountID == body.AccountID {
			cache.Bodies[i] = body
			found = true
			break
		}
	}
	if !found {
		cache.Bodies = append(cache.Bodies, body)
	}

	return saveEmailBodyCache(cache)
}

// PruneEmailBodyCache removes cached bodies for emails that are no longer in the folder.
// validUIDs is a map of UID -> AccountID for emails still present.
func PruneEmailBodyCache(folderName string, validUIDs map[uint32]string) error {
	cache, err := LoadEmailBodyCache(folderName)
	if err != nil {
		return nil // No cache to prune
	}

	var kept []CachedEmailBody
	for _, b := range cache.Bodies {
		if accID, ok := validUIDs[b.UID]; ok && accID == b.AccountID {
			kept = append(kept, b)
		}
	}

	if len(kept) == len(cache.Bodies) {
		return nil // Nothing pruned
	}

	cache.Bodies = kept
	return saveEmailBodyCache(cache)
}
