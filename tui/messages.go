package tui

import (
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
)

type MailboxKind string

const (
	MailboxInbox   MailboxKind = "inbox"
	MailboxSent    MailboxKind = "sent"
	MailboxTrash   MailboxKind = "trash"
	MailboxArchive MailboxKind = "archive"
)

type ViewEmailMsg struct {
	Index     int
	UID       uint32
	AccountID string
	Mailbox   MailboxKind
}

type SendEmailMsg struct {
	To             string
	Subject        string
	Body           string
	AttachmentPath string
	InReplyTo      string
	References     []string
	AccountID      string // ID of the account to send from
	QuotedText     string // Hidden quoted text appended when sending
	Signature      string // Signature to append to email body
}

type Credentials struct {
	Provider   string
	Name       string
	Host       string // Host (this was the previous \"Email Address\" field in the UI)
	FetchEmail string // Single email address to fetch messages for. If empty, code should default this to Host when creating the account.
	Password   string
	IMAPServer string
	IMAPPort   int
	SMTPServer string
	SMTPPort   int
}

type ChooseServiceMsg struct {
	Service string
}

type EmailResultMsg struct {
	Err error
}

type ClearStatusMsg struct{}

type EmailsFetchedMsg struct {
	Emails    []fetcher.Email
	AccountID string
	Mailbox   MailboxKind
}

type FetchErr error

type GoToInboxMsg struct{}

type GoToSentInboxMsg struct{}

type GoToSendMsg struct {
	To      string
	Subject string
	Body    string
}

type GoToSettingsMsg struct{}

type GoToTrashArchiveMsg struct{}

type GoToSignatureEditorMsg struct{}

type FetchMoreEmailsMsg struct {
	Offset    uint32
	AccountID string
	Mailbox   MailboxKind
}

type FetchingMoreEmailsMsg struct{}

type EmailsAppendedMsg struct {
	Emails    []fetcher.Email
	AccountID string
	Mailbox   MailboxKind
}

type ReplyToEmailMsg struct {
	Email fetcher.Email
}

type SetComposerCursorToStartMsg struct{}

type GoToFilePickerMsg struct{}

type FileSelectedMsg struct {
	Path string
}

type CancelFilePickerMsg struct{}

type DeleteEmailMsg struct {
	UID       uint32
	AccountID string
	Mailbox   MailboxKind
}

type ArchiveEmailMsg struct {
	UID       uint32
	AccountID string
	Mailbox   MailboxKind
}

type EmailActionDoneMsg struct {
	UID       uint32
	AccountID string
	Mailbox   MailboxKind
	Err       error
}

type GoToChoiceMenuMsg struct{}

type DownloadAttachmentMsg struct {
	Index     int
	Filename  string
	PartID    string
	Data      []byte
	AccountID string
	Encoding  string
	Mailbox   MailboxKind
}

type AttachmentDownloadedMsg struct {
	Path string
	Err  error
}

type RestoreViewMsg struct{}

type BackToInboxMsg struct{}

type BackToMailboxMsg struct {
	Mailbox MailboxKind
}

// --- Draft Messages ---

// DiscardDraftMsg signals that a draft should be cached.
type DiscardDraftMsg struct {
	ComposerState *Composer
}

type EmailBodyFetchedMsg struct {
	UID         uint32
	Body        string
	Attachments []fetcher.Attachment
	Err         error
	AccountID   string
	Mailbox     MailboxKind
}

// --- Multi-Account Messages ---

// GoToAddAccountMsg signals navigation to the add account screen.
type GoToAddAccountMsg struct{}

// AddAccountMsg signals that a new account should be added.
type AddAccountMsg struct {
	Credentials Credentials
}

// AccountAddedMsg signals that an account was successfully added.
type AccountAddedMsg struct {
	AccountID string
	Err       error
}

// DeleteAccountMsg signals that an account should be deleted.
type DeleteAccountMsg struct {
	AccountID string
}

// AccountDeletedMsg signals that an account was successfully deleted.
type AccountDeletedMsg struct {
	AccountID string
	Err       error
}

// SwitchAccountMsg signals switching to view a specific account's inbox.
type SwitchAccountMsg struct {
	AccountID string // Empty string means "ALL" accounts
}

// AllEmailsFetchedMsg signals that emails from all accounts have been fetched.
type AllEmailsFetchedMsg struct {
	EmailsByAccount map[string][]fetcher.Email
	Mailbox         MailboxKind
}

// SwitchFromAccountMsg signals changing the "From" account in composer.
type SwitchFromAccountMsg struct {
	AccountID string
}

// GoToAccountListMsg signals navigation to the account list in settings.
type GoToAccountListMsg struct{}

// --- Draft Messages (persisted) ---

// SaveDraftMsg signals that the current draft should be saved to disk.
type SaveDraftMsg struct {
	Draft config.Draft
}

// DraftSavedMsg signals that a draft was saved successfully.
type DraftSavedMsg struct {
	DraftID string
	Err     error
}

// LoadDraftsMsg signals a request to load all saved drafts.
type LoadDraftsMsg struct{}

// DraftsLoadedMsg signals that drafts were loaded from disk.
type DraftsLoadedMsg struct {
	Drafts []config.Draft
}

// OpenDraftMsg signals that a specific draft should be opened in the composer.
type OpenDraftMsg struct {
	Draft config.Draft
}

// DeleteDraftMsg signals that a draft should be deleted.
type DeleteSavedDraftMsg struct {
	DraftID string
}

// DraftDeletedMsg signals that a draft was deleted.
type DraftDeletedMsg struct {
	DraftID string
	Err     error
}

// GoToDraftsMsg signals navigation to the drafts list.
type GoToDraftsMsg struct{}

// --- Cache Messages ---

// CachedEmailsLoadedMsg signals that cached emails were loaded from disk.
type CachedEmailsLoadedMsg struct {
	Cache *config.EmailCache
}

// RefreshingEmailsMsg signals that a background refresh is in progress.
type RefreshingEmailsMsg struct {
	Mailbox MailboxKind
}

// EmailsRefreshedMsg signals that fresh emails have been fetched in the background.
type EmailsRefreshedMsg struct {
	EmailsByAccount map[string][]fetcher.Email
	Mailbox         MailboxKind
}

// RequestRefreshMsg signals a request to refresh emails from the server.
type RequestRefreshMsg struct {
	Mailbox MailboxKind
}
