package fetcher

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime"
	"mime/quotedprintable"
	"net/textproto"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-pgpmail"
	"github.com/floatpane/matcha/config"
	"go.mozilla.org/pkcs7"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/transform"
)

// debugIMAPFile holds a single shared file handle for IMAP debug logging,
// opened once via debugIMAPOnce to avoid leaking a descriptor per connection.
var (
	debugIMAPFile *os.File
	debugIMAPOnce sync.Once
)

func getDebugIMAPWriter() io.Writer {
	debugIMAPOnce.Do(func() {
		if path := os.Getenv("DEBUG_IMAP"); path != "" {
			f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
			if err == nil {
				debugIMAPFile = f
			}
		}
	})
	if debugIMAPFile != nil {
		return debugIMAPFile
	}
	return nil
}

// Attachment holds data for an email attachment.
type Attachment struct {
	Filename         string
	PartID           string // Keep PartID to fetch on demand
	Data             []byte
	Encoding         string      // Store encoding for proper decoding
	MIMEType         string      // Full MIME type (e.g., image/png)
	ContentID        string      // Content-ID for inline assets (e.g., cid: references)
	Inline           bool        // True when the part is meant to be displayed inline
	IsSMIMESignature bool        // True if this attachment is an S/MIME signature
	SMIMEVerified    bool        // True if the S/MIME signature was verified successfully
	IsSMIMEEncrypted bool        // True if the S/MIME content was successfully decrypted
	IsPGPSignature   bool        // True if this attachment is a PGP signature
	PGPVerified      bool        // True if the PGP signature was verified successfully
	IsPGPEncrypted   bool        // True if the PGP content was successfully decrypted
	IsCalendarInvite bool        // True if this attachment is a calendar invite (.ics)
	CalendarEvent    interface{} // Parsed calendar event (calendar.Event pointer)
}

type Email struct {
	UID         uint32
	From        string
	To          []string
	ReplyTo     []string
	Subject     string
	Body        string
	Date        time.Time
	IsRead      bool
	MessageID   string
	References  []string
	Attachments []Attachment
	AccountID   string // ID of the account this email belongs to
}

// Folder represents an IMAP mailbox/folder.
type Folder struct {
	Name       string
	Delimiter  string
	Attributes []string
}

// formatAddress returns "Name <email>" when a Name is present,
// otherwise just "email".
func formatAddress(addr imap.Address) string {
	email := addr.Addr()
	if addr.Name != "" {
		return addr.Name + " <" + email + ">"
	}
	return email
}

func hasSeenFlag(flags []imap.Flag) bool {
	return slices.Contains(flags, imap.FlagSeen)
}

// normalizeGmailAddress canonicalizes a Gmail address by stripping the "+tag"
// subaddress and removing dots from the local part. Gmail treats
// "u.s.e.r+tag@gmail.com" and "user@gmail.com" as the same mailbox.
func normalizeGmailAddress(addr string) string {
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return addr
	}
	local, domain := addr[:at], addr[at:]
	if plus := strings.Index(local, "+"); plus >= 0 {
		local = local[:plus]
	}
	local = strings.ReplaceAll(local, ".", "")
	return local + domain
}

// addressMatches reports whether candidate matches the configured fetch email.
// For Gmail accounts, subaddressed forms ("local+tag@gmail.com") and dotted
// forms ("l.o.c.a.l@gmail.com") also match.
// fetchEmail must already be lowercased and trimmed.
func addressMatches(candidate, fetchEmail string, account *config.Account) bool {
	candidate = strings.ToLower(strings.TrimSpace(candidate))
	if candidate == "" || fetchEmail == "" {
		return false
	}
	if candidate == fetchEmail {
		return true
	}
	if account != nil && strings.EqualFold(account.ServiceProvider, "gmail") {
		return normalizeGmailAddress(candidate) == normalizeGmailAddress(fetchEmail)
	}
	return false
}

// deliveryHeadersMatch checks if any of the Delivered-To, X-Forwarded-To, or
// X-Original-To headers contain the given email address. This catches
// auto-forwarded emails where the envelope To/Cc don't match the local account.
func deliveryHeadersMatch(data []byte, fetchEmail string, account *config.Account) bool {
	if len(data) == 0 {
		return false
	}
	// Parse as MIME headers
	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(data)))
	headers, err := reader.ReadMIMEHeader()
	if err != nil && len(headers) == 0 {
		return false
	}
	for _, key := range []string{"Delivered-To", "X-Forwarded-To", "X-Original-To"} {
		for _, val := range headers.Values(key) {
			if addressMatches(val, fetchEmail, account) {
				return true
			}
		}
	}
	return false
}

func decodePart(reader io.Reader, header mail.PartHeader) (string, error) {
	contentType := header.Get("Content-Type")
	mediaType, params, parseErr := mime.ParseMediaType(contentType)

	charset := "utf-8"
	if parseErr != nil {
		charset = bestEffortCharset(contentType)
	} else if params["charset"] != "" {
		charset = strings.ToLower(params["charset"])
	}

	decodedBody, err := decodeReaderWithCharset(reader, charset)
	if err != nil {
		return "", err
	}

	if parseErr == nil && strings.HasPrefix(mediaType, "multipart/") {
		return "[This is a multipart message]", nil
	}

	return string(decodedBody), nil
}

func decodeReaderWithCharset(reader io.Reader, charset string) ([]byte, error) {
	encoding, err := ianaindex.IANA.Encoding(charset)
	if err != nil || encoding == nil {
		encoding, _ = ianaindex.IANA.Encoding("utf-8")
	}

	transformReader := transform.NewReader(reader, encoding.NewDecoder())
	return ioutil.ReadAll(transformReader)
}

func bestEffortCharset(contentType string) string {
	for _, param := range strings.Split(contentType, ";") {
		key, value, found := strings.Cut(param, "=")
		if !found || !strings.EqualFold(strings.TrimSpace(key), "charset") {
			continue
		}

		value = strings.Trim(strings.TrimSpace(value), `"`)
		if value != "" {
			return strings.ToLower(value)
		}
	}

	return "utf-8"
}

func decodeHeader(header string) string {
	dec := new(mime.WordDecoder)
	dec.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		encoding, err := ianaindex.IANA.Encoding(charset)
		if err != nil {
			return nil, err
		}
		return transform.NewReader(input, encoding.NewDecoder()), nil
	}
	decoded, err := dec.DecodeHeader(header)
	if err != nil {
		return header
	}
	return decoded
}

func decodeAttachmentData(rawBytes []byte, encoding string) ([]byte, error) {
	switch strings.ToLower(encoding) {
	case "base64":
		decoder := base64.NewDecoder(base64.StdEncoding, bytes.NewReader(rawBytes))
		data, err := ioutil.ReadAll(decoder)
		if err != nil {
			return nil, err
		}
		return data, nil
	case "quoted-printable":
		data, err := ioutil.ReadAll(quotedprintable.NewReader(bytes.NewReader(rawBytes)))
		if err != nil {
			return nil, err
		}
		return data, nil
	default:
		return rawBytes, nil
	}
}

// parsePartID converts a string part ID like "1.2.3" to []int{1, 2, 3}.
// Special cases: "TEXT" maps to empty with PartSpecifierText (handled by caller).
func parsePartID(partID string) []int {
	if partID == "" || partID == "TEXT" {
		return nil
	}
	var parts []int
	for _, s := range strings.Split(partID, ".") {
		n := 0
		for _, c := range s {
			if c >= '0' && c <= '9' {
				n = n*10 + int(c-'0')
			}
		}
		parts = append(parts, n)
	}
	return parts
}

// formatPartPath converts a Walk path like []int{1, 2, 3} to "1.2.3".
func formatPartPath(path []int) string {
	if len(path) == 0 {
		return "1"
	}
	parts := make([]string, len(path))
	for i, p := range path {
		parts[i] = fmt.Sprintf("%d", p)
	}
	return strings.Join(parts, ".")
}

// getBodyStructureBoundary extracts the boundary parameter from a multipart body structure.
func getBodyStructureBoundary(bs imap.BodyStructure) string {
	if mp, ok := bs.(*imap.BodyStructureMultiPart); ok {
		if mp.Extended != nil && mp.Extended.Params != nil {
			return mp.Extended.Params["boundary"]
		}
	}
	return ""
}

// uidsToUIDSet converts a slice of uint32 UIDs to an imap.UIDSet.
func uidsToUIDSet(uids []uint32) imap.UIDSet {
	var uidSet imap.UIDSet
	for _, uid := range uids {
		uidSet.AddNum(imap.UID(uid))
	}
	return uidSet
}

func connectWithHandler(account *config.Account, handler *imapclient.UnilateralDataHandler) (*imapclient.Client, error) {
	return connectWithOptions(account, &imapclient.Options{
		UnilateralDataHandler: handler,
	})
}

func connect(account *config.Account) (*imapclient.Client, error) {
	return connectWithOptions(account, nil)
}

func connectWithOptions(account *config.Account, extraOpts *imapclient.Options) (*imapclient.Client, error) {
	imapServer := account.GetIMAPServer()
	imapPort := account.GetIMAPPort()

	if imapServer == "" {
		return nil, fmt.Errorf("unsupported service_provider: %s", account.ServiceProvider)
	}

	addr := fmt.Sprintf("%s:%d", imapServer, imapPort)

	options := &imapclient.Options{
		TLSConfig: &tls.Config{
			ServerName:         imapServer,
			InsecureSkipVerify: account.Insecure,
			MinVersion:         tls.VersionTLS12,
		},
	}
	if extraOpts != nil {
		options.UnilateralDataHandler = extraOpts.UnilateralDataHandler
		options.DebugWriter = extraOpts.DebugWriter
	}
	if w := getDebugIMAPWriter(); w != nil {
		options.DebugWriter = w
	}

	var c *imapclient.Client
	var err error

	// If using standard non-implicit ports (1143 or 143), use DialStartTLS
	if imapPort == 1143 || imapPort == 143 {
		c, err = imapclient.DialStartTLS(addr, options)
		if err != nil {
			return nil, err
		}
	} else {
		// Otherwise default to implicit TLS (port 993)
		c, err = imapclient.DialTLS(addr, options)
		if err != nil {
			return nil, err
		}
	}

	if err := c.WaitGreeting(); err != nil {
		c.Close()
		return nil, err
	}

	// Authenticate using OAuth2 (XOAUTH2) or plain password
	if account.IsOAuth2() {
		token, err := config.GetOAuth2Token(account.Email)
		if err != nil {
			return nil, fmt.Errorf("oauth2: %w", err)
		}
		if err := c.Authenticate(newXOAuth2Client(account.Email, token)); err != nil {
			return nil, fmt.Errorf("XOAUTH2 authentication failed: %w", err)
		}
	} else {
		if err := c.Login(account.Email, account.Password).Wait(); err != nil {
			return nil, fmt.Errorf("authentication error: %w", err)
		}
	}

	return c, nil
}

func getSentMailbox(account *config.Account) string {
	switch account.ServiceProvider {
	case "gmail":
		return "[Gmail]/Sent Mail"
	case "outlook":
		return "Sent Items"
	case "icloud":
		return "Sent Messages"
	default:
		return "Sent"
	}
}

// getMailboxByAttr finds a mailbox with the given IMAP attribute (e.g., \All, \Sent, \Trash).
func getMailboxByAttr(c *imapclient.Client, attr imap.MailboxAttr) (string, error) {
	listCmd := c.List("", "*", nil)
	defer listCmd.Close()

	var foundMailbox string
	for {
		data := listCmd.Next()
		if data == nil {
			break
		}
		for _, a := range data.Attrs {
			if a == attr {
				foundMailbox = data.Mailbox
				break
			}
		}
	}

	if err := listCmd.Close(); err != nil {
		return "", err
	}

	if foundMailbox == "" {
		return "", fmt.Errorf("no mailbox found with attribute %s", attr)
	}

	return foundMailbox, nil
}

func FetchMailboxEmails(account *config.Account, mailbox string, limit, offset uint32) ([]Email, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	selectData, err := c.Select(mailbox, nil).Wait()
	if err != nil {
		return nil, err
	}

	if selectData.NumMessages == 0 {
		return []Email{}, nil
	}

	var allEmails []Email

	// Start from the top minus offset
	cursor := uint32(0)
	if selectData.NumMessages > offset {
		cursor = selectData.NumMessages - offset
	} else {
		return []Email{}, nil
	}

	// Determine if we should filter
	fetchEmail := strings.ToLower(strings.TrimSpace(account.FetchEmail))
	if fetchEmail == "" {
		fetchEmail = strings.ToLower(strings.TrimSpace(account.Email))
	}
	isSentMailbox := mailbox == getSentMailbox(account)

	// Delivery header section for matching auto-forwarded emails
	deliveryHeaderSection := &imap.FetchItemBodySection{
		Specifier:    imap.PartSpecifierHeader,
		HeaderFields: []string{"Delivered-To", "X-Forwarded-To", "X-Original-To"},
		Peek:         true,
	}

	// Loop until we have enough emails or run out of messages
	for len(allEmails) < int(limit) && cursor > 0 {
		// Determine chunk size
		chunkSize := limit
		if chunkSize < 50 {
			chunkSize = 50
		}

		from := uint32(1)
		if cursor > uint32(chunkSize) {
			from = cursor - uint32(chunkSize) + 1
		}

		var seqset imap.SeqSet
		seqset.AddRange(from, cursor)

		fetchCmd := c.Fetch(seqset, &imap.FetchOptions{
			Envelope:    true,
			UID:         true,
			Flags:       true,
			BodySection: []*imap.FetchItemBodySection{deliveryHeaderSection},
		})

		batchMsgs, err := fetchCmd.Collect()
		if err != nil {
			return nil, err
		}

		// Filter messages in this batch
		var batchEmails []Email
		for _, msg := range batchMsgs {
			if msg.Envelope == nil {
				continue
			}

			var fromAddr string
			if len(msg.Envelope.From) > 0 {
				fromAddr = formatAddress(msg.Envelope.From[0])
			}

			var toAddrList []string
			for _, addr := range msg.Envelope.To {
				toAddrList = append(toAddrList, addr.Addr())
			}
			for _, addr := range msg.Envelope.Cc {
				toAddrList = append(toAddrList, addr.Addr())
			}

			var replyToAddrList []string
			for _, addr := range msg.Envelope.ReplyTo {
				replyToAddrList = append(replyToAddrList, addr.Addr())
			}

			matched := false
			if account.CatchAll {
				matched = true
			} else if isSentMailbox {
				var senderEmail string
				if len(msg.Envelope.From) > 0 {
					senderEmail = msg.Envelope.From[0].Addr()
				}
				if addressMatches(senderEmail, fetchEmail, account) {
					matched = true
				}
			} else {
				for _, r := range toAddrList {
					if addressMatches(r, fetchEmail, account) {
						matched = true
						break
					}
				}
				// Check delivery headers for auto-forwarded emails
				if !matched {
					headerData := msg.FindBodySection(deliveryHeaderSection)
					matched = deliveryHeadersMatch(headerData, fetchEmail, account)
				}
			}

			if !matched {
				continue
			}

			batchEmails = append(batchEmails, Email{
				UID:       uint32(msg.UID),
				From:      fromAddr,
				To:        toAddrList,
				ReplyTo:   replyToAddrList,
				Subject:   decodeHeader(msg.Envelope.Subject),
				Date:      msg.Envelope.Date,
				IsRead:    hasSeenFlag(msg.Flags),
				AccountID: account.ID,
			})
		}

		// Sort batch Newest -> Oldest by UID desc
		sort.Slice(batchEmails, func(i, j int) bool {
			return batchEmails[i].UID > batchEmails[j].UID
		})

		allEmails = append(allEmails, batchEmails...)
		cursor = from - 1
	}

	// Trim if we have too many
	if len(allEmails) > int(limit) {
		allEmails = allEmails[:limit]
	}

	return allEmails, nil
}

func FetchEmailBodyFromMailbox(account *config.Account, mailbox string, uid uint32) (string, []Attachment, error) {
	c, err := connect(account)
	if err != nil {
		return "", nil, err
	}
	defer c.Close()

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return "", nil, err
	}

	uidSet := imap.UIDSetNum(imap.UID(uid))

	fetchWholeMessage := func() ([]byte, error) {
		wholeSection := &imap.FetchItemBodySection{Peek: true}
		fetchCmd := c.Fetch(uidSet, &imap.FetchOptions{
			BodySection: []*imap.FetchItemBodySection{wholeSection},
		})
		msgs, err := fetchCmd.Collect()
		if err != nil {
			return nil, err
		}
		if len(msgs) > 0 {
			if data := msgs[0].FindBodySection(wholeSection); data != nil {
				return data, nil
			}
		}
		return nil, fmt.Errorf("could not fetch whole message")
	}

	fetchInlinePart := func(partID, encoding string) ([]byte, error) {
		part := parsePartID(partID)
		section := &imap.FetchItemBodySection{
			Part: part,
			Peek: true,
		}

		fetchCmd := c.Fetch(uidSet, &imap.FetchOptions{
			BodySection: []*imap.FetchItemBodySection{section},
		})
		msgs, err := fetchCmd.Collect()
		if err != nil {
			return nil, err
		}

		if len(msgs) == 0 {
			return nil, fmt.Errorf("could not fetch inline part %s", partID)
		}

		rawBytes := msgs[0].FindBodySection(section)
		if rawBytes == nil {
			return nil, fmt.Errorf("could not get inline part body %s", partID)
		}

		return decodeAttachmentData(rawBytes, encoding)
	}

	fetchCmd := c.Fetch(uidSet, &imap.FetchOptions{
		BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
	})
	bsMsgs, err := fetchCmd.Collect()
	if err != nil {
		return "", nil, err
	}

	if len(bsMsgs) == 0 || bsMsgs[0].BodyStructure == nil {
		return "", nil, fmt.Errorf("no message or body structure found with UID %d", uid)
	}

	msg := bsMsgs[0]

	var plainPartID, plainPartEncoding string
	var htmlPartID, htmlPartEncoding string
	var attachments []Attachment
	var extractedBody string // Used if we intercept and decrypt a payload

	var checkPart func(part *imap.BodyStructureSinglePart, partID string)
	checkPart = func(part *imap.BodyStructureSinglePart, partID string) {
		// Check for text content (prefer html over plain)
		if strings.EqualFold(part.Type, "text") {
			sub := strings.ToLower(part.Subtype)
			switch sub {
			case "html":
				if htmlPartID == "" {
					htmlPartID = partID
					htmlPartEncoding = part.Encoding
				}
			case "plain":
				if plainPartID == "" {
					plainPartID = partID
					plainPartEncoding = part.Encoding
				}
			}
		}

		// Check for attachments using multiple methods
		filename := part.Filename()
		// Fallback: check Params (for name parameter)
		if filename == "" {
			if fn, ok := part.Params["name"]; ok && fn != "" {
				filename = fn
			}
		}
		// Fallback: check Params for filename
		if filename == "" {
			if fn, ok := part.Params["filename"]; ok && fn != "" {
				filename = fn
			}
		}

		// Add as attachment if it has a disposition or a filename (and not just plain text).
		// Allow inline parts without filenames (common for cid images).
		contentID := strings.Trim(part.ID, "<>")
		mimeType := part.MediaType()
		dispValue := ""
		dispParams := map[string]string{}
		if part.Disposition() != nil {
			dispValue = part.Disposition().Value
			dispParams = part.Disposition().Params
		}
		_ = dispParams // used below in attachment fallback checks
		isCID := contentID != ""
		isInline := strings.EqualFold(dispValue, "inline") || isCID

		if filename == "" && isInline && strings.HasPrefix(mimeType, "image/") {
			filename = "inline"
		}

		// === S/MIME ENCRYPTION AND OPAQUE VERIFICATION ===
		if filename == "smime.p7m" || mimeType == "application/pkcs7-mime" {
			data, err := fetchInlinePart(partID, part.Encoding)
			if err != nil && partID == "1" {
				// Fallback for single-part messages where PEEK[1] fails
				data, err = fetchInlinePart("TEXT", part.Encoding)
			}

			if err != nil {
				extractedBody = fmt.Sprintf("**S/MIME Error:** Failed to fetch encrypted part from IMAP server: %v\n", err)
				htmlPartID = "extracted"
			} else {
				p7, parseErr := pkcs7.Parse(data)
				if parseErr != nil {
					// Fallback: IMAP servers sometimes drop the transfer-encoding header.
					// We manually strip newlines and attempt a base64 decode just in case.
					cleanData := bytes.ReplaceAll(data, []byte("\n"), []byte(""))
					cleanData = bytes.ReplaceAll(cleanData, []byte("\r"), []byte(""))
					if decoded, b64err := base64.StdEncoding.DecodeString(string(cleanData)); b64err == nil {
						p7, parseErr = pkcs7.Parse(decoded)
					}
				}

				if parseErr != nil {
					extractedBody = fmt.Sprintf("**S/MIME Error:** Failed to parse PKCS7 payload: %v\n", parseErr)
					htmlPartID = "extracted"
				} else {
					var innerBytes []byte
					isEncrypted, isOpaqueSigned, smimeTrusted := false, false, false
					decryptionErr := ""

					// 1. Try to Decrypt
					if account.SMIMECert != "" && account.SMIMEKey != "" {
						cData, err1 := os.ReadFile(account.SMIMECert)
						kData, err2 := os.ReadFile(account.SMIMEKey)
						if err1 != nil || err2 != nil {
							decryptionErr = fmt.Sprintf("Failed to read cert/key files. Cert: %v, Key: %v", err1, err2)
						} else {
							cBlock, _ := pem.Decode(cData)
							kBlock, _ := pem.Decode(kData)
							if cBlock == nil || kBlock == nil {
								decryptionErr = "Failed to decode PEM blocks from cert/key files."
							} else {
								cert, err3 := x509.ParseCertificate(cBlock.Bytes)
								var privKey any
								var err4 error
								if key, err := x509.ParsePKCS8PrivateKey(kBlock.Bytes); err == nil {
									privKey = key
								} else if key, err := x509.ParsePKCS1PrivateKey(kBlock.Bytes); err == nil {
									privKey = key
								} else if key, err := x509.ParseECPrivateKey(kBlock.Bytes); err == nil {
									privKey = key
								} else {
									err4 = errors.New("unsupported private key format")
								}

								if err3 != nil || err4 != nil {
									decryptionErr = fmt.Sprintf("Failed to parse cert/key. Cert: %v, Key: %v", err3, err4)
								} else {
									dec, err := p7.Decrypt(cert, privKey)
									if err == nil {
										innerBytes = dec
										isEncrypted = true
									} else {
										decryptionErr = fmt.Sprintf("PKCS7 Decrypt failed: %v", err)
									}
								}
							}
						}
					} else {
						// Only set error if it actually is enveloped data (encrypted)
						// If it's just opaque signed, we shouldn't error out.
						decryptionErr = "S/MIME Cert or Key path is missing in settings."
					}

					// 2. If not encrypted, check if it's an opaque signature
					if !isEncrypted && len(p7.Signers) > 0 {
						isOpaqueSigned = true
						innerBytes = p7.Content
						decryptionErr = "" // Clear encryption error because it wasn't encrypted to begin with
						roots, _ := x509.SystemCertPool()
						if roots == nil {
							roots = x509.NewCertPool()
						}
						if err := p7.VerifyWithChain(roots); err == nil {
							smimeTrusted = true
						}
					}

					// 3. Parse Inner MIME payload
					if len(innerBytes) > 0 {
						mr, err := mail.CreateReader(bytes.NewReader(innerBytes))
						if err == nil {
							for {
								p, err := mr.NextPart()
								if err != nil {
									break
								}
								cType, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
								disp, dParams, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
								b, readErr := io.ReadAll(p.Body) // Auto-decodes quoted-printable/base64
								if readErr != nil {
									log.Printf("fetcher: reading inner MIME part body: %v", readErr)
									continue
								}

								if disp == "attachment" || disp == "inline" || (!strings.HasPrefix(cType, "multipart/") && cType != "text/plain" && cType != "text/html") {
									fn := dParams["filename"]
									if fn == "" {
										_, cp, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
										fn = cp["name"]
									}
									attachments = append(attachments, Attachment{
										Filename: fn, Data: b, MIMEType: cType, Inline: disp == "inline",
									})
								} else {
									if cType == "text/html" {
										extractedBody = string(b)
										htmlPartID = "extracted" // Skip IMAP fetch
									} else if cType == "text/plain" && extractedBody == "" {
										extractedBody = string(b)
										plainPartID = "extracted"
									}
								}
							}
						} else {
							extractedBody = fmt.Sprintf("**S/MIME Error:** Failed to read inner decrypted MIME: %v\n\n```\n%s\n```", err, string(innerBytes))
							htmlPartID = "extracted"
						}

						attachments = append(attachments, Attachment{
							Filename:         "smime-status.internal",
							IsSMIMESignature: isOpaqueSigned,
							SMIMEVerified:    smimeTrusted,
							IsSMIMEEncrypted: isEncrypted,
						})
						return // Stop checking IMAP structure, we hijacked it
					} else {
						extractedBody = fmt.Sprintf("**S/MIME Decryption Failed:** %s\n", decryptionErr)
						htmlPartID = "extracted"
					}
				}
			}
		}

		// === S/MIME DETACHED SIGNATURE VERIFICATION ===
		if filename == "smime.p7s" || mimeType == "application/pkcs7-signature" {
			att := Attachment{
				Filename:         filename,
				PartID:           partID,
				Encoding:         part.Encoding,
				MIMEType:         mimeType,
				ContentID:        contentID,
				Inline:           isInline,
				IsSMIMESignature: true,
			}
			if data, err := fetchInlinePart(partID, part.Encoding); err == nil {
				att.Data = data
				p7, err := pkcs7.Parse(data)
				if err == nil {
					boundary := getBodyStructureBoundary(msg.BodyStructure)
					if boundary != "" {
						rawEmail, err := fetchWholeMessage()
						if err == nil {
							fullBoundary := []byte("--" + boundary)
							firstIdx := bytes.Index(rawEmail, fullBoundary)
							if firstIdx != -1 {
								startIdx := firstIdx + len(fullBoundary)
								if startIdx < len(rawEmail) && rawEmail[startIdx] == '\r' {
									startIdx++
								}
								if startIdx < len(rawEmail) && rawEmail[startIdx] == '\n' {
									startIdx++
								}
								secondIdx := bytes.Index(rawEmail[startIdx:], fullBoundary)
								if secondIdx != -1 {
									endIdx := startIdx + secondIdx
									if endIdx > 0 && rawEmail[endIdx-1] == '\n' {
										endIdx--
									}
									if endIdx > 0 && rawEmail[endIdx-1] == '\r' {
										endIdx--
									}
									signedData := rawEmail[startIdx:endIdx]
									canonical := bytes.ReplaceAll(signedData, []byte("\r\n"), []byte("\n"))
									canonical = bytes.ReplaceAll(canonical, []byte("\n"), []byte("\r\n"))

									roots, _ := x509.SystemCertPool()
									if roots == nil {
										roots = x509.NewCertPool()
									}

									p7.Content = canonical
									if err := p7.VerifyWithChain(roots); err == nil {
										att.SMIMEVerified = true
									} else {
										p7.Content = append(canonical, '\r', '\n')
										if err := p7.VerifyWithChain(roots); err == nil {
											att.SMIMEVerified = true
										} else {
											p7.Content = bytes.TrimRight(canonical, "\r\n")
											if err := p7.VerifyWithChain(roots); err == nil {
												att.SMIMEVerified = true
											}
										}
									}
								}
							}
						}
					}
				}
			}
			attachments = append(attachments, att)
		}

		// === PGP ENCRYPTED MESSAGE DETECTION ===
		if mimeType == "application/pgp-encrypted" || (mimeType == "multipart/encrypted" && strings.Contains(part.Subtype, "pgp")) {
			// PGP encrypted messages typically have two parts:
			// 1. Version info (application/pgp-encrypted)
			// 2. Encrypted data (application/octet-stream)
			// We'll handle decryption when we find the encrypted data part
			// Skip this part and continue processing
		}

		// Detect encrypted data part of PGP message
		if strings.Contains(filename, ".asc") || (mimeType == "application/octet-stream" && part.Encoding == "7bit") {
			// This might be PGP encrypted data
			data, err := fetchInlinePart(partID, part.Encoding)
			if err == nil && bytes.Contains(data, []byte("-----BEGIN PGP MESSAGE-----")) {
				// This is PGP encrypted content
				if account.PGPPrivateKey != "" {
					decrypted, err := decryptPGPMessage(data, account)
					if err == nil {
						// Parse the decrypted MIME content
						mr, err := mail.CreateReader(bytes.NewReader(decrypted))
						if err == nil {
							for {
								p, err := mr.NextPart()
								if err == io.EOF {
									break
								}
								if err != nil {
									break
								}

								switch h := p.Header.(type) {
								case *mail.InlineHeader:
									ct, _, _ := h.ContentType()
									if strings.HasPrefix(ct, "text/html") {
										body, _ := io.ReadAll(p.Body)
										extractedBody = string(body)
										htmlPartID = "decrypted"
									} else if strings.HasPrefix(ct, "text/plain") && extractedBody == "" {
										body, _ := io.ReadAll(p.Body)
										extractedBody = string(body)
										htmlPartID = "decrypted"
									}
								}
							}

							// Add status marker
							attachments = append(attachments, Attachment{
								Filename:       "pgp-status.internal",
								IsPGPEncrypted: true,
								PGPVerified:    true, // Decryption succeeded
							})
						}
					} else {
						extractedBody = fmt.Sprintf("**PGP Decryption Failed:** %s\n", err)
						htmlPartID = "extracted"
					}
				} else {
					extractedBody = "**PGP Encrypted:** Private key not configured\n"
					htmlPartID = "extracted"
				}
			}
		}

		// === PGP DETACHED SIGNATURE VERIFICATION ===
		if filename == "signature.asc" || mimeType == "application/pgp-signature" {
			att := Attachment{
				Filename:       filename,
				PartID:         partID,
				Encoding:       part.Encoding,
				MIMEType:       mimeType,
				ContentID:      contentID,
				Inline:         isInline,
				IsPGPSignature: true,
			}

			if data, err := fetchInlinePart(partID, part.Encoding); err == nil {
				att.Data = data

				// Try to verify the signature
				boundary := getBodyStructureBoundary(msg.BodyStructure)
				if boundary != "" {
					rawEmail, err := fetchWholeMessage()
					if err == nil {
						// Extract signed content (similar to S/MIME)
						fullBoundary := []byte("--" + boundary)
						firstIdx := bytes.Index(rawEmail, fullBoundary)
						if firstIdx != -1 {
							startIdx := firstIdx + len(fullBoundary)
							if startIdx < len(rawEmail) && rawEmail[startIdx] == '\r' {
								startIdx++
							}
							if startIdx < len(rawEmail) && rawEmail[startIdx] == '\n' {
								startIdx++
							}
							secondIdx := bytes.Index(rawEmail[startIdx:], fullBoundary)
							if secondIdx != -1 {
								endIdx := startIdx + secondIdx
								if endIdx > 0 && rawEmail[endIdx-1] == '\n' {
									endIdx--
								}
								if endIdx > 0 && rawEmail[endIdx-1] == '\r' {
									endIdx--
								}
								signedData := rawEmail[startIdx:endIdx]

								// Verify PGP signature
								verified := verifyPGPSignature(signedData, data, account)
								att.PGPVerified = verified
							}
						}
					}
				}
			}
			attachments = append(attachments, att)
		} else if mimeType == "text/calendar" || strings.HasSuffix(strings.ToLower(filename), ".ics") {
			// === CALENDAR INVITE DETECTION ===
			att := Attachment{
				Filename:         filename,
				PartID:           partID,
				Encoding:         part.Encoding,
				MIMEType:         mimeType,
				IsCalendarInvite: true,
			}

			// Fetch and parse calendar data
			if data, err := fetchInlinePart(partID, part.Encoding); err == nil {
				att.Data = data
				// Parse will be done lazily in calendar package when needed
			}
			attachments = append(attachments, att)
		} else if (filename != "" || isCID) && (strings.EqualFold(dispValue, "attachment") || isInline || !strings.EqualFold(part.Type, "text")) {
			att := Attachment{
				Filename:  filename,
				PartID:    partID,
				Encoding:  part.Encoding, // Store encoding for proper decoding
				MIMEType:  mimeType,
				ContentID: contentID,
				Inline:    isInline,
			}
			if att.Inline && strings.HasPrefix(att.MIMEType, "image/") {
				if data, err := fetchInlinePart(partID, part.Encoding); err == nil {
					att.Data = data
				}
			}
			attachments = append(attachments, att)
		}
	}

	// Walk the body structure tree
	msg.BodyStructure.Walk(func(path []int, part imap.BodyStructure) bool {
		if sp, ok := part.(*imap.BodyStructureSinglePart); ok {
			partID := formatPartPath(path)
			checkPart(sp, partID)
		}
		return true
	})

	// If we hijacked and decrypted the body, return it immediately
	if extractedBody != "" {
		return extractedBody, attachments, nil
	}

	var body string
	textPartID := ""
	textPartEncoding := ""
	if htmlPartID != "" {
		textPartID = htmlPartID
		textPartEncoding = htmlPartEncoding
	} else if plainPartID != "" {
		textPartID = plainPartID
		textPartEncoding = plainPartEncoding
	}
	if os.Getenv("DEBUG_KITTY_IMAGES") != "" {
		msg := fmt.Sprintf("[kitty-img] body selection html=%s plain=%s chosen=%s\n", htmlPartID, plainPartID, textPartID)
		log.Print(msg)
		if path := os.Getenv("DEBUG_KITTY_LOG"); path != "" {
			// Use a closure with defer so a panic between open and
			// WriteString doesn't leak the file descriptor (#894).
			func() {
				f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return
				}
				defer f.Close()
				_, _ = f.WriteString(msg)
			}()
		}
	}
	if textPartID != "" {
		part := parsePartID(textPartID)
		section := &imap.FetchItemBodySection{
			Part: part,
			Peek: true,
		}

		fetchCmd := c.Fetch(uidSet, &imap.FetchOptions{
			BodySection: []*imap.FetchItemBodySection{section},
		})
		msgs, err := fetchCmd.Collect()
		if err != nil {
			return "", nil, err
		}

		if len(msgs) > 0 {
			if buf := msgs[0].FindBodySection(section); buf != nil {
				// Use the encoding from BodyStructure to decode
				if decoded, err := decodeAttachmentData(buf, textPartEncoding); err == nil {
					body = string(decoded)
				} else {
					body = string(buf)
				}
			}
		}
	}

	return body, attachments, nil
}

func FetchAttachmentFromMailbox(account *config.Account, mailbox string, uid uint32, partID string, encoding string) ([]byte, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return nil, err
	}

	uidSet := imap.UIDSetNum(imap.UID(uid))
	part := parsePartID(partID)
	section := &imap.FetchItemBodySection{
		Part: part,
		Peek: true,
	}

	fetchCmd := c.Fetch(uidSet, &imap.FetchOptions{
		BodySection: []*imap.FetchItemBodySection{section},
	})
	msgs, err := fetchCmd.Collect()
	if err != nil {
		return nil, err
	}

	if len(msgs) == 0 {
		return nil, fmt.Errorf("could not fetch attachment")
	}

	rawBytes := msgs[0].FindBodySection(section)
	if rawBytes == nil {
		return nil, fmt.Errorf("could not get attachment body")
	}

	decoded, err := decodeAttachmentData(rawBytes, encoding)
	if err != nil {
		return rawBytes, nil
	}
	return decoded, nil
}

func moveEmail(account *config.Account, uid uint32, sourceMailbox, destMailbox string) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Close()

	if _, err := c.Select(sourceMailbox, nil).Wait(); err != nil {
		return err
	}

	uidSet := imap.UIDSetNum(imap.UID(uid))
	_, err = c.Move(uidSet, destMailbox).Wait()
	return err
}

func MarkEmailAsReadInMailbox(account *config.Account, mailbox string, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Close()

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return err
	}

	uidSet := imap.UIDSetNum(imap.UID(uid))
	return c.Store(uidSet, &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagSeen},
	}, nil).Close()
}

func DeleteEmailFromMailbox(account *config.Account, mailbox string, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Close()

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return err
	}

	uidSet := imap.UIDSetNum(imap.UID(uid))
	if err := c.Store(uidSet, &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagDeleted},
	}, nil).Close(); err != nil {
		return err
	}

	return c.Expunge().Close()
}

func ArchiveEmailFromMailbox(account *config.Account, mailbox string, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Close()

	var archiveMailbox string
	switch account.ServiceProvider {
	case "gmail":
		// For Gmail, find the mailbox with the \All attribute
		archiveMailbox, err = getMailboxByAttr(c, imap.MailboxAttrAll)
		if err != nil {
			// Fallback to hardcoded path if attribute lookup fails
			archiveMailbox = "[Gmail]/All Mail"
		}
	default:
		archiveMailbox = "Archive"
	}

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return err
	}

	uidSet := imap.UIDSetNum(imap.UID(uid))
	_, err = c.Move(uidSet, archiveMailbox).Wait()
	return err
}

// Batch operations for multiple emails

// DeleteEmailsFromMailbox deletes multiple emails from a mailbox (batch operation)
func DeleteEmailsFromMailbox(account *config.Account, mailbox string, uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}

	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Close()

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return err
	}

	uidSet := uidsToUIDSet(uids)
	if err := c.Store(uidSet, &imap.StoreFlags{
		Op:     imap.StoreFlagsAdd,
		Silent: true,
		Flags:  []imap.Flag{imap.FlagDeleted},
	}, nil).Close(); err != nil {
		return err
	}

	return c.Expunge().Close()
}

// ArchiveEmailsFromMailbox archives multiple emails from a mailbox (batch operation)
func ArchiveEmailsFromMailbox(account *config.Account, mailbox string, uids []uint32) error {
	if len(uids) == 0 {
		return nil
	}

	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Close()

	var archiveMailbox string
	switch account.ServiceProvider {
	case "gmail":
		archiveMailbox, err = getMailboxByAttr(c, imap.MailboxAttrAll)
		if err != nil {
			archiveMailbox = "[Gmail]/All Mail"
		}
	default:
		archiveMailbox = "Archive"
	}

	if _, err := c.Select(mailbox, nil).Wait(); err != nil {
		return err
	}

	uidSet := uidsToUIDSet(uids)
	_, err = c.Move(uidSet, archiveMailbox).Wait()
	return err
}

// MoveEmailsToFolder moves multiple emails to a different folder (batch operation)
func MoveEmailsToFolder(account *config.Account, uids []uint32, sourceFolder, destFolder string) error {
	if len(uids) == 0 {
		return nil
	}

	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Close()

	if _, err := c.Select(sourceFolder, nil).Wait(); err != nil {
		return err
	}

	uidSet := uidsToUIDSet(uids)
	_, err = c.Move(uidSet, destFolder).Wait()
	return err
}

// Convenience wrappers defaulting to INBOX for existing call sites.

func FetchEmails(account *config.Account, limit, offset uint32) ([]Email, error) {
	return FetchMailboxEmails(account, "INBOX", limit, offset)
}

func FetchSentEmails(account *config.Account, limit, offset uint32) ([]Email, error) {
	return FetchMailboxEmails(account, getSentMailbox(account), limit, offset)
}

func FetchEmailBody(account *config.Account, uid uint32) (string, []Attachment, error) {
	return FetchEmailBodyFromMailbox(account, "INBOX", uid)
}

func FetchSentEmailBody(account *config.Account, uid uint32) (string, []Attachment, error) {
	return FetchEmailBodyFromMailbox(account, getSentMailbox(account), uid)
}

func FetchAttachment(account *config.Account, uid uint32, partID string, encoding string) ([]byte, error) {
	return FetchAttachmentFromMailbox(account, "INBOX", uid, partID, encoding)
}

func FetchSentAttachment(account *config.Account, uid uint32, partID string, encoding string) ([]byte, error) {
	return FetchAttachmentFromMailbox(account, getSentMailbox(account), uid, partID, encoding)
}

func DeleteEmail(account *config.Account, uid uint32) error {
	return DeleteEmailFromMailbox(account, "INBOX", uid)
}

func DeleteSentEmail(account *config.Account, uid uint32) error {
	return DeleteEmailFromMailbox(account, getSentMailbox(account), uid)
}

func ArchiveEmail(account *config.Account, uid uint32) error {
	return ArchiveEmailFromMailbox(account, "INBOX", uid)
}

func ArchiveSentEmail(account *config.Account, uid uint32) error {
	return ArchiveEmailFromMailbox(account, getSentMailbox(account), uid)
}

// AppendToSentMailbox appends a raw RFC822 message to the Sent mailbox via IMAP APPEND.
func AppendToSentMailbox(account *config.Account, rawMsg []byte) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Close()

	sentMailbox := getSentMailbox(account)
	appendCmd := c.Append(sentMailbox, int64(len(rawMsg)), &imap.AppendOptions{
		Flags: []imap.Flag{imap.FlagSeen},
		Time:  time.Now(),
	})
	if _, err := appendCmd.Write(rawMsg); err != nil {
		return err
	}
	if err := appendCmd.Close(); err != nil {
		return err
	}
	_, err = appendCmd.Wait()
	return err
}

// getTrashMailbox returns the trash mailbox name for the account
func getTrashMailbox(account *config.Account) string {
	switch account.ServiceProvider {
	case "gmail":
		return "[Gmail]/Trash"
	case "outlook":
		return "Deleted Items"
	case "icloud":
		return "Deleted Messages"
	default:
		return "Trash"
	}
}

// getArchiveMailbox returns the archive/all mail mailbox name for the account
func getArchiveMailbox(account *config.Account) string {
	switch account.ServiceProvider {
	case "gmail":
		return "[Gmail]/All Mail"
	case "outlook", "icloud":
		return "Archive"
	default:
		return "Archive"
	}
}

// FetchTrashEmails fetches emails from the trash folder
func FetchTrashEmails(account *config.Account, limit, offset uint32) ([]Email, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	// Try to find trash by attribute first
	trashMailbox, err := getMailboxByAttr(c, imap.MailboxAttrTrash)
	if err != nil {
		// Fallback to hardcoded path
		trashMailbox = getTrashMailbox(account)
	}

	return FetchMailboxEmails(account, trashMailbox, limit, offset)
}

// FetchArchiveEmails fetches emails from the archive/all mail folder
// Archive contains all emails, so we match where user is sender OR recipient
func FetchArchiveEmails(account *config.Account, limit, offset uint32) ([]Email, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	// Try to find archive by attribute first (Gmail uses \All)
	archiveMailbox, err := getMailboxByAttr(c, imap.MailboxAttrAll)
	if err != nil {
		// Fallback to hardcoded path
		archiveMailbox = getArchiveMailbox(account)
	}

	selectData, err := c.Select(archiveMailbox, nil).Wait()
	if err != nil {
		return nil, err
	}

	if selectData.NumMessages == 0 {
		return []Email{}, nil
	}

	to := selectData.NumMessages - offset
	from := uint32(1)
	if to > limit {
		from = to - limit + 1
	}

	if to < 1 {
		return []Email{}, nil
	}

	var seqset imap.SeqSet
	seqset.AddRange(from, to)

	// Delivery header section for matching auto-forwarded emails
	deliveryHeaderSection := &imap.FetchItemBodySection{
		Specifier:    imap.PartSpecifierHeader,
		HeaderFields: []string{"Delivered-To", "X-Forwarded-To", "X-Original-To"},
		Peek:         true,
	}

	fetchCmd := c.Fetch(seqset, &imap.FetchOptions{
		Envelope:    true,
		UID:         true,
		Flags:       true,
		BodySection: []*imap.FetchItemBodySection{deliveryHeaderSection},
	})
	msgs, err := fetchCmd.Collect()
	if err != nil {
		return nil, err
	}

	// Determine which email to filter on: prefer Account.FetchEmail, fallback to Account.Email
	fetchEmail := strings.ToLower(strings.TrimSpace(account.FetchEmail))
	if fetchEmail == "" {
		fetchEmail = strings.ToLower(strings.TrimSpace(account.Email))
	}

	var emails []Email
	for _, msg := range msgs {
		if msg.Envelope == nil {
			continue
		}

		var fromAddr string
		if len(msg.Envelope.From) > 0 {
			fromAddr = formatAddress(msg.Envelope.From[0])
		}

		var toAddrList []string
		for _, addr := range msg.Envelope.To {
			toAddrList = append(toAddrList, addr.Addr())
		}
		for _, addr := range msg.Envelope.Cc {
			toAddrList = append(toAddrList, addr.Addr())
		}

		// For archive/All Mail, match emails where user is sender OR recipient
		matched := false
		if account.CatchAll {
			matched = true
		} else {
			// Check if user is the sender
			if addressMatches(fromAddr, fetchEmail, account) {
				matched = true
			}
			// Check if user is a recipient
			if !matched {
				for _, r := range toAddrList {
					if addressMatches(r, fetchEmail, account) {
						matched = true
						break
					}
				}
			}
			// Check delivery headers for auto-forwarded emails
			if !matched {
				headerData := msg.FindBodySection(deliveryHeaderSection)
				matched = deliveryHeadersMatch(headerData, fetchEmail, account)
			}
		}

		if !matched {
			continue
		}

		emails = append(emails, Email{
			UID:       uint32(msg.UID),
			From:      fromAddr,
			To:        toAddrList,
			Subject:   decodeHeader(msg.Envelope.Subject),
			Date:      msg.Envelope.Date,
			IsRead:    hasSeenFlag(msg.Flags),
			AccountID: account.ID,
		})
	}

	// Reverse to get newest first
	for i, j := 0, len(emails)-1; i < j; i, j = i+1, j-1 {
		emails[i], emails[j] = emails[j], emails[i]
	}

	return emails, nil
}

// FetchTrashEmailBody fetches the body of an email from trash
func FetchTrashEmailBody(account *config.Account, uid uint32) (string, []Attachment, error) {
	c, err := connect(account)
	if err != nil {
		return "", nil, err
	}
	defer c.Close()

	trashMailbox, err := getMailboxByAttr(c, imap.MailboxAttrTrash)
	if err != nil {
		trashMailbox = getTrashMailbox(account)
	}

	return FetchEmailBodyFromMailbox(account, trashMailbox, uid)
}

// FetchArchiveEmailBody fetches the body of an email from archive
func FetchArchiveEmailBody(account *config.Account, uid uint32) (string, []Attachment, error) {
	c, err := connect(account)
	if err != nil {
		return "", nil, err
	}
	defer c.Close()

	archiveMailbox, err := getMailboxByAttr(c, imap.MailboxAttrAll)
	if err != nil {
		archiveMailbox = getArchiveMailbox(account)
	}

	return FetchEmailBodyFromMailbox(account, archiveMailbox, uid)
}

// FetchTrashAttachment fetches an attachment from trash
func FetchTrashAttachment(account *config.Account, uid uint32, partID string, encoding string) ([]byte, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	trashMailbox, err := getMailboxByAttr(c, imap.MailboxAttrTrash)
	if err != nil {
		trashMailbox = getTrashMailbox(account)
	}

	return FetchAttachmentFromMailbox(account, trashMailbox, uid, partID, encoding)
}

// FetchArchiveAttachment fetches an attachment from archive
func FetchArchiveAttachment(account *config.Account, uid uint32, partID string, encoding string) ([]byte, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	archiveMailbox, err := getMailboxByAttr(c, imap.MailboxAttrAll)
	if err != nil {
		archiveMailbox = getArchiveMailbox(account)
	}

	return FetchAttachmentFromMailbox(account, archiveMailbox, uid, partID, encoding)
}

// DeleteTrashEmail permanently deletes an email from trash
func DeleteTrashEmail(account *config.Account, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Close()

	trashMailbox, err := getMailboxByAttr(c, imap.MailboxAttrTrash)
	if err != nil {
		trashMailbox = getTrashMailbox(account)
	}

	return DeleteEmailFromMailbox(account, trashMailbox, uid)
}

// DeleteArchiveEmail deletes an email from archive (moves to trash)
func DeleteArchiveEmail(account *config.Account, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Close()

	archiveMailbox, err := getMailboxByAttr(c, imap.MailboxAttrAll)
	if err != nil {
		archiveMailbox = getArchiveMailbox(account)
	}

	return DeleteEmailFromMailbox(account, archiveMailbox, uid)
}

// FetchFolders lists all IMAP folders/mailboxes for an account.
func FetchFolders(account *config.Account) ([]Folder, error) {
	c, err := connect(account)
	if err != nil {
		return nil, err
	}
	defer c.Close()

	listCmd := c.List("", "*", nil)
	defer listCmd.Close()

	var folders []Folder
	for {
		data := listCmd.Next()
		if data == nil {
			break
		}
		delim := ""
		if data.Delim != 0 {
			delim = string(data.Delim)
		}
		var attrs []string
		for _, a := range data.Attrs {
			attrs = append(attrs, string(a))
		}
		folders = append(folders, Folder{
			Name:       data.Mailbox,
			Delimiter:  delim,
			Attributes: attrs,
		})
	}

	if err := listCmd.Close(); err != nil {
		return nil, err
	}

	return folders, nil
}

// MoveEmailToFolder moves an email from one folder to another via IMAP.
func MoveEmailToFolder(account *config.Account, uid uint32, sourceFolder, destFolder string) error {
	return moveEmail(account, uid, sourceFolder, destFolder)
}

// FetchFolderEmails fetches emails from an arbitrary folder.
func FetchFolderEmails(account *config.Account, folder string, limit, offset uint32) ([]Email, error) {
	return FetchMailboxEmails(account, folder, limit, offset)
}

// FetchFolderEmailBody fetches the body of an email from an arbitrary folder.
func FetchFolderEmailBody(account *config.Account, folder string, uid uint32) (string, []Attachment, error) {
	return FetchEmailBodyFromMailbox(account, folder, uid)
}

// FetchFolderAttachment fetches an attachment from an arbitrary folder.
func FetchFolderAttachment(account *config.Account, folder string, uid uint32, partID string, encoding string) ([]byte, error) {
	return FetchAttachmentFromMailbox(account, folder, uid, partID, encoding)
}

// DeleteFolderEmail deletes an email from an arbitrary folder.
func DeleteFolderEmail(account *config.Account, folder string, uid uint32) error {
	return DeleteEmailFromMailbox(account, folder, uid)
}

// ArchiveFolderEmail archives an email from an arbitrary folder.
func ArchiveFolderEmail(account *config.Account, folder string, uid uint32) error {
	return ArchiveEmailFromMailbox(account, folder, uid)
}

// decryptPGPMessage decrypts a PGP-encrypted message using the account's private key.
func decryptPGPMessage(encryptedData []byte, account *config.Account) ([]byte, error) {
	if account.PGPPrivateKey == "" {
		return nil, errors.New("PGP private key not configured")
	}

	// Load private key
	keyFile, err := os.ReadFile(account.PGPPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read PGP private key: %w", err)
	}

	// Try armored format first
	entityList, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(keyFile))
	if err != nil {
		// Try binary format
		entityList, err = openpgp.ReadKeyRing(bytes.NewReader(keyFile))
		if err != nil {
			return nil, fmt.Errorf("failed to parse PGP private key: %w", err)
		}
	}

	if len(entityList) == 0 {
		return nil, errors.New("no PGP keys found in private keyring")
	}

	// Decrypt using go-pgpmail
	mr, err := pgpmail.Read(bytes.NewReader(encryptedData), openpgp.EntityList{entityList[0]}, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt PGP message: %w", err)
	}

	// Read decrypted content from UnverifiedBody
	if mr.MessageDetails == nil || mr.MessageDetails.UnverifiedBody == nil {
		return nil, errors.New("no decrypted content available")
	}

	var decrypted bytes.Buffer
	if _, err := io.Copy(&decrypted, mr.MessageDetails.UnverifiedBody); err != nil {
		return nil, fmt.Errorf("failed to read decrypted content: %w", err)
	}

	return decrypted.Bytes(), nil
}

// loadPGPKeyring builds an openpgp.EntityList from the account's public key
// and any keys stored in the pgp/ config directory.
func loadPGPKeyring(account *config.Account) openpgp.EntityList {
	var keyring openpgp.EntityList

	readKeys := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		entities, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(data))
		if err != nil {
			entities, err = openpgp.ReadKeyRing(bytes.NewReader(data))
			if err != nil {
				return
			}
		}
		keyring = append(keyring, entities...)
	}

	// Load account's own public key
	if account.PGPPublicKey != "" {
		readKeys(account.PGPPublicKey)
	}

	// Load all keys from the pgp/ config directory
	cfgDir, err := config.GetConfigDir()
	if err == nil {
		pgpDir := cfgDir + "/pgp"
		entries, err := os.ReadDir(pgpDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				name := entry.Name()
				if strings.HasSuffix(name, ".asc") || strings.HasSuffix(name, ".gpg") {
					readKeys(pgpDir + "/" + name)
				}
			}
		}
	}

	return keyring
}

// verifyPGPSignature verifies a PGP detached signature against signed content.
func verifyPGPSignature(signedContent, signatureData []byte, account *config.Account) bool {
	keyring := loadPGPKeyring(account)
	if len(keyring) == 0 {
		return false
	}

	// Build a complete multipart/signed message for go-pgpmail
	boundary := "pgp-verify-boundary"
	var msg bytes.Buffer
	msg.WriteString("Content-Type: multipart/signed; boundary=\"" + boundary + "\"; micalg=pgp-sha256; protocol=\"application/pgp-signature\"\r\n\r\n")
	msg.WriteString("--" + boundary + "\r\n")
	msg.Write(signedContent)
	msg.WriteString("\r\n--" + boundary + "\r\n")
	msg.WriteString("Content-Type: application/pgp-signature\r\n\r\n")
	msg.Write(signatureData)
	msg.WriteString("\r\n--" + boundary + "--\r\n")

	mr, err := pgpmail.Read(&msg, keyring, nil, nil)
	if err != nil {
		return false
	}

	if mr.MessageDetails == nil {
		return false
	}

	// Must read UnverifiedBody to EOF to trigger signature verification
	_, _ = io.ReadAll(mr.MessageDetails.UnverifiedBody)

	return mr.MessageDetails.SignatureError == nil
}
