package fetcher

import (
	"github.com/floatpane/matcha/backend"
	_ "github.com/floatpane/matcha/backend/maildir" // register maildir backend
	"github.com/floatpane/matcha/config"
)

// backendProvider returns a backend.Provider for accounts whose Protocol
// is handled by a non-IMAP backend (currently only "maildir"). It returns
// (nil, nil) for the default IMAP path so callers fall through to the
// existing imapclient code.
func backendProvider(account *config.Account) (backend.Provider, error) {
	if account == nil {
		return nil, nil
	}
	switch account.Protocol {
	case "maildir":
		return backend.New(account)
	}
	return nil, nil
}

func backendFoldersToFetcher(in []backend.Folder) []Folder {
	out := make([]Folder, len(in))
	for i, f := range in {
		out[i] = Folder{
			Name:       f.Name,
			Delimiter:  f.Delimiter,
			Attributes: f.Attributes,
			Unread:     f.Unread,
		}
	}
	return out
}

func backendEmailsToFetcher(in []backend.Email) []Email {
	out := make([]Email, len(in))
	for i, e := range in {
		out[i] = Email{
			UID:         e.UID,
			From:        e.From,
			To:          e.To,
			ReplyTo:     e.ReplyTo,
			Subject:     e.Subject,
			Body:        e.Body,
			Date:        e.Date,
			IsRead:      e.IsRead,
			MessageID:   e.MessageID,
			InReplyTo:   e.InReplyTo,
			References:  e.References,
			Attachments: backendAttachmentsToFetcher(e.Attachments),
			AccountID:   e.AccountID,
		}
	}
	return out
}

func backendAttachmentsToFetcher(in []backend.Attachment) []Attachment {
	out := make([]Attachment, len(in))
	for i, a := range in {
		out[i] = Attachment{
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
			IsPGPSignature:   a.IsPGPSignature,
			PGPVerified:      a.PGPVerified,
			IsPGPEncrypted:   a.IsPGPEncrypted,
		}
	}
	return out
}
