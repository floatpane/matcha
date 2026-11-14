package fetcher

import (
	"github.com/floatpane/matcha/backend"
	_ "github.com/floatpane/matcha/backend/jmap"    // register jmap backend
	_ "github.com/floatpane/matcha/backend/maildir" // register maildir backend
	_ "github.com/floatpane/matcha/backend/pop3"    // register pop3 backend
	"github.com/floatpane/matcha/config"
)

// hasBackendProvider reports whether the account is served by a non-IMAP
// backend ("maildir", "jmap" or "pop3") and should be routed through the
// backend.Provider abstraction instead of the legacy IMAP code path. POP3 was
// missing, so POP3 accounts were fetched over IMAP with an empty server name
// and always showed an empty inbox.
func hasBackendProvider(account *config.Account) bool {
	if account == nil {
		return false
	}
	switch account.Protocol {
	case "maildir", "jmap", "pop3":
		return true
	default:
		return false
	}
}

// newBackendProvider builds the backend.Provider for the account. Callers
// must guard with hasBackendProvider before invoking it.
func newBackendProvider(account *config.Account) (backend.Provider, error) {
	return backend.New(account)
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
