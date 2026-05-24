// Package maildir implements the backend.Provider interface for local
// Maildir mailboxes (the `mutt -f Maildir` style). It is read/edit only —
// there is no SMTP transport, so SendEmail returns ErrNotSupported.
//
// Folder layout follows Maildir++:
//   - The configured root path is "INBOX".
//   - Sibling directories prefixed with "." (e.g. ".Sent", ".Archive") are
//     additional folders. Inner dots map to a "/" hierarchy.
package maildir

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	emaildir "github.com/emersion/go-maildir"
	"github.com/emersion/go-message"
	gomail "github.com/emersion/go-message/mail"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
)

var messageIDRE = regexp.MustCompile(`<[^>]+>`)

func init() {
	backend.RegisterBackend("maildir", func(account *config.Account) (backend.Provider, error) {
		return New(account)
	})
}

// Provider implements backend.Provider against a local Maildir tree.
type Provider struct {
	account *config.Account
	root    string
}

// New creates a new Maildir provider for the given account.
func New(account *config.Account) (*Provider, error) {
	root := strings.TrimSpace(account.MaildirPath)
	if root == "" {
		return nil, fmt.Errorf("maildir path not configured")
	}

	root = os.ExpandEnv(root)
	if strings.HasPrefix(root, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			root = filepath.Join(home, root[2:])
		}
	}
	root = filepath.Clean(root)

	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("maildir path %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("maildir path %q is not a directory", root)
	}

	return &Provider{account: account, root: root}, nil
}

// dirForFolder resolves a logical folder name to the on-disk Maildir directory.
// "" and "INBOX" map to the configured root; anything else is treated as a
// Maildir++ subfolder. "/" in the folder name is converted to "." per spec.
func (p *Provider) dirForFolder(folder string) emaildir.Dir {
	if folder == "" || strings.EqualFold(folder, "INBOX") {
		return emaildir.Dir(p.root)
	}
	subdir := "." + strings.ReplaceAll(folder, "/", ".")
	return emaildir.Dir(filepath.Join(p.root, subdir))
}

// FetchFolders returns INBOX plus any Maildir++ subfolders found at the root.
func (p *Provider) FetchFolders(_ context.Context) ([]backend.Folder, error) {
	folders := []backend.Folder{{Name: "INBOX", Delimiter: "/"}}

	entries, err := os.ReadDir(p.root)
	if err != nil {
		return nil, fmt.Errorf("maildir read root: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, ".") || name == "." || name == ".." {
			continue
		}
		// Sanity check: a Maildir folder has a cur/ subdir.
		if _, err := os.Stat(filepath.Join(p.root, name, "cur")); err != nil {
			continue
		}
		// Strip leading dot, map "." → "/" for nested folders.
		logical := strings.ReplaceAll(strings.TrimPrefix(name, "."), ".", "/")
		folders = append(folders, backend.Folder{Name: logical, Delimiter: "/"})
	}

	return folders, nil
}

// FetchEmails returns messages from the folder, newest first. Any messages
// sitting in new/ are first promoted to cur/ (same semantics as mutt opening
// a Maildir): they remain unread (no Seen flag) but become trackable.
func (p *Provider) FetchEmails(_ context.Context, folder string, limit, offset uint32) ([]backend.Email, error) {
	dir := p.dirForFolder(folder)
	if _, err := dir.Unseen(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("maildir promote new/: %w", err)
	}
	msgs, err := dir.Messages()
	if err != nil {
		return nil, fmt.Errorf("maildir messages: %w", err)
	}

	type entry struct {
		msg     *emaildir.Message
		modTime time.Time
	}
	entries := make([]entry, 0, len(msgs))
	for _, m := range msgs {
		info, err := os.Stat(m.Filename())
		if err != nil {
			continue
		}
		entries = append(entries, entry{msg: m, modTime: info.ModTime()})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].modTime.After(entries[j].modTime)
	})

	if int(offset) >= len(entries) {
		return []backend.Email{}, nil
	}
	end := int(offset) + int(limit)
	if end > len(entries) || limit == 0 {
		end = len(entries)
	}
	entries = entries[offset:end]

	emails := make([]backend.Email, 0, len(entries))
	for _, e := range entries {
		email, err := p.readHeader(e.msg)
		if err != nil {
			continue
		}
		emails = append(emails, email)
	}
	return emails, nil
}

// readHeader opens the message file and parses just enough to fill an Email.
func (p *Provider) readHeader(msg *emaildir.Message) (backend.Email, error) {
	rc, err := msg.Open()
	if err != nil {
		return backend.Email{}, err
	}
	defer rc.Close() //nolint:errcheck

	entity, err := message.Read(rc)
	if err != nil && entity == nil {
		return backend.Email{}, err
	}

	email := headerToEmail(&entity.Header, msg.Key(), p.account.ID)

	for _, fl := range msg.Flags() {
		if fl == emaildir.FlagSeen {
			email.IsRead = true
			break
		}
	}

	return email, nil
}

// FetchEmailBody returns the chosen body, MIME type, and attachments.
func (p *Provider) FetchEmailBody(_ context.Context, folder string, uid uint32) (string, string, []backend.Attachment, error) {
	msg, err := p.findMessageByUID(folder, uid)
	if err != nil {
		return "", "", nil, err
	}
	rc, err := msg.Open()
	if err != nil {
		return "", "", nil, fmt.Errorf("maildir open: %w", err)
	}
	defer rc.Close() //nolint:errcheck

	return parseMessageBody(rc)
}

// FetchAttachment returns the raw bytes of an attachment part.
func (p *Provider) FetchAttachment(_ context.Context, folder string, uid uint32, partID, _ string) ([]byte, error) {
	msg, err := p.findMessageByUID(folder, uid)
	if err != nil {
		return nil, err
	}
	rc, err := msg.Open()
	if err != nil {
		return nil, fmt.Errorf("maildir open: %w", err)
	}
	defer rc.Close() //nolint:errcheck

	return findAttachmentData(rc, partID)
}

// MarkAsRead sets the Seen flag while preserving the others.
func (p *Provider) MarkAsRead(_ context.Context, folder string, uid uint32) error {
	msg, err := p.findMessageByUID(folder, uid)
	if err != nil {
		return err
	}
	flags := msg.Flags()
	for _, fl := range flags {
		if fl == emaildir.FlagSeen {
			return nil
		}
	}
	return msg.SetFlags(append(flags, emaildir.FlagSeen))
}

// MarkAsUnread removes the Seen flag while preserving the others.
func (p *Provider) MarkAsUnread(_ context.Context, folder string, uid uint32) error {
	msg, err := p.findMessageByUID(folder, uid)
	if err != nil {
		return err
	}
	flags := msg.Flags()
	filtered := flags[:0]
	for _, fl := range flags {
		if fl != emaildir.FlagSeen {
			filtered = append(filtered, fl)
		}
	}
	if len(filtered) == len(flags) {
		return nil // already unread
	}
	return msg.SetFlags(filtered)
}

// DeleteEmail removes the message file from disk.
func (p *Provider) DeleteEmail(_ context.Context, folder string, uid uint32) error {
	msg, err := p.findMessageByUID(folder, uid)
	if err != nil {
		return err
	}
	return msg.Remove()
}

// ArchiveEmail moves the message to the ".Archive" subfolder if one exists.
func (p *Provider) ArchiveEmail(ctx context.Context, folder string, uid uint32) error {
	if _, err := os.Stat(filepath.Join(p.root, ".Archive", "cur")); err != nil {
		return backend.ErrNotSupported
	}
	return p.MoveEmail(ctx, uid, folder, "Archive")
}

// MoveEmail relocates a message between two Maildir folders.
func (p *Provider) MoveEmail(_ context.Context, uid uint32, srcFolder, dstFolder string) error {
	msg, err := p.findMessageByUID(srcFolder, uid)
	if err != nil {
		return err
	}
	dst := p.dirForFolder(dstFolder)
	return msg.MoveTo(dst)
}

// DeleteEmails removes the listed messages from the folder.
func (p *Provider) DeleteEmails(ctx context.Context, folder string, uids []uint32) error {
	for _, uid := range uids {
		if err := p.DeleteEmail(ctx, folder, uid); err != nil {
			return err
		}
	}
	return nil
}

// ArchiveEmails archives the listed messages.
func (p *Provider) ArchiveEmails(ctx context.Context, folder string, uids []uint32) error {
	if _, err := os.Stat(filepath.Join(p.root, ".Archive", "cur")); err != nil {
		return backend.ErrNotSupported
	}
	for _, uid := range uids {
		if err := p.MoveEmail(ctx, uid, folder, "Archive"); err != nil {
			return err
		}
	}
	return nil
}

// MoveEmails relocates the listed messages between folders.
func (p *Provider) MoveEmails(ctx context.Context, uids []uint32, srcFolder, dstFolder string) error {
	for _, uid := range uids {
		if err := p.MoveEmail(ctx, uid, srcFolder, dstFolder); err != nil {
			return err
		}
	}
	return nil
}

// SendEmail is not supported by the Maildir backend.
func (p *Provider) SendEmail(_ context.Context, _ *backend.OutgoingEmail) error {
	return backend.ErrNotSupported
}

// Search filters messages in a folder by the given query, parsing headers
// locally. Body matching scans the decoded body parts.
func (p *Provider) Search(_ context.Context, folder string, query backend.SearchQuery) ([]backend.Email, error) {
	dir := p.dirForFolder(folder)
	if _, err := dir.Unseen(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("maildir promote new/: %w", err)
	}
	msgs, err := dir.Messages()
	if err != nil {
		return nil, fmt.Errorf("maildir messages: %w", err)
	}

	results := make([]backend.Email, 0)
	for _, m := range msgs {
		if query.Limit > 0 && uint32(len(results)) >= query.Limit {
			break
		}
		email, body, err := p.matchOpen(m)
		if err != nil {
			continue
		}
		if !matchesQuery(email, body, query) {
			continue
		}
		results = append(results, email)
	}
	return results, nil
}

// matchOpen returns the email metadata and a plain-text body slice for search.
func (p *Provider) matchOpen(msg *emaildir.Message) (backend.Email, string, error) {
	rc, err := msg.Open()
	if err != nil {
		return backend.Email{}, "", err
	}
	defer rc.Close() //nolint:errcheck

	entity, err := message.Read(rc)
	if err != nil && entity == nil {
		return backend.Email{}, "", err
	}
	email := headerToEmail(&entity.Header, msg.Key(), p.account.ID)

	for _, fl := range msg.Flags() {
		if fl == emaildir.FlagSeen {
			email.IsRead = true
			break
		}
	}

	// Lightweight body read: only needed if query asks for it.
	var body string
	if b, err := io.ReadAll(entity.Body); err == nil {
		body = string(b)
	}

	return email, body, nil
}

// matchesQuery applies the parsed search filters to an email + body.
func matchesQuery(email backend.Email, body string, query backend.SearchQuery) bool {
	containsCI := func(haystack, needle string) bool {
		if needle == "" {
			return true
		}
		return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
	}
	if !containsCI(email.From, query.From) {
		return false
	}
	if query.To != "" {
		match := false
		for _, addr := range email.To {
			if containsCI(addr, query.To) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}
	if !containsCI(email.Subject, query.Subject) {
		return false
	}
	if !containsCI(body, query.Body) {
		return false
	}
	if !query.Since.IsZero() && email.Date.Before(query.Since) {
		return false
	}
	if !query.Before.IsZero() && email.Date.After(query.Before) {
		return false
	}
	return true
}

// Watch is not supported. Future: fsnotify on new/ to emit NotifyNewEmail.
func (p *Provider) Watch(_ context.Context, _ string) (<-chan backend.NotifyEvent, func(), error) {
	return nil, nil, backend.ErrNotSupported
}

// Close releases any provider-held resources. None for Maildir.
func (p *Provider) Close() error { return nil }

// Capabilities reports what the Maildir backend can do.
func (p *Provider) Capabilities() backend.Capabilities {
	_, hasArchive := os.Stat(filepath.Join(p.root, ".Archive", "cur"))
	return backend.Capabilities{
		CanSend:         false,
		CanMove:         true,
		CanArchive:      hasArchive == nil,
		CanPush:         false,
		CanSearchServer: true,
		CanFetchFolders: true,
		SupportsSMIME:   false,
	}
}

// findMessageByUID locates a Maildir message by its UID hash.
func (p *Provider) findMessageByUID(folder string, uid uint32) (*emaildir.Message, error) {
	dir := p.dirForFolder(folder)
	msgs, err := dir.Messages()
	if err != nil {
		return nil, fmt.Errorf("maildir messages: %w", err)
	}
	for _, m := range msgs {
		if hashUID(m.Key()) == uid {
			return m, nil
		}
	}
	return nil, fmt.Errorf("maildir: message with UID %d not found in %q", uid, folder)
}

// hashUID converts a Maildir base filename (the part before the flag suffix)
// into a stable uint32 identifier. Same FNV-style hash as the POP3 backend.
func hashUID(key string) uint32 {
	var hash uint32
	for _, c := range key {
		hash = hash*31 + uint32(c)
	}
	if hash == 0 {
		hash = 1
	}
	return hash
}

// headerToEmail converts a parsed message Header into a backend.Email.
func headerToEmail(header *message.Header, key, accountID string) backend.Email {
	from := header.Get("From")
	subject := header.Get("Subject")
	dateStr := header.Get("Date")
	messageID := header.Get("Message-ID")
	inReplyTo := firstMessageID(header.Get("In-Reply-To"))
	references := messageIDList(header.Get("References"))

	var to []string
	if toHeader := header.Get("To"); toHeader != "" {
		if addrs, err := mail.ParseAddressList(toHeader); err == nil {
			for _, addr := range addrs {
				to = append(to, addr.Address)
			}
		}
	}

	var replyTo []string
	if replyToHeader := header.Get("Reply-To"); replyToHeader != "" {
		if addrs, err := mail.ParseAddressList(replyToHeader); err == nil {
			for _, addr := range addrs {
				replyTo = append(replyTo, addr.Address)
			}
		}
	}

	var date time.Time
	if dateStr != "" {
		if parsed, err := mail.ParseDate(dateStr); err == nil {
			date = parsed
		}
	}

	dec := new(mime.WordDecoder)
	if decoded, err := dec.DecodeHeader(subject); err == nil {
		subject = decoded
	}
	if decoded, err := dec.DecodeHeader(from); err == nil {
		from = decoded
	}

	return backend.Email{
		UID:        hashUID(key),
		From:       from,
		To:         to,
		ReplyTo:    replyTo,
		Subject:    subject,
		Date:       date,
		MessageID:  messageID,
		InReplyTo:  inReplyTo,
		References: references,
		AccountID:  accountID,
	}
}

func firstMessageID(value string) string {
	ids := messageIDList(value)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

func messageIDList(value string) []string {
	matches := messageIDRE.FindAllString(value, -1)
	if len(matches) == 0 {
		return strings.Fields(value)
	}
	return matches
}

// parseMessageBody extracts the body text and attachments from a raw message.
// Mirrors the POP3 backend's logic since the on-wire representation is the
// same RFC822 stream.
func parseMessageBody(r io.Reader) (string, string, []backend.Attachment, error) {
	mr, err := gomail.CreateReader(r)
	if err != nil {
		body, rerr := io.ReadAll(r)
		if rerr != nil {
			return "", "", nil, rerr
		}
		return string(body), "", nil, nil
	}

	var bodyText string
	var htmlBody string
	var attachments []backend.Attachment
	partIdx := 0

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
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

		switch {
		case disposition == "attachment" || (disposition == "inline" && !strings.HasPrefix(contentType, "text/")):
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
		case contentType == "text/html":
			htmlBody = string(data)
		case contentType == "text/plain" && bodyText == "":
			bodyText = string(data)
		}
	}

	if htmlBody != "" {
		return htmlBody, "text/html", attachments, nil
	}
	return bodyText, "text/plain", attachments, nil
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
		if errors.Is(err, io.EOF) {
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

	return nil, fmt.Errorf("maildir: attachment part %s not found", targetPartID)
}

// Verify interface compliance at compile time.
var _ backend.Provider = (*Provider)(nil)
