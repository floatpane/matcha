// Package backend defines the Provider interface for multi-protocol email support.
package backend

import (
	"context"
	"errors"
	"time"
)

// ErrNotSupported is returned when a provider does not support an operation.
var ErrNotSupported = errors.New("operation not supported by this provider")

// Provider is the unified interface that all email backends must implement.
type Provider interface {
	EmailReader
	EmailWriter
	EmailSender
	FolderManager
	Notifier
	Close() error
}

// EmailReader fetches emails and their content.
type EmailReader interface {
	FetchEmails(ctx context.Context, folder string, limit, offset uint32) ([]Email, error)
	FetchEmailBody(ctx context.Context, folder string, uid uint32) (string, []Attachment, error)
	FetchAttachment(ctx context.Context, folder string, uid uint32, partID, encoding string) ([]byte, error)
}

// EmailWriter modifies email state.
type EmailWriter interface {
	MarkAsRead(ctx context.Context, folder string, uid uint32) error
	DeleteEmail(ctx context.Context, folder string, uid uint32) error
	ArchiveEmail(ctx context.Context, folder string, uid uint32) error
	MoveEmail(ctx context.Context, uid uint32, srcFolder, dstFolder string) error

	// Batch operations
	DeleteEmails(ctx context.Context, folder string, uids []uint32) error
	ArchiveEmails(ctx context.Context, folder string, uids []uint32) error
	MoveEmails(ctx context.Context, uids []uint32, srcFolder, dstFolder string) error
}

// EmailSender sends outgoing email.
type EmailSender interface {
	SendEmail(ctx context.Context, msg *OutgoingEmail) error
}

// FolderManager lists folders/mailboxes.
type FolderManager interface {
	FetchFolders(ctx context.Context) ([]Folder, error)
}

// Notifier provides real-time notifications for new email.
type Notifier interface {
	Watch(ctx context.Context, folder string) (<-chan NotifyEvent, func(), error)
}

// CapabilityProvider optionally reports what a backend can do.
type CapabilityProvider interface {
	Capabilities() Capabilities
}

// Email represents a single email message.
type Email struct {
	UID         uint32
	From        string
	To          []string
	Subject     string
	Body        string
	Date        time.Time
	IsRead      bool
	MessageID   string
	References  []string
	Attachments []Attachment
	AccountID   string
}

// Attachment holds data for an email attachment.
type Attachment struct {
	Filename         string
	PartID           string
	Data             []byte
	Encoding         string
	MIMEType         string
	ContentID        string
	Inline           bool
	IsSMIMESignature bool
	SMIMEVerified    bool
	IsSMIMEEncrypted bool
	IsPGPSignature   bool
	PGPVerified      bool
	IsPGPEncrypted   bool
}

// Folder represents a mailbox/folder.
type Folder struct {
	Name       string
	Delimiter  string
	Attributes []string
}

// OutgoingEmail contains everything needed to send an email.
type OutgoingEmail struct {
	To           []string
	Cc           []string
	Bcc          []string
	Subject      string
	PlainBody    string
	HTMLBody     string
	Images       map[string][]byte
	Attachments  map[string][]byte
	InReplyTo    string
	References   []string
	SignSMIME    bool
	EncryptSMIME bool
	SignPGP      bool
	EncryptPGP   bool
}

// NotifyType indicates the kind of notification event.
type NotifyType int

const (
	NotifyNewEmail NotifyType = iota
	NotifyExpunge
	NotifyFlagChange
)

// NotifyEvent is emitted by Watch() when something changes in a mailbox.
type NotifyEvent struct {
	Type      NotifyType
	Folder    string
	AccountID string
}

// Capabilities describes what a backend supports.
type Capabilities struct {
	CanSend         bool
	CanMove         bool
	CanArchive      bool
	CanPush         bool
	CanSearchServer bool
	CanFetchFolders bool
	SupportsSMIME   bool
}
