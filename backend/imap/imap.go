// Package imap implements the backend.Provider interface by delegating
// to the existing fetcher and sender packages.
package imap

import (
	"context"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/sender"
)

func init() {
	backend.RegisterBackend("imap", func(account *config.Account) (backend.Provider, error) {
		return New(account)
	})
}

// Provider wraps the existing fetcher/sender packages behind the backend.Provider interface.
type Provider struct {
	account *config.Account
}

// New creates a new IMAP provider.
func New(account *config.Account) (*Provider, error) {
	return &Provider{account: account}, nil
}

func (p *Provider) FetchEmails(_ context.Context, folder string, limit, offset uint32) ([]backend.Email, error) {
	emails, err := fetcher.FetchMailboxEmails(p.account, folder, limit, offset)
	if err != nil {
		return nil, err
	}
	return toBackendEmails(emails), nil
}

func (p *Provider) FetchEmailBody(_ context.Context, folder string, uid uint32) (string, []backend.Attachment, error) {
	body, atts, err := fetcher.FetchEmailBodyFromMailbox(p.account, folder, uid)
	if err != nil {
		return "", nil, err
	}
	return body, toBackendAttachments(atts), nil
}

func (p *Provider) FetchAttachment(_ context.Context, folder string, uid uint32, partID, encoding string) ([]byte, error) {
	return fetcher.FetchAttachmentFromMailbox(p.account, folder, uid, partID, encoding)
}

func (p *Provider) MarkAsRead(_ context.Context, folder string, uid uint32) error {
	return fetcher.MarkEmailAsReadInMailbox(p.account, folder, uid)
}

func (p *Provider) DeleteEmail(_ context.Context, folder string, uid uint32) error {
	return fetcher.DeleteEmailFromMailbox(p.account, folder, uid)
}

func (p *Provider) ArchiveEmail(_ context.Context, folder string, uid uint32) error {
	return fetcher.ArchiveEmailFromMailbox(p.account, folder, uid)
}

func (p *Provider) MoveEmail(_ context.Context, uid uint32, srcFolder, dstFolder string) error {
	return fetcher.MoveEmailToFolder(p.account, uid, srcFolder, dstFolder)
}

func (p *Provider) SendEmail(_ context.Context, msg *backend.OutgoingEmail) error {
	return sender.SendEmail(
		p.account, msg.To, msg.Cc, msg.Bcc,
		msg.Subject, msg.PlainBody, msg.HTMLBody,
		msg.Images, msg.Attachments,
		msg.InReplyTo, msg.References,
		msg.SignSMIME, msg.EncryptSMIME,
		msg.SignPGP, msg.EncryptPGP,
	)
}

func (p *Provider) FetchFolders(_ context.Context) ([]backend.Folder, error) {
	folders, err := fetcher.FetchFolders(p.account)
	if err != nil {
		return nil, err
	}
	return toBackendFolders(folders), nil
}

func (p *Provider) Watch(_ context.Context, _ string) (<-chan backend.NotifyEvent, func(), error) {
	// IMAP IDLE is handled by the existing IdleWatcher in main.go
	return nil, nil, backend.ErrNotSupported
}

func (p *Provider) Close() error {
	return nil
}

// Verify interface compliance at compile time.
var _ backend.Provider = (*Provider)(nil)

// Conversion helpers

func toBackendEmails(emails []fetcher.Email) []backend.Email {
	result := make([]backend.Email, len(emails))
	for i, e := range emails {
		result[i] = backend.Email{
			UID:         e.UID,
			From:        e.From,
			To:          e.To,
			Subject:     e.Subject,
			Body:        e.Body,
			Date:        e.Date,
			IsRead:      e.IsRead,
			MessageID:   e.MessageID,
			References:  e.References,
			Attachments: toBackendAttachments(e.Attachments),
			AccountID:   e.AccountID,
		}
	}
	return result
}

func toBackendAttachments(atts []fetcher.Attachment) []backend.Attachment {
	result := make([]backend.Attachment, len(atts))
	for i, a := range atts {
		result[i] = backend.Attachment{
			Filename:         a.Filename,
			PartID:           a.PartID,
			Data:             a.Data,
			Encoding:         a.Encoding,
			MIMEType:         a.MIMEType,
			ContentID:        a.ContentID,
			Inline:           a.Inline,
			IsSMIMESignature: a.IsSMIMESignature,
			SMIMEVerified:    a.SMIMEVerified,
			IsSMIMEEncrypted: a.IsSMIMEEncrypted,
		}
	}
	return result
}

func toBackendFolders(folders []fetcher.Folder) []backend.Folder {
	result := make([]backend.Folder, len(folders))
	for i, f := range folders {
		result[i] = backend.Folder{
			Name:       f.Name,
			Delimiter:  f.Delimiter,
			Attributes: f.Attributes,
		}
	}
	return result
}
