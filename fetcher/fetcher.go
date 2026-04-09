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
	"mime"
	"mime/quotedprintable"
	"net/textproto"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	"github.com/emersion/go-message/mail"
	"github.com/emersion/go-pgpmail"
	"github.com/floatpane/matcha/config"
	"go.mozilla.org/pkcs7"
	"golang.org/x/text/encoding/ianaindex"
	"golang.org/x/text/transform"
)

// Attachment holds data for an email attachment.
type Attachment struct {
	Filename         string
	PartID           string // Keep PartID to fetch on demand
	Data             []byte
	Encoding         string // Store encoding for proper decoding
	MIMEType         string // Full MIME type (e.g., image/png)
	ContentID        string // Content-ID for inline assets (e.g., cid: references)
	Inline           bool   // True when the part is meant to be displayed inline
	IsSMIMESignature bool   // True if this attachment is an S/MIME signature
	SMIMEVerified    bool   // True if the S/MIME signature was verified successfully
	IsSMIMEEncrypted bool   // True if the S/MIME content was successfully decrypted
	IsPGPSignature   bool   // True if this attachment is a PGP signature
	PGPVerified      bool   // True if the PGP signature was verified successfully
	IsPGPEncrypted   bool   // True if the PGP content was successfully decrypted
}

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
	AccountID   string // ID of the account this email belongs to
}

// Folder represents an IMAP mailbox/folder.
type Folder struct {
	Name       string
	Delimiter  string
	Attributes []string
}

// formatAddress returns "Name <email>" when a PersonalName is present,
// otherwise just "email".
func formatAddress(addr *imap.Address) string {
	email := addr.Address()
	if addr.PersonalName != "" {
		return addr.PersonalName + " <" + email + ">"
	}
	return email
}

func hasSeenFlag(flags []string) bool {
	return slices.Contains(flags, imap.SeenFlag)
}

// deliveryHeadersMatch checks if any of the Delivered-To, X-Forwarded-To, or
// X-Original-To headers contain the given email address. This catches
// auto-forwarded emails where the envelope To/Cc don't match the local account.
func deliveryHeadersMatch(msg *imap.Message, section *imap.BodySectionName, fetchEmail string) bool {
	if section == nil {
		return false
	}
	literal := msg.GetBody(section)
	if literal == nil {
		return false
	}
	data, err := ioutil.ReadAll(literal)
	if err != nil || len(data) == 0 {
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
			if strings.EqualFold(strings.TrimSpace(val), fetchEmail) {
				return true
			}
		}
	}
	return false
}

func decodePart(reader io.Reader, header mail.PartHeader) (string, error) {
	mediaType, params, err := mime.ParseMediaType(header.Get("Content-Type"))
	if err != nil {
		body, _ := ioutil.ReadAll(reader)
		return string(body), nil
	}

	charset := "utf-8"
	if params["charset"] != "" {
		charset = strings.ToLower(params["charset"])
	}

	encoding, err := ianaindex.IANA.Encoding(charset)
	if err != nil || encoding == nil {
		encoding, _ = ianaindex.IANA.Encoding("utf-8")
	}

	transformReader := transform.NewReader(reader, encoding.NewDecoder())
	decodedBody, err := ioutil.ReadAll(transformReader)
	if err != nil {
		return "", err
	}

	if strings.HasPrefix(mediaType, "multipart/") {
		return "[This is a multipart message]", nil
	}

	return string(decodedBody), nil
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
		return ioutil.ReadAll(decoder)
	case "quoted-printable":
		return ioutil.ReadAll(quotedprintable.NewReader(bytes.NewReader(rawBytes)))
	default:
		return rawBytes, nil
	}
}

func connect(account *config.Account) (*client.Client, error) {
	imapServer := account.GetIMAPServer()
	imapPort := account.GetIMAPPort()

	if imapServer == "" {
		return nil, fmt.Errorf("unsupported service_provider: %s", account.ServiceProvider)
	}

	addr := fmt.Sprintf("%s:%d", imapServer, imapPort)

	tlsConfig := &tls.Config{
		ServerName:         imapServer,
		InsecureSkipVerify: account.Insecure,
	}

	var c *client.Client
	var err error

	// If using standard non-implicit ports (1143 or 143), use Dial + STARTTLS
	if imapPort == 1143 || imapPort == 143 {
		c, err = client.Dial(addr)
		if err != nil {
			return nil, err
		}
		if err := c.StartTLS(tlsConfig); err != nil {
			return nil, err
		}
	} else {
		// Otherwise default to implicit TLS (port 993)
		c, err = client.DialTLS(addr, tlsConfig)
		if err != nil {
			return nil, err
		}
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
		if err := c.Login(account.Email, account.Password); err != nil {
			return nil, err
		}
	}

	return c, nil
}

func getSentMailbox(account *config.Account) string {
	switch account.ServiceProvider {
	case "gmail":
		return "[Gmail]/Sent Mail"
	case "icloud":
		return "Sent Messages"
	default:
		return "Sent"
	}
}

// getMailboxByAttr finds a mailbox with the given IMAP attribute (e.g., \All, \Sent, \Trash).
func getMailboxByAttr(c *client.Client, attr string) (string, error) {
	mailboxes := make(chan *imap.MailboxInfo, 10)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	var foundMailbox string
	for m := range mailboxes {
		for _, a := range m.Attributes {
			if a == attr {
				foundMailbox = m.Name
				break
			}
		}
	}

	if err := <-done; err != nil {
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
	defer c.Logout()

	mbox, err := c.Select(mailbox, false)
	if err != nil {
		return nil, err
	}

	if mbox.Messages == 0 {
		return []Email{}, nil
	}

	var allEmails []Email

	// Start from the top minus offset
	cursor := uint32(0)
	if mbox.Messages > offset {
		cursor = mbox.Messages - offset
	} else {
		return []Email{}, nil
	}

	// Determine if we should filter
	fetchEmail := strings.ToLower(strings.TrimSpace(account.FetchEmail))
	if fetchEmail == "" {
		fetchEmail = strings.ToLower(strings.TrimSpace(account.Email))
	}
	isSentMailbox := mailbox == getSentMailbox(account)

	// Loop until we have enough emails or run out of messages
	for len(allEmails) < int(limit) && cursor > 0 {
		// Determine chunk size
		// Fetch at least 'limit' or 50 messages to reduce round trips
		chunkSize := limit
		if chunkSize < 50 {
			chunkSize = 50
		}

		from := uint32(1)
		if cursor > uint32(chunkSize) {
			from = cursor - uint32(chunkSize) + 1
		}

		seqset := new(imap.SeqSet)
		seqset.AddRange(from, cursor)

		messages := make(chan *imap.Message, chunkSize)
		done := make(chan error, 1)
		// Fetch delivery headers to match auto-forwarded emails (Delivered-To, X-Forwarded-To, X-Original-To)
		deliveryHeaderItem := imap.FetchItem("BODY.PEEK[HEADER.FIELDS (Delivered-To X-Forwarded-To X-Original-To)]")
		deliveryHeaderSection, _ := imap.ParseBodySectionName(deliveryHeaderItem)
		fetchItems := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid, imap.FetchFlags, deliveryHeaderItem}

		go func() {
			done <- c.Fetch(seqset, fetchItems, messages)
		}()

		var batchMsgs []*imap.Message
		for msg := range messages {
			batchMsgs = append(batchMsgs, msg)
		}

		if err := <-done; err != nil {
			return nil, err
		}

		// Filter messages in this batch
		var batchEmails []Email
		for _, msg := range batchMsgs {
			if msg == nil || msg.Envelope == nil {
				continue
			}

			var fromAddr string
			if len(msg.Envelope.From) > 0 {
				fromAddr = formatAddress(msg.Envelope.From[0])
			}

			var toAddrList []string
			for _, addr := range msg.Envelope.To {
				toAddrList = append(toAddrList, addr.Address())
			}
			for _, addr := range msg.Envelope.Cc {
				toAddrList = append(toAddrList, addr.Address())
			}

			matched := false
			if isSentMailbox {
				var senderEmail string
				if len(msg.Envelope.From) > 0 {
					senderEmail = msg.Envelope.From[0].Address()
				}
				if strings.EqualFold(strings.TrimSpace(senderEmail), fetchEmail) {
					matched = true
				}
			} else {
				for _, r := range toAddrList {
					if strings.EqualFold(strings.TrimSpace(r), fetchEmail) {
						matched = true
						break
					}
				}
				// Check delivery headers for auto-forwarded emails
				if !matched {
					matched = deliveryHeadersMatch(msg, deliveryHeaderSection, fetchEmail)
				}
			}

			if !matched {
				continue
			}

			batchEmails = append(batchEmails, Email{
				UID:       msg.Uid,
				From:      fromAddr,
				To:        toAddrList,
				Subject:   decodeHeader(msg.Envelope.Subject),
				Date:      msg.Envelope.Date,
				IsRead:    hasSeenFlag(msg.Flags),
				AccountID: account.ID,
			})
		}

		// Sort batch Newest -> Oldest (since IMAP usually returns Oldest->Newest or arbitrary)
		// Assuming seqset order or standard behavior, we want to ensure we append Newest emails first
		// so that the final list is correct.
		// Actually, let's just sort the batch by UID desc (Newest first)
		// Simple bubble sort for small batch
		for i := 0; i < len(batchEmails); i++ {
			for j := i + 1; j < len(batchEmails); j++ {
				if batchEmails[j].UID > batchEmails[i].UID {
					batchEmails[i], batchEmails[j] = batchEmails[j], batchEmails[i]
				}
			}
		}

		// Append to allEmails
		allEmails = append(allEmails, batchEmails...)

		// Update cursor for next iteration
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
	defer c.Logout()

	if _, err := c.Select(mailbox, false); err != nil {
		return "", nil, err
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	fetchWholeMessage := func() ([]byte, error) {
		fetchItem := imap.FetchItem("BODY.PEEK[]")
		section, _ := imap.ParseBodySectionName(fetchItem)
		partMessages := make(chan *imap.Message, 1)
		partDone := make(chan error, 1)
		go func() {
			partDone <- c.UidFetch(seqset, []imap.FetchItem{fetchItem}, partMessages)
		}()
		if err := <-partDone; err != nil {
			return nil, err
		}
		partMsg := <-partMessages
		if partMsg != nil {
			literal := partMsg.GetBody(section)
			if literal != nil {
				return ioutil.ReadAll(literal)
			}
		}
		return nil, fmt.Errorf("could not fetch whole message")
	}

	fetchInlinePart := func(partID, encoding string) ([]byte, error) {
		fetchItem := imap.FetchItem(fmt.Sprintf("BODY.PEEK[%s]", partID))
		section, err := imap.ParseBodySectionName(fetchItem)
		if err != nil {
			return nil, err
		}

		partMessages := make(chan *imap.Message, 1)
		partDone := make(chan error, 1)
		go func() {
			partDone <- c.UidFetch(seqset, []imap.FetchItem{fetchItem}, partMessages)
		}()

		if err := <-partDone; err != nil {
			return nil, err
		}

		partMsg := <-partMessages
		if partMsg == nil {
			return nil, fmt.Errorf("could not fetch inline part %s", partID)
		}

		literal := partMsg.GetBody(section)
		if literal == nil {
			return nil, fmt.Errorf("could not get inline part body %s", partID)
		}

		rawBytes, err := ioutil.ReadAll(literal)
		if err != nil {
			return nil, err
		}

		return decodeAttachmentData(rawBytes, encoding)
	}

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	fetchItems := []imap.FetchItem{imap.FetchBodyStructure}
	go func() {
		done <- c.UidFetch(seqset, fetchItems, messages)
	}()

	if err := <-done; err != nil {
		return "", nil, err
	}

	msg := <-messages
	if msg == nil || msg.BodyStructure == nil {
		return "", nil, fmt.Errorf("no message or body structure found with UID %d", uid)
	}

	var plainPartID, plainPartEncoding string
	var htmlPartID, htmlPartEncoding string
	var attachments []Attachment
	var extractedBody string // Used if we intercept and decrypt a payload

	var checkPart func(part *imap.BodyStructure, partID string)
	checkPart = func(part *imap.BodyStructure, partID string) {
		// Check for text content (prefer html over plain)
		if part.MIMEType == "text" {
			sub := strings.ToLower(part.MIMESubType)
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
		filename := ""
		// First try the Filename() method which handles various cases
		if fn, err := part.Filename(); err == nil && fn != "" {
			filename = fn
		}
		// Fallback: check DispositionParams
		if filename == "" {
			if fn, ok := part.DispositionParams["filename"]; ok && fn != "" {
				filename = fn
			}
		}
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
		contentID := strings.Trim(part.Id, "<>")
		mimeType := fmt.Sprintf("%s/%s", strings.ToLower(part.MIMEType), strings.ToLower(part.MIMESubType))
		isCID := contentID != ""
		isInline := part.Disposition == "inline" || isCID

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
								b, _ := ioutil.ReadAll(p.Body) // Auto-decodes quoted-printable/base64

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
					boundary := msg.BodyStructure.Params["boundary"]
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
		if mimeType == "application/pgp-encrypted" || (mimeType == "multipart/encrypted" && strings.Contains(part.MIMESubType, "pgp")) {
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
				boundary := msg.BodyStructure.Params["boundary"]
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
		} else if (filename != "" || isCID) && (part.Disposition == "attachment" || isInline || part.MIMEType != "text") {
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

	var findParts func(*imap.BodyStructure, string)
	findParts = func(bs *imap.BodyStructure, prefix string) {
		// If this is a non-multipart message, check the body structure itself
		if len(bs.Parts) == 0 {
			partID := prefix
			if partID == "" {
				partID = "1"
			}
			checkPart(bs, partID)
			return
		}

		// Iterate through parts
		for i, part := range bs.Parts {
			partID := fmt.Sprintf("%d", i+1)
			if prefix != "" {
				partID = fmt.Sprintf("%s.%d", prefix, i+1)
			}

			checkPart(part, partID)

			if len(part.Parts) > 0 {
				findParts(part, partID)
			}
		}
	}
	findParts(msg.BodyStructure, "")

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
		fmt.Print(msg)
		if path := os.Getenv("DEBUG_KITTY_LOG"); path != "" {
			if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
				_, _ = f.WriteString(msg)
				_ = f.Close()
			}
		}
	}
	if textPartID != "" {
		partMessages := make(chan *imap.Message, 1)
		partDone := make(chan error, 1)

		fetchItem := imap.FetchItem(fmt.Sprintf("BODY.PEEK[%s]", textPartID))
		section, err := imap.ParseBodySectionName(fetchItem)
		if err != nil {
			return "", nil, err
		}

		go func() {
			partDone <- c.UidFetch(seqset, []imap.FetchItem{fetchItem}, partMessages)
		}()

		if err := <-partDone; err != nil {
			return "", nil, err
		}

		partMsg := <-partMessages
		if partMsg != nil {
			literal := partMsg.GetBody(section)
			if literal != nil {
				buf, _ := ioutil.ReadAll(literal)
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
	defer c.Logout()

	if _, err := c.Select(mailbox, false); err != nil {
		return nil, err
	}

	seqset := new(imap.SeqSet)
	seqset.AddNum(uid)

	fetchItem := imap.FetchItem(fmt.Sprintf("BODY.PEEK[%s]", partID))
	section, err := imap.ParseBodySectionName(fetchItem)
	if err != nil {
		return nil, err
	}

	messages := make(chan *imap.Message, 1)
	done := make(chan error, 1)
	go func() {
		done <- c.UidFetch(seqset, []imap.FetchItem{fetchItem}, messages)
	}()

	if err := <-done; err != nil {
		return nil, err
	}

	msg := <-messages
	if msg == nil {
		return nil, fmt.Errorf("could not fetch attachment")
	}

	literal := msg.GetBody(section)
	if literal == nil {
		return nil, fmt.Errorf("could not get attachment body")
	}

	rawBytes, err := ioutil.ReadAll(literal)
	if err != nil {
		return nil, err
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
	defer c.Logout()

	if _, err := c.Select(sourceMailbox, false); err != nil {
		return err
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	return c.UidMove(seqSet, destMailbox)
}

func MarkEmailAsReadInMailbox(account *config.Account, mailbox string, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Logout()

	if _, err := c.Select(mailbox, false); err != nil {
		return err
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.SeenFlag}

	return c.UidStore(seqSet, item, flags, nil)
}

func DeleteEmailFromMailbox(account *config.Account, mailbox string, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Logout()

	if _, err := c.Select(mailbox, false); err != nil {
		return err
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	item := imap.FormatFlagsOp(imap.AddFlags, true)
	flags := []interface{}{imap.DeletedFlag}

	if err := c.UidStore(seqSet, item, flags, nil); err != nil {
		return err
	}

	return c.Expunge(nil)
}

func ArchiveEmailFromMailbox(account *config.Account, mailbox string, uid uint32) error {
	c, err := connect(account)
	if err != nil {
		return err
	}
	defer c.Logout()

	var archiveMailbox string
	switch account.ServiceProvider {
	case "gmail":
		// For Gmail, find the mailbox with the \All attribute
		archiveMailbox, err = getMailboxByAttr(c, imap.AllAttr)
		if err != nil {
			// Fallback to hardcoded path if attribute lookup fails
			archiveMailbox = "[Gmail]/All Mail"
		}
	default:
		archiveMailbox = "Archive"
	}

	if _, err := c.Select(mailbox, false); err != nil {
		return err
	}

	seqSet := new(imap.SeqSet)
	seqSet.AddNum(uid)

	return c.UidMove(seqSet, archiveMailbox)
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

// getTrashMailbox returns the trash mailbox name for the account
func getTrashMailbox(account *config.Account) string {
	switch account.ServiceProvider {
	case "gmail":
		return "[Gmail]/Trash"
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
	case "icloud":
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
	defer c.Logout()

	// Try to find trash by attribute first
	trashMailbox, err := getMailboxByAttr(c, imap.TrashAttr)
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
	defer c.Logout()

	// Try to find archive by attribute first (Gmail uses \All)
	archiveMailbox, err := getMailboxByAttr(c, imap.AllAttr)
	if err != nil {
		// Fallback to hardcoded path
		archiveMailbox = getArchiveMailbox(account)
	}

	mbox, err := c.Select(archiveMailbox, false)
	if err != nil {
		return nil, err
	}

	if mbox.Messages == 0 {
		return []Email{}, nil
	}

	to := mbox.Messages - offset
	from := uint32(1)
	if to > limit {
		from = to - limit + 1
	}

	if to < 1 {
		return []Email{}, nil
	}

	seqset := new(imap.SeqSet)
	seqset.AddRange(from, to)

	messages := make(chan *imap.Message, limit)
	done := make(chan error, 1)
	// Fetch delivery headers to match auto-forwarded emails
	deliveryHeaderItem := imap.FetchItem("BODY.PEEK[HEADER.FIELDS (Delivered-To X-Forwarded-To X-Original-To)]")
	deliveryHeaderSection, _ := imap.ParseBodySectionName(deliveryHeaderItem)
	fetchItems := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid, imap.FetchFlags, deliveryHeaderItem}
	go func() {
		done <- c.Fetch(seqset, fetchItems, messages)
	}()

	var msgs []*imap.Message
	for msg := range messages {
		msgs = append(msgs, msg)
	}

	if err := <-done; err != nil {
		return nil, err
	}

	// Determine which email to filter on: prefer Account.FetchEmail, fallback to Account.Email
	fetchEmail := strings.ToLower(strings.TrimSpace(account.FetchEmail))
	if fetchEmail == "" {
		fetchEmail = strings.ToLower(strings.TrimSpace(account.Email))
	}

	var emails []Email
	for _, msg := range msgs {
		if msg == nil || msg.Envelope == nil {
			continue
		}

		var fromAddr string
		if len(msg.Envelope.From) > 0 {
			fromAddr = formatAddress(msg.Envelope.From[0])
		}

		var toAddrList []string
		for _, addr := range msg.Envelope.To {
			toAddrList = append(toAddrList, addr.Address())
		}
		for _, addr := range msg.Envelope.Cc {
			toAddrList = append(toAddrList, addr.Address())
		}

		// For archive/All Mail, match emails where user is sender OR recipient
		matched := false
		// Check if user is the sender
		if strings.EqualFold(strings.TrimSpace(fromAddr), fetchEmail) {
			matched = true
		}
		// Check if user is a recipient
		if !matched {
			for _, r := range toAddrList {
				if strings.EqualFold(strings.TrimSpace(r), fetchEmail) {
					matched = true
					break
				}
			}
		}
		// Check delivery headers for auto-forwarded emails
		if !matched {
			matched = deliveryHeadersMatch(msg, deliveryHeaderSection, fetchEmail)
		}

		if !matched {
			continue
		}

		emails = append(emails, Email{
			UID:       msg.Uid,
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
	defer c.Logout()

	trashMailbox, err := getMailboxByAttr(c, imap.TrashAttr)
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
	defer c.Logout()

	archiveMailbox, err := getMailboxByAttr(c, imap.AllAttr)
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
	defer c.Logout()

	trashMailbox, err := getMailboxByAttr(c, imap.TrashAttr)
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
	defer c.Logout()

	archiveMailbox, err := getMailboxByAttr(c, imap.AllAttr)
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
	defer c.Logout()

	trashMailbox, err := getMailboxByAttr(c, imap.TrashAttr)
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
	defer c.Logout()

	archiveMailbox, err := getMailboxByAttr(c, imap.AllAttr)
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
	defer c.Logout()

	mailboxes := make(chan *imap.MailboxInfo, 50)
	done := make(chan error, 1)
	go func() {
		done <- c.List("", "*", mailboxes)
	}()

	var folders []Folder
	for m := range mailboxes {
		folders = append(folders, Folder{
			Name:       m.Name,
			Delimiter:  m.Delimiter,
			Attributes: m.Attributes,
		})
	}

	if err := <-done; err != nil {
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
