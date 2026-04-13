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
	To              string
	Cc              string // Cc recipient(s)
	Bcc             string // Bcc recipient(s)
	Subject         string
	Body            string
	AttachmentPaths []string
	InReplyTo       string
	References      []string
	AccountID       string // ID of the account to send from
	QuotedText      string // Hidden quoted text appended when sending
	Signature       string // Signature to append to email body
	SignSMIME       bool   // Whether to sign the email using S/MIME
	EncryptSMIME    bool   // Whether to encrypt the email using S/MIME
	SignPGP         bool   // Whether to sign the email using PGP
}

type Credentials struct {
	Provider     string
	Name         string
	Host         string // Host (this was the previous "Email Address" field in the UI)
	FetchEmail   string // Single email address to fetch messages for. If empty, code should default this to Host when creating the account.
	SendAsEmail  string // Optional From header email. If empty, sending falls back to FetchEmail, then Host.
	Password     string
	IMAPServer   string
	IMAPPort     int
	SMTPServer   string
	SMTPPort     int
	AuthMethod   string // "password" or "oauth2"
	Protocol     string // "imap" (default), "jmap", or "pop3"
	JMAPEndpoint string // JMAP session URL
	POP3Server   string // POP3 server hostname
	POP3Port     int    // POP3 server port
}

// StartOAuth2Msg is sent when the user requests OAuth2 authorization for a Gmail account.
type StartOAuth2Msg struct {
	Email string
}

// OAuth2CompleteMsg is sent when OAuth2 authorization completes.
type OAuth2CompleteMsg struct {
	Email string
	Err   error
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
	Limit     uint32
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

type ForwardEmailMsg struct {
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

// GoToAddMailingListMsg signals navigation to the add mailing list screen.
type GoToAddMailingListMsg struct{}

// GoToEditAccountMsg signals navigation to edit an existing account.
type GoToEditAccountMsg struct {
	AccountID    string
	Provider     string
	Name         string
	Email        string
	FetchEmail   string
	SendAsEmail  string
	IMAPServer   string
	IMAPPort     int
	SMTPServer   string
	SMTPPort     int
	Protocol     string
	JMAPEndpoint string
	POP3Server   string
	POP3Port     int
}

// GoToEditMailingListMsg signals navigation to edit an existing mailing list.
type GoToEditMailingListMsg struct {
	Index     int
	Name      string
	Addresses string
}

// SaveMailingListMsg signals that a new or edited mailing list should be saved.
type SaveMailingListMsg struct {
	Name      string
	Addresses string
	EditIndex int // -1 means new, >= 0 means editing existing
}

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
	Mailbox    MailboxKind
	Counts     map[string]int
	FolderName string
}

// --- Folder Messages ---

// FoldersFetchedMsg signals that IMAP folders have been fetched for all accounts.
type FoldersFetchedMsg struct {
	FoldersByAccount map[string][]fetcher.Folder // accountID -> folders
	MergedFolders    []fetcher.Folder            // unique folders across all accounts
}

// SwitchFolderMsg signals switching to a different IMAP folder.
type SwitchFolderMsg struct {
	FolderName string
	AccountID  string
}

// FolderEmailsFetchedMsg signals that emails from a folder have been fetched.
type FolderEmailsFetchedMsg struct {
	Emails     []fetcher.Email
	AccountID  string
	FolderName string
}

// FolderEmailsAppendedMsg signals that more emails from a folder have been fetched (pagination).
type FolderEmailsAppendedMsg struct {
	Emails     []fetcher.Email
	AccountID  string
	FolderName string
}

// MoveEmailMsg signals a request to show the move-to-folder picker.
type MoveEmailMsg struct {
	UID          uint32
	AccountID    string
	SourceFolder string
}

// MoveEmailToFolderMsg signals that an email should be moved to a folder.
type MoveEmailToFolderMsg struct {
	UID          uint32
	AccountID    string
	SourceFolder string
	DestFolder   string
}

// EmailMovedMsg signals that an email was moved to a folder.
type EmailMovedMsg struct {
	UID          uint32
	AccountID    string
	SourceFolder string
	DestFolder   string
	Err          error
}

// MarkEmailAsReadMsg signals that an email should be marked as read on the server.
type MarkEmailAsReadMsg struct {
	UID        uint32
	AccountID  string
	FolderName string
}

// EmailMarkedReadMsg signals that an email was marked as read.
type EmailMarkedReadMsg struct {
	UID       uint32
	AccountID string
	Err       error
}

// FetchFolderMoreEmailsMsg signals a request to fetch more emails from a folder (pagination).
type FetchFolderMoreEmailsMsg struct {
	Offset     uint32
	AccountID  string
	FolderName string
	Limit      uint32
}

// --- External Editor Messages ---

// OpenEditorMsg signals that the composer body should be opened in $EDITOR.
type OpenEditorMsg struct{}

// EditorFinishedMsg signals that the external editor has closed.
type EditorFinishedMsg struct {
	Body string
	Err  error
}

// --- IDLE Messages ---

// IdleNewMailMsg signals that IMAP IDLE detected new mail for an account/folder.
type IdleNewMailMsg struct {
	AccountID  string
	FolderName string
}

// --- Plugin Messages ---

// PluginNotifyMsg signals that a plugin wants to show a notification.
type PluginNotifyMsg struct {
	Message  string
	Duration float64 // Duration in seconds (default 2)
}

// PluginKeyBinding describes a plugin-registered keyboard shortcut for display in the help bar.
type PluginKeyBinding struct {
	Key         string
	Description string
}

// PluginPromptSubmitMsg signals that the user submitted a plugin prompt input.
type PluginPromptSubmitMsg struct {
	Value string
}

// PluginPromptCancelMsg signals that the user cancelled a plugin prompt input.
type PluginPromptCancelMsg struct{}

// GoToMarketplaceMsg signals navigation to the plugin marketplace.
type GoToMarketplaceMsg struct{}

// PasswordVerifiedMsg signals that the encryption password was verified (or failed).
type PasswordVerifiedMsg struct {
	Key []byte // The derived encryption key (nil on failure)
	Err error  // Non-nil if verification failed
}

// SecureModeEnabledMsg signals that encryption was enabled from settings.
type SecureModeEnabledMsg struct {
	Err error
}

// SecureModeDisabledMsg signals that encryption was disabled from settings.
type SecureModeDisabledMsg struct {
	Err error
}
