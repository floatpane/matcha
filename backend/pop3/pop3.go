// Package pop3 implements the backend.Provider interface using POP3 for
// reading email and SMTP for sending.
//
// POP3 is inherently limited compared to IMAP/JMAP:
//   - Only supports a single "INBOX" folder
//   - No support for flags (mark as read is a no-op)
//   - No support for moving or archiving emails
//   - No support for push notifications (IDLE)
//   - Delete marks for deletion; executed on Quit()
package pop3

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"strings"
	"time"

	"github.com/emersion/go-message"
	gomail "github.com/emersion/go-message/mail"
	pop3client "github.com/knadh/go-pop3"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/sender"
)

func init() {
	backend.RegisterBackend("pop3", func(account *config.Account) (backend.Provider, error) {
		return New(account)
	})
}

// Provider implements backend.Provider using POP3+SMTP.
type Provider struct {
	account *config.Account
	opt     pop3client.Opt
}

// New creates a new POP3 provider for the given account.
func New(account *config.Account) (*Provider, error) {
	server := account.GetPOP3Server()
	port := account.GetPOP3Port()

	if server == "" {
		return nil, fmt.Errorf("POP3 server not configured")
	}

	opt := pop3client.Opt{
		Host:          server,
		Port:          port,
		TLSEnabled:    true,
		TLSSkipVerify: account.Insecure,
	}

	// Non-SSL ports use plain connection
	if port == 110 {
		opt.TLSEnabled = false
	}

	return &Provider{
		account: account,
		opt:     opt,
	}, nil
}

// connect creates a new POP3 connection and authenticates.
func (p *Provider) connect() (*pop3client.Conn, error) {
	client := pop3client.New(p.opt)
	conn, err := client.NewConn()
	if err != nil {
		return nil, fmt.Errorf("pop3 connect: %w", err)
	}

	if err := conn.Auth(p.account.Email, p.account.Password); err != nil {
		conn.Quit()
		return nil, fmt.Errorf("pop3 auth: %w", err)
	}

	return conn, nil
}

func (p *Provider) FetchEmails(_ context.Context, _ string, limit, offset uint32) ([]backend.Email, error) {
	conn, err := p.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Quit()

	// Get message list with UIDs
	msgs, err := conn.Uidl(0)
	if err != nil {
		// Fallback to LIST if UIDL not supported
		msgs, err = conn.List(0)
		if err != nil {
			return nil, fmt.Errorf("pop3 list: %w", err)
		}
	}

	if len(msgs) == 0 {
		return []backend.Email{}, nil
	}

	// POP3 messages are 1-indexed. We want newest first (highest ID first).
	start := len(msgs) - int(offset)
	if start <= 0 {
		return []backend.Email{}, nil
	}

	end := start - int(limit)
	if end < 0 {
		end = 0
	}

	var emails []backend.Email
	for i := start; i > end; i-- {
		msgInfo := msgs[i-1]

		// Fetch headers only using TOP (0 lines of body)
		entity, err := conn.Top(msgInfo.ID, 0)
		if err != nil {
			continue
		}

		email := entityToEmail(&entity.Header, msgInfo, p.account.ID)
		emails = append(emails, email)
	}

	return emails, nil
}

func (p *Provider) FetchEmailBody(_ context.Context, _ string, uid uint32) (string, []backend.Attachment, error) {
	conn, err := p.connect()
	if err != nil {
		return "", nil, err
	}
	defer conn.Quit()

	msgID, err := p.findMessageByUID(conn, uid)
	if err != nil {
		return "", nil, err
	}

	raw, err := conn.RetrRaw(msgID)
	if err != nil {
		return "", nil, fmt.Errorf("pop3 retr: %w", err)
	}

	return parseMessageBody(raw)
}

func (p *Provider) FetchAttachment(_ context.Context, _ string, uid uint32, partID, _ string) ([]byte, error) {
	conn, err := p.connect()
	if err != nil {
		return nil, err
	}
	defer conn.Quit()

	msgID, err := p.findMessageByUID(conn, uid)
	if err != nil {
		return nil, err
	}

	raw, err := conn.RetrRaw(msgID)
	if err != nil {
		return nil, fmt.Errorf("pop3 retr: %w", err)
	}

	return findAttachmentData(raw, partID)
}

func (p *Provider) MarkAsRead(_ context.Context, _ string, _ uint32) error {
	// POP3 has no concept of read/unread flags — this is a no-op
	return nil
}

func (p *Provider) DeleteEmail(_ context.Context, _ string, uid uint32) error {
	conn, err := p.connect()
	if err != nil {
		return err
	}

	msgID, err := p.findMessageByUID(conn, uid)
	if err != nil {
		conn.Quit()
		return err
	}

	if err := conn.Dele(msgID); err != nil {
		conn.Quit()
		return fmt.Errorf("pop3 dele: %w", err)
	}

	// Quit commits the deletion
	return conn.Quit()
}

func (p *Provider) ArchiveEmail(_ context.Context, _ string, _ uint32) error {
	return backend.ErrNotSupported
}

func (p *Provider) MoveEmail(_ context.Context, _ uint32, _, _ string) error {
	return backend.ErrNotSupported
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
	return []backend.Folder{
		{Name: "INBOX", Delimiter: "/"},
	}, nil
}

func (p *Provider) Watch(_ context.Context, _ string) (<-chan backend.NotifyEvent, func(), error) {
	return nil, nil, backend.ErrNotSupported
}

func (p *Provider) Close() error {
	return nil
}

// Verify interface compliance at compile time.
var _ backend.Provider = (*Provider)(nil)

// findMessageByUID finds a POP3 message ID by matching the UID hash.
func (p *Provider) findMessageByUID(conn *pop3client.Conn, uid uint32) (int, error) {
	msgs, err := conn.Uidl(0)
	if err != nil {
		msgs, err = conn.List(0)
		if err != nil {
			return 0, fmt.Errorf("pop3 list: %w", err)
		}
		for _, m := range msgs {
			if hashUID(fmt.Sprintf("%d", m.ID)) == uid {
				return m.ID, nil
			}
		}
		return 0, fmt.Errorf("pop3: message with UID %d not found", uid)
	}

	for _, m := range msgs {
		if hashUID(m.UID) == uid {
			return m.ID, nil
		}
	}
	return 0, fmt.Errorf("pop3: message with UID %d not found", uid)
}

// hashUID converts a POP3 UIDL string to a uint32 hash.
func hashUID(uidl string) uint32 {
	var hash uint32
	for _, c := range uidl {
		hash = hash*31 + uint32(c)
	}
	if hash == 0 {
		hash = 1
	}
	return hash
}

// entityToEmail converts message headers to a backend.Email.
func entityToEmail(header *message.Header, msgInfo pop3client.MessageID, accountID string) backend.Email {
	from := header.Get("From")
	subject := header.Get("Subject")
	dateStr := header.Get("Date")
	messageID := header.Get("Message-ID")

	var to []string
	if toHeader := header.Get("To"); toHeader != "" {
		for _, addr := range strings.Split(toHeader, ",") {
			to = append(to, strings.TrimSpace(addr))
		}
	}

	var date time.Time
	if dateStr != "" {
		if parsed, err := mail.ParseDate(dateStr); err == nil {
			date = parsed
		}
	}

	// Decode MIME-encoded headers
	dec := new(mime.WordDecoder)
	if decoded, err := dec.DecodeHeader(subject); err == nil {
		subject = decoded
	}
	if decoded, err := dec.DecodeHeader(from); err == nil {
		from = decoded
	}

	uidStr := msgInfo.UID
	if uidStr == "" {
		uidStr = fmt.Sprintf("%d", msgInfo.ID)
	}

	return backend.Email{
		UID:       hashUID(uidStr),
		From:      from,
		To:        to,
		Subject:   subject,
		Date:      date,
		IsRead:    false,
		MessageID: messageID,
		AccountID: accountID,
	}
}

// parseMessageBody extracts the body text and attachments from a raw message.
func parseMessageBody(r io.Reader) (string, []backend.Attachment, error) {
	mr, err := gomail.CreateReader(r)
	if err != nil {
		// Not a multipart message — read body directly
		body, err := io.ReadAll(r)
		if err != nil {
			return "", nil, err
		}
		return string(body), nil, nil
	}

	var bodyText string
	var htmlBody string
	var attachments []backend.Attachment
	partIdx := 0

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		partIdx++

		contentType, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		disposition, dParams, _ := mime.ParseMediaType(part.Header.Get("Content-Disposition"))

		data, readErr := io.ReadAll(part.Body)
		if readErr != nil {
			continue
		}

		if disposition == "attachment" || (disposition == "inline" && !strings.HasPrefix(contentType, "text/")) {
			filename := dParams["filename"]
			if filename == "" {
				_, cp, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
				filename = cp["name"]
			}
			att := backend.Attachment{
				Filename: filename,
				PartID:   fmt.Sprintf("%d", partIdx),
				Data:     data,
				MIMEType: contentType,
				Inline:   disposition == "inline",
			}
			if cid := part.Header.Get("Content-ID"); cid != "" {
				att.ContentID = strings.Trim(cid, "<>")
			}
			attachments = append(attachments, att)
		} else if contentType == "text/html" {
			htmlBody = string(data)
		} else if contentType == "text/plain" && bodyText == "" {
			bodyText = string(data)
		}
	}

	if htmlBody != "" {
		return htmlBody, attachments, nil
	}
	return bodyText, attachments, nil
}

// findAttachmentData walks a raw message to find attachment data by partID.
func findAttachmentData(r io.Reader, targetPartID string) ([]byte, error) {
	mr, err := gomail.CreateReader(r)
	if err != nil {
		return nil, fmt.Errorf("not a multipart message")
	}

	partIdx := 0
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		partIdx++

		if fmt.Sprintf("%d", partIdx) == targetPartID {
			return io.ReadAll(part.Body)
		}
	}

	return nil, fmt.Errorf("pop3: attachment part %s not found", targetPartID)
}
