// Package jmap implements the backend.Provider interface using the JMAP protocol
// (RFC 8620 Core + RFC 8621 Mail).
package jmap

import (
	"bytes"
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"strings"
	"sync"
	"time"

	jmapclient "git.sr.ht/~rockorager/go-jmap"
	"git.sr.ht/~rockorager/go-jmap/core/push"
	"git.sr.ht/~rockorager/go-jmap/mail"
	"git.sr.ht/~rockorager/go-jmap/mail/email"
	"git.sr.ht/~rockorager/go-jmap/mail/emailsubmission"
	"git.sr.ht/~rockorager/go-jmap/mail/mailbox"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
)

func init() {
	backend.RegisterBackend("jmap", func(account *config.Account) (backend.Provider, error) {
		return New(account)
	})
}

// Provider implements backend.Provider using JMAP.
type Provider struct {
	account   *config.Account
	client    *jmapclient.Client
	accountID jmapclient.ID

	mu         sync.Mutex
	mailboxes  map[string]jmapclient.ID // name -> ID
	roleToID   map[mailbox.Role]jmapclient.ID
	idToJMAPID map[uint32]jmapclient.ID // UID hash -> JMAP ID
}

// New creates a new JMAP provider.
func New(account *config.Account) (*Provider, error) {
	if account.JMAPEndpoint == "" {
		return nil, fmt.Errorf("JMAP endpoint URL not configured")
	}

	client := &jmapclient.Client{
		SessionEndpoint: account.JMAPEndpoint,
	}

	if account.AuthMethod == "oauth2" {
		client.WithAccessToken(account.Password)
	} else {
		client.WithBasicAuth(account.Email, account.Password)
	}

	if err := client.Authenticate(); err != nil {
		return nil, fmt.Errorf("jmap auth: %w", err)
	}

	acctID := client.Session.PrimaryAccounts[mail.URI]
	if acctID == "" {
		return nil, fmt.Errorf("jmap: no mail account found in session")
	}

	p := &Provider{
		account:    account,
		client:     client,
		accountID:  acctID,
		mailboxes:  make(map[string]jmapclient.ID),
		roleToID:   make(map[mailbox.Role]jmapclient.ID),
		idToJMAPID: make(map[uint32]jmapclient.ID),
	}

	// Pre-fetch mailbox list
	if err := p.refreshMailboxes(); err != nil {
		return nil, fmt.Errorf("jmap mailboxes: %w", err)
	}

	return p, nil
}

func (p *Provider) refreshMailboxes() error {
	req := &jmapclient.Request{}
	req.Invoke(&mailbox.Get{
		Account: p.accountID,
	})

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, inv := range resp.Responses {
		if r, ok := inv.Args.(*mailbox.GetResponse); ok {
			for _, mbox := range r.List {
				p.mailboxes[mbox.Name] = mbox.ID
				if mbox.Role != "" {
					p.roleToID[mbox.Role] = mbox.ID
				}
			}
		}
	}
	return nil
}

// resolveMailboxID maps a folder name to a JMAP mailbox ID.
func (p *Provider) resolveMailboxID(folder string) (jmapclient.ID, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Direct name match
	if id, ok := p.mailboxes[folder]; ok {
		return id, nil
	}

	// Role-based fallback for common folder names
	nameToRole := map[string]mailbox.Role{
		"INBOX":   mailbox.RoleInbox,
		"Inbox":   mailbox.RoleInbox,
		"Sent":    mailbox.RoleSent,
		"Drafts":  mailbox.RoleDrafts,
		"Trash":   mailbox.RoleTrash,
		"Junk":    mailbox.RoleJunk,
		"Spam":    mailbox.RoleJunk,
		"Archive": mailbox.RoleArchive,
	}
	if role, ok := nameToRole[folder]; ok {
		if id, ok := p.roleToID[role]; ok {
			return id, nil
		}
	}

	return "", fmt.Errorf("jmap: mailbox %q not found", folder)
}

func (p *Provider) FetchEmails(_ context.Context, folder string, limit, offset uint32) ([]backend.Email, error) {
	mboxID, err := p.resolveMailboxID(folder)
	if err != nil {
		return nil, err
	}

	req := &jmapclient.Request{}

	queryCallID := req.Invoke(&email.Query{
		Account: p.accountID,
		Filter:  &email.FilterCondition{InMailbox: mboxID},
		Sort: []*email.SortComparator{
			{Property: "receivedAt", IsAscending: false},
		},
		Position: int64(offset),
		Limit:    uint64(limit),
	})

	req.Invoke(&email.Get{
		Account: p.accountID,
		ReferenceIDs: &jmapclient.ResultReference{
			ResultOf: queryCallID,
			Name:     "Email/query",
			Path:     "/ids",
		},
		Properties: []string{
			"id", "subject", "from", "to", "replyTo", "receivedAt",
			"preview", "keywords", "mailboxIds", "hasAttachment",
			"messageId",
		},
	})

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jmap fetch: %w", err)
	}

	var emails []backend.Email
	for _, inv := range resp.Responses {
		if r, ok := inv.Args.(*email.GetResponse); ok {
			for _, eml := range r.List {
				uid := jmapIDToUID(eml.ID)
				p.mu.Lock()
				p.idToJMAPID[uid] = eml.ID
				p.mu.Unlock()

				e := jmapEmailToBackend(eml, uid, p.account.ID)
				emails = append(emails, e)
			}
		}
	}

	return emails, nil
}

func (p *Provider) FetchEmailBody(_ context.Context, _ string, uid uint32) (string, []backend.Attachment, error) {
	jmapID, err := p.lookupJMAPID(uid)
	if err != nil {
		return "", nil, err
	}

	req := &jmapclient.Request{}
	req.Invoke(&email.Get{
		Account: p.accountID,
		IDs:     []jmapclient.ID{jmapID},
		Properties: []string{
			"id", "bodyValues", "htmlBody", "textBody", "attachments",
			"bodyStructure",
		},
		BodyProperties:      []string{"partId", "blobId", "size", "type", "name", "disposition", "cid"},
		FetchHTMLBodyValues: true,
		FetchTextBodyValues: true,
	})

	resp, err := p.client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("jmap body: %w", err)
	}

	for _, inv := range resp.Responses {
		if r, ok := inv.Args.(*email.GetResponse); ok && len(r.List) > 0 {
			eml := r.List[0]

			// Get body text (prefer HTML)
			var body string
			for _, part := range eml.HTMLBody {
				if val, ok := eml.BodyValues[part.PartID]; ok {
					body = val.Value
					break
				}
			}
			if body == "" {
				for _, part := range eml.TextBody {
					if val, ok := eml.BodyValues[part.PartID]; ok {
						body = val.Value
						break
					}
				}
			}

			// Get attachments
			var atts []backend.Attachment
			for _, att := range eml.Attachments {
				a := backend.Attachment{
					Filename: att.Name,
					PartID:   string(att.BlobID),
					MIMEType: att.Type,
					Inline:   att.Disposition == "inline",
				}
				if att.CID != "" {
					a.ContentID = strings.Trim(att.CID, "<>")
				}
				atts = append(atts, a)
			}

			return body, atts, nil
		}
	}

	return "", nil, fmt.Errorf("jmap: email not found")
}

func (p *Provider) FetchAttachment(_ context.Context, _ string, _ uint32, partID, _ string) ([]byte, error) {
	// partID is the blobId for JMAP
	blobID := jmapclient.ID(partID)
	reader, err := p.client.Download(p.accountID, blobID)
	if err != nil {
		return nil, fmt.Errorf("jmap download: %w", err)
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (p *Provider) MarkAsRead(_ context.Context, _ string, uid uint32) error {
	jmapID, err := p.lookupJMAPID(uid)
	if err != nil {
		return err
	}

	req := &jmapclient.Request{}
	req.Invoke(&email.Set{
		Account: p.accountID,
		Update: map[jmapclient.ID]jmapclient.Patch{
			jmapID: {"keywords/$seen": true},
		},
	})

	_, err = p.client.Do(req)
	return err
}

func (p *Provider) DeleteEmail(_ context.Context, _ string, uid uint32) error {
	jmapID, err := p.lookupJMAPID(uid)
	if err != nil {
		return err
	}

	trashID, ok := p.roleToID[mailbox.RoleTrash]
	if !ok {
		// No trash, permanently delete
		req := &jmapclient.Request{}
		req.Invoke(&email.Set{
			Account: p.accountID,
			Destroy: []jmapclient.ID{jmapID},
		})
		_, err = p.client.Do(req)
		return err
	}

	// Move to trash
	req := &jmapclient.Request{}
	req.Invoke(&email.Set{
		Account: p.accountID,
		Update: map[jmapclient.ID]jmapclient.Patch{
			jmapID: {"mailboxIds": map[jmapclient.ID]bool{trashID: true}},
		},
	})
	_, err = p.client.Do(req)
	return err
}

func (p *Provider) ArchiveEmail(_ context.Context, _ string, uid uint32) error {
	jmapID, err := p.lookupJMAPID(uid)
	if err != nil {
		return err
	}

	archiveID, ok := p.roleToID[mailbox.RoleArchive]
	if !ok {
		return fmt.Errorf("jmap: no archive mailbox found")
	}

	req := &jmapclient.Request{}
	req.Invoke(&email.Set{
		Account: p.accountID,
		Update: map[jmapclient.ID]jmapclient.Patch{
			jmapID: {"mailboxIds": map[jmapclient.ID]bool{archiveID: true}},
		},
	})
	_, err = p.client.Do(req)
	return err
}

func (p *Provider) MoveEmail(_ context.Context, uid uint32, _, dstFolder string) error {
	jmapID, err := p.lookupJMAPID(uid)
	if err != nil {
		return err
	}

	dstID, err := p.resolveMailboxID(dstFolder)
	if err != nil {
		return err
	}

	req := &jmapclient.Request{}
	req.Invoke(&email.Set{
		Account: p.accountID,
		Update: map[jmapclient.ID]jmapclient.Patch{
			jmapID: {"mailboxIds": map[jmapclient.ID]bool{dstID: true}},
		},
	})
	_, err = p.client.Do(req)
	return err
}

func (p *Provider) DeleteEmails(ctx context.Context, folder string, uids []uint32) error {
	// JMAP can handle batch operations - loop through for now
	for _, uid := range uids {
		if err := p.DeleteEmail(ctx, folder, uid); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) ArchiveEmails(ctx context.Context, folder string, uids []uint32) error {
	// JMAP can handle batch operations - loop through for now
	for _, uid := range uids {
		if err := p.ArchiveEmail(ctx, folder, uid); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) MoveEmails(ctx context.Context, uids []uint32, srcFolder, dstFolder string) error {
	// JMAP can handle batch operations - loop through for now
	for _, uid := range uids {
		if err := p.MoveEmail(ctx, uid, srcFolder, dstFolder); err != nil {
			return err
		}
	}
	return nil
}

func (p *Provider) SendEmail(_ context.Context, msg *backend.OutgoingEmail) error {
	// Build the email as a draft first
	toAddrs := make([]*mail.Address, len(msg.To))
	for i, addr := range msg.To {
		toAddrs[i] = &mail.Address{Email: addr}
	}
	ccAddrs := make([]*mail.Address, len(msg.Cc))
	for i, addr := range msg.Cc {
		ccAddrs[i] = &mail.Address{Email: addr}
	}

	// Build raw RFC5322 message and upload as blob
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "From: %s\r\n", p.account.FormatFromHeader())
	fmt.Fprintf(&buf, "To: %s\r\n", strings.Join(msg.To, ", "))
	if len(msg.Cc) > 0 {
		fmt.Fprintf(&buf, "Cc: %s\r\n", strings.Join(msg.Cc, ", "))
	}
	fmt.Fprintf(&buf, "Subject: %s\r\n", msg.Subject)
	fmt.Fprintf(&buf, "Date: %s\r\n", time.Now().Format(time.RFC1123Z))
	if msg.InReplyTo != "" {
		fmt.Fprintf(&buf, "In-Reply-To: %s\r\n", msg.InReplyTo)
	}
	if len(msg.References) > 0 {
		fmt.Fprintf(&buf, "References: %s\r\n", strings.Join(msg.References, " "))
	}
	fmt.Fprintf(&buf, "MIME-Version: 1.0\r\n")

	body := msg.HTMLBody
	ct := "text/html"
	if body == "" {
		body = msg.PlainBody
		ct = "text/plain"
	}
	fmt.Fprintf(&buf, "Content-Type: %s; charset=utf-8\r\n", ct)
	fmt.Fprintf(&buf, "\r\n%s", body)

	// Upload the blob
	uploadResp, err := p.client.Upload(p.accountID, &buf)
	if err != nil {
		return fmt.Errorf("jmap upload: %w", err)
	}

	// Create the email from the blob via Email/import would be ideal,
	// but we can use Email/set create with the uploaded blob
	draftsID := p.roleToID[mailbox.RoleDrafts]
	if draftsID == "" {
		// Use inbox as fallback
		draftsID = p.roleToID[mailbox.RoleInbox]
	}

	req := &jmapclient.Request{}

	// Import the uploaded blob as an email
	createID := jmapclient.ID("draft")
	req.Invoke(&email.Set{
		Account: p.accountID,
		Create: map[jmapclient.ID]*email.Email{
			createID: {
				BlobID:     uploadResp.ID,
				MailboxIDs: map[jmapclient.ID]bool{draftsID: true},
				Keywords:   map[string]bool{"$draft": true, "$seen": true},
			},
		},
	})

	// Build envelope recipients
	var rcptTo []*emailsubmission.Address
	for _, addr := range msg.To {
		rcptTo = append(rcptTo, &emailsubmission.Address{Email: addr})
	}
	for _, addr := range msg.Cc {
		rcptTo = append(rcptTo, &emailsubmission.Address{Email: addr})
	}
	for _, addr := range msg.Bcc {
		rcptTo = append(rcptTo, &emailsubmission.Address{Email: addr})
	}

	sentID := p.roleToID[mailbox.RoleSent]

	// Submit for sending
	subReq := &emailsubmission.Set{
		Account: p.accountID,
		Create: map[jmapclient.ID]*emailsubmission.EmailSubmission{
			"sub": {
				EmailID: "#draft",
				Envelope: &emailsubmission.Envelope{
					MailFrom: &emailsubmission.Address{Email: p.account.Email},
					RcptTo:   rcptTo,
				},
			},
		},
	}
	if sentID != "" {
		subReq.OnSuccessUpdateEmail = map[jmapclient.ID]jmapclient.Patch{
			"#sub": {
				"mailboxIds":      map[jmapclient.ID]bool{sentID: true},
				"keywords/$draft": nil,
			},
		}
	}
	req.Invoke(subReq)

	_, err = p.client.Do(req)
	return err
}

func (p *Provider) FetchFolders(_ context.Context) ([]backend.Folder, error) {
	if err := p.refreshMailboxes(); err != nil {
		return nil, err
	}

	req := &jmapclient.Request{}
	req.Invoke(&mailbox.Get{
		Account: p.accountID,
	})

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}

	var folders []backend.Folder
	for _, inv := range resp.Responses {
		if r, ok := inv.Args.(*mailbox.GetResponse); ok {
			for _, mbox := range r.List {
				folders = append(folders, backend.Folder{
					Name:      mbox.Name,
					Delimiter: "/",
				})
			}
		}
	}

	return folders, nil
}

func (p *Provider) Watch(_ context.Context, _ string) (<-chan backend.NotifyEvent, func(), error) {
	ch := make(chan backend.NotifyEvent, 16)

	es := &push.EventSource{
		Client: p.client,
		Handler: func(change *jmapclient.StateChange) {
			for _, typeState := range change.Changed {
				for objType := range typeState {
					if objType == "Email" || objType == "Mailbox" {
						ch <- backend.NotifyEvent{
							Type:      backend.NotifyNewEmail,
							AccountID: p.account.ID,
						}
					}
				}
			}
		},
		Ping: 30,
	}

	go func() {
		defer close(ch)
		_ = es.Listen()
	}()

	cancel := func() {
		es.Close()
	}

	return ch, cancel, nil
}

func (p *Provider) Close() error {
	return nil
}

// Verify interface compliance at compile time.
var _ backend.Provider = (*Provider)(nil)

// lookupJMAPID resolves a uint32 UID hash back to the JMAP string ID.
func (p *Provider) lookupJMAPID(uid uint32) (jmapclient.ID, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	id, ok := p.idToJMAPID[uid]
	if !ok {
		return "", fmt.Errorf("jmap: no cached ID for UID %d", uid)
	}
	return id, nil
}

// jmapIDToUID converts a JMAP string ID to a uint32 hash for use as a UID.
func jmapIDToUID(id jmapclient.ID) uint32 {
	h := fnv.New32a()
	h.Write([]byte(id))
	v := h.Sum32()
	if v == 0 {
		v = 1
	}
	return v
}

// jmapEmailToBackend converts a JMAP email to a backend.Email.
func jmapEmailToBackend(eml *email.Email, uid uint32, accountID string) backend.Email {
	e := backend.Email{
		UID:       uid,
		Subject:   eml.Subject,
		Date:      safeTime(eml.ReceivedAt),
		IsRead:    eml.Keywords["$seen"],
		AccountID: accountID,
	}
	if len(eml.From) > 0 {
		e.From = eml.From[0].String()
	}
	for _, addr := range eml.To {
		e.To = append(e.To, addr.Email)
	}
	for _, addr := range eml.ReplyTo {
		e.ReplyTo = append(e.ReplyTo, addr.Email)
	}
	if len(eml.MessageID) > 0 {
		e.MessageID = eml.MessageID[0]
	}
	return e
}

func safeTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
