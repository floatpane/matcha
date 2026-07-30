package daemonrpc

import udsrpc "github.com/floatpane/go-uds-jsonrpc"

// Wire-level message types and the discriminating decoder live in the shared
// go-uds-jsonrpc library. They are aliased here so matcha code keeps using the
// daemonrpc.* names while sharing a single implementation with the daemon's
// transport layer.
type (
	Request  = udsrpc.Request
	Response = udsrpc.Response
	Event    = udsrpc.Event
	Error    = udsrpc.Error
	Message  = udsrpc.Message
)

// DecodeMessage discriminates a raw JSON object into a Request, Response, or
// Event.
var DecodeMessage = udsrpc.DecodeMessage

// Standard error codes, re-exported from the shared library.
const (
	ErrCodeParse         = udsrpc.ErrCodeParse
	ErrCodeInvalidReq    = udsrpc.ErrCodeInvalidReq
	ErrCodeInvalidParams = udsrpc.ErrCodeInvalidParams
	ErrCodeNotFound      = udsrpc.ErrCodeNotFound
	ErrCodeInternal      = udsrpc.ErrCodeInternal
)

// RPC method names.
const (
	MethodPing             = "Ping"
	MethodGetStatus        = "GetStatus"
	MethodGetAccounts      = "GetAccounts"
	MethodReloadConfig     = "ReloadConfig"
	MethodFetchEmails      = "FetchEmails"
	MethodFetchEmailBody   = "FetchEmailBody"
	MethodSendEmail        = "SendEmail"
	MethodDeleteEmails     = "DeleteEmails"
	MethodArchiveEmails    = "ArchiveEmails"
	MethodMoveEmails       = "MoveEmails"
	MethodMarkRead         = "MarkRead"
	MethodFetchFolders     = "FetchFolders"
	MethodRefreshFolder    = "RefreshFolder"
	MethodSubscribe        = "Subscribe"
	MethodUnsubscribe      = "Unsubscribe"
	MethodSendRSVP         = "SendRSVP"
	MethodGetCachedEmails  = "GetCachedEmails"
	MethodGetCachedBody    = "GetCachedBody"
	MethodExportContacts   = "ExportContacts"
	MethodQueueEmail       = "QueueEmail"
	MethodCancelEmail      = "CancelEmail"
	MethodAddGmailLabel    = "AddGmailLabel"
	MethodRemoveGmailLabel = "RemoveGmailLabel"
)

// Event type names.
const (
	EventNewMail        = "NewMail"
	EventSyncStarted    = "SyncStarted"
	EventSyncComplete   = "SyncComplete"
	EventSyncError      = "SyncError"
	EventEmailsUpdated  = "EmailsUpdated"
	EventConfigReloaded = "ConfigReloaded"
)

// Param/result types for RPC methods.

type PingResult struct {
	Pong bool `json:"pong"`
}

type StatusResult struct {
	Running  bool     `json:"running"`
	Uptime   int64    `json:"uptime_seconds"`
	Accounts []string `json:"accounts"`
	PID      int      `json:"pid"`
}

type AccountInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Protocol string `json:"protocol"`
}

type FetchEmailsParams struct {
	AccountID string `json:"account_id"`
	Folder    string `json:"folder"`
	Limit     uint32 `json:"limit"`
	Offset    uint32 `json:"offset"`
}

type FetchEmailBodyParams struct {
	AccountID string `json:"account_id"`
	Folder    string `json:"folder"`
	UID       uint32 `json:"uid"`
}

type QueueEmailParams struct {
	Email        SendEmailParams `json:"email"`
	DelaySeconds int             `json:"delay_seconds"`
}

type QueueEmailResult struct {
	JobID string `json:"job_id"`
}

type CancelEmailParams struct {
	JobID string `json:"job_id"`
}

type FetchEmailBodyResult struct {
	Body         string           `json:"body"`
	BodyMIMEType string           `json:"body_mime_type,omitempty"`
	Attachments  []AttachmentInfo `json:"attachments"`
}

type AttachmentInfo struct {
	Filename         string `json:"filename"`
	PartID           string `json:"part_id"`
	Encoding         string `json:"encoding"`
	MIMEType         string `json:"mime_type"`
	IsCalendarInvite bool   `json:"is_calendar_invite,omitempty"`
	CalendarData     []byte `json:"calendar_data,omitempty"`
}

type SendEmailParams struct {
	AccountID    string            `json:"account_id"`
	To           []string          `json:"to"`
	Cc           []string          `json:"cc,omitempty"`
	Bcc          []string          `json:"bcc,omitempty"`
	Subject      string            `json:"subject"`
	Body         string            `json:"body"`
	HTMLBody     string            `json:"html_body,omitempty"`
	Images       map[string][]byte `json:"images,omitempty"`
	Attachments  map[string][]byte `json:"attachments,omitempty"`
	InReplyTo    string            `json:"in_reply_to,omitempty"`
	References   []string          `json:"references,omitempty"`
	SignSMIME    bool              `json:"sign_smime,omitempty"`
	EncryptSMIME bool              `json:"encrypt_smime,omitempty"`
	SignPGP      bool              `json:"sign_pgp,omitempty"`
	EncryptPGP   bool              `json:"encrypt_pgp,omitempty"`
	PrebuiltRaw  []byte            `json:"prebuilt_raw,omitempty"`
}

type DeleteEmailsParams struct {
	AccountID string   `json:"account_id"`
	Folder    string   `json:"folder"`
	UIDs      []uint32 `json:"uids"`
}

type ArchiveEmailsParams struct {
	AccountID string   `json:"account_id"`
	Folder    string   `json:"folder"`
	UIDs      []uint32 `json:"uids"`
}

type MoveEmailsParams struct {
	AccountID    string   `json:"account_id"`
	UIDs         []uint32 `json:"uids"`
	SourceFolder string   `json:"source_folder"`
	DestFolder   string   `json:"dest_folder"`
}

type MarkReadParams struct {
	AccountID string   `json:"account_id"`
	Folder    string   `json:"folder"`
	UIDs      []uint32 `json:"uids"`
	Read      bool     `json:"read"`
}

type FetchFoldersParams struct {
	AccountID string `json:"account_id"`
}

type RefreshFolderParams struct {
	AccountID string `json:"account_id"`
	Folder    string `json:"folder"`
}

type SubscribeParams struct {
	AccountID string `json:"account_id"`
	Folder    string `json:"folder"`
}

type UnsubscribeParams struct {
	AccountID string `json:"account_id"`
	Folder    string `json:"folder"`
}

type SendRSVPParams struct {
	AccountID   string   `json:"account_id"`
	OriginalICS []byte   `json:"original_ics"`
	Response    string   `json:"response"`
	InReplyTo   string   `json:"in_reply_to,omitempty"`
	References  []string `json:"references,omitempty"`
}

type GetCachedEmailsParams struct {
	Folder string `json:"folder"`
}

type GetCachedBodyParams struct {
	Folder    string `json:"folder"`
	UID       uint32 `json:"uid"`
	AccountID string `json:"account_id"`
}

type ExportContactsParams struct {
	Format string `json:"format"` // "json" or "csv"
}

type AddGmailLabelParams struct {
	AccountID string `json:"account_id"`
	Folder    string `json:"folder"`
	UID       uint32 `json:"uid"`
	Label     string `json:"label"`
}

type RemoveGmailLabelParams struct {
	AccountID string `json:"account_id"`
	Folder    string `json:"folder"`
	UID       uint32 `json:"uid"`
	Label     string `json:"label"`
}

// Event data types.

type NewMailEvent struct {
	AccountID string `json:"account_id"`
	Folder    string `json:"folder"`
}

type SyncStartedEvent struct {
	AccountID string `json:"account_id"`
	Folder    string `json:"folder"`
}

type SyncCompleteEvent struct {
	AccountID  string `json:"account_id"`
	Folder     string `json:"folder"`
	EmailCount int    `json:"email_count"`
}

type SyncErrorEvent struct {
	AccountID string `json:"account_id"`
	Folder    string `json:"folder"`
	Error     string `json:"error"`
}
