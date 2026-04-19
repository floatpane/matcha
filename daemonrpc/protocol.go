package daemonrpc

import "encoding/json"

// Request from client to daemon. Has an ID for matching responses.
type Request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Response from daemon to client. Matched to request by ID.
type Response struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Event pushed from daemon to subscribed clients. No ID field.
type Event struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Error returned in a Response.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// Message is a union type for wire decoding. Exactly one of the
// fields will be populated based on the presence of "id" and "type".
type Message struct {
	Request  *Request
	Response *Response
	Event    *Event
}

// Discriminate: if "type" present → Event, if "method" present → Request, else → Response.
func DecodeMessage(raw json.RawMessage) (Message, error) {
	var probe struct {
		Type   string  `json:"type"`
		Method string  `json:"method"`
		ID     *uint64 `json:"id"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return Message{}, err
	}

	var m Message
	if probe.Type != "" {
		var ev Event
		if err := json.Unmarshal(raw, &ev); err != nil {
			return m, err
		}
		m.Event = &ev
	} else if probe.Method != "" {
		var req Request
		if err := json.Unmarshal(raw, &req); err != nil {
			return m, err
		}
		m.Request = &req
	} else {
		var resp Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			return m, err
		}
		m.Response = &resp
	}
	return m, nil
}

// Standard error codes.
const (
	ErrCodeParse      = -32700
	ErrCodeInvalidReq = -32600
	ErrCodeNotFound   = -32601
	ErrCodeInternal   = -32603
)

// RPC method names.
const (
	MethodPing            = "Ping"
	MethodGetStatus       = "GetStatus"
	MethodGetAccounts     = "GetAccounts"
	MethodReloadConfig    = "ReloadConfig"
	MethodFetchEmails     = "FetchEmails"
	MethodFetchEmailBody  = "FetchEmailBody"
	MethodSendEmail       = "SendEmail"
	MethodDeleteEmails    = "DeleteEmails"
	MethodArchiveEmails   = "ArchiveEmails"
	MethodMoveEmails      = "MoveEmails"
	MethodMarkRead        = "MarkRead"
	MethodFetchFolders    = "FetchFolders"
	MethodRefreshFolder   = "RefreshFolder"
	MethodSubscribe       = "Subscribe"
	MethodUnsubscribe     = "Unsubscribe"
	MethodSendRSVP        = "SendRSVP"
	MethodGetCachedEmails = "GetCachedEmails"
	MethodGetCachedBody   = "GetCachedBody"
	MethodExportContacts  = "ExportContacts"
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

type FetchEmailBodyResult struct {
	Body        string           `json:"body"`
	Attachments []AttachmentInfo `json:"attachments"`
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
	Attachments  map[string][]byte `json:"attachments,omitempty"`
	InReplyTo    string            `json:"in_reply_to,omitempty"`
	References   []string          `json:"references,omitempty"`
	SignSMIME    bool              `json:"sign_smime,omitempty"`
	EncryptSMIME bool              `json:"encrypt_smime,omitempty"`
	SignPGP      bool              `json:"sign_pgp,omitempty"`
	EncryptPGP   bool              `json:"encrypt_pgp,omitempty"`
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
