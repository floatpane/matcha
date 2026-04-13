package sender

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/smtp"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	messagetextproto "github.com/emersion/go-message/textproto"
	"github.com/emersion/go-pgpmail"
	"github.com/floatpane/matcha/clib"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/pgp"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"go.mozilla.org/pkcs7"
)

// xoauth2Auth implements the SMTP XOAUTH2 authentication mechanism for OAuth2.
// See https://developers.google.com/gmail/imap/xoauth2-protocol
type xoauth2Auth struct {
	username, token string
}

func (a *xoauth2Auth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	resp := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", a.username, a.token)
	return "XOAUTH2", []byte(resp), nil
}

func (a *xoauth2Auth) Next(fromServer []byte, more bool) ([]byte, error) {
	if more {
		// Server sent an error challenge; respond with empty to finish.
		return []byte{}, nil
	}
	return nil, nil
}

// loginAuth implements the SMTP LOGIN authentication mechanism.
// Some SMTP servers (e.g. Mailo) only support LOGIN and not PLAIN.
type loginAuth struct {
	username, password string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.TrimSpace(string(fromServer))
	switch strings.ToLower(prompt) {
	case "username:":
		return []byte(a.username), nil
	case "password:":
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected LOGIN prompt: %s", prompt)
	}
}

// generateMessageID creates a unique Message-ID header.
func generateMessageID(from string) string {
	buf := make([]byte, 16)
	_, err := rand.Read(buf)
	if err != nil {
		return fmt.Sprintf("<%d.%s>", time.Now().UnixNano(), from)
	}
	return fmt.Sprintf("<%x@%s>", buf, from)
}

// containsMarkup returns true if the string contains Markdown or HTML elements.
func containsMarkup(body string) bool {
	// Parse the Markdown into an AST. We will consider most AST node kinds as
	// markup, but treat bare/autolinks (raw URLs) as plaintext for this
	// detection: if a link node's visible text equals its destination (or is
	// the destination wrapped in <>), we allow it.
	source := []byte(body)
	md := goldmark.New()
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	var hasMarkup bool
	ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node.Kind() {
		case ast.KindDocument, ast.KindParagraph, ast.KindText:
			// not considered formatting
			return ast.WalkContinue, nil
		case ast.KindLink:
			// Check if this is an autolink/raw URL: the link's text equals the
			// destination. If so, don't treat it as markup for our purposes.
			linkNode, ok := node.(*ast.Link)
			if !ok {
				hasMarkup = true
				return ast.WalkStop, nil
			}

			// Collect the visible text of the link
			var b strings.Builder
			for c := node.FirstChild(); c != nil; c = c.NextSibling() {
				if txt, ok := c.(*ast.Text); ok {
					b.Write(txt.Segment.Value(source))
				} else {
					// non-text content inside link -> treat as markup
					hasMarkup = true
					return ast.WalkStop, nil
				}
			}
			linkText := b.String()
			dest := string(linkNode.Destination)

			// Normalize common autolink representations and allow them.
			if linkText == dest || linkText == "<"+dest+">" {
				return ast.WalkContinue, nil
			}

			// Otherwise treat as markup
			hasMarkup = true
			return ast.WalkStop, nil
		default:
			hasMarkup = true
			return ast.WalkStop, nil
		}
	})
	return hasMarkup
}

// detectPlaintextOnly returns true when the body contains only plain text
// (no images, no attachments, no markdown/HTML formatting that requires multipart).
func detectPlaintextOnly(body string, images, attachments map[string][]byte) bool {
	if len(images) > 0 || len(attachments) > 0 {
		return false
	}
	return !containsMarkup(body)
}

// SendEmail constructs a multipart message with plain text, HTML, embedded images, and attachments.
func SendEmail(account *config.Account, to, cc, bcc []string, subject, plainBody, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string, signSMIME bool, encryptSMIME bool, signPGP bool, encryptPGP bool) error {
	smtpServer := account.GetSMTPServer()
	smtpPort := account.GetSMTPPort()

	if smtpServer == "" {
		return fmt.Errorf("unsupported or missing service_provider: %s", account.ServiceProvider)
	}

	plainAuth := smtp.PlainAuth("", account.Email, account.Password, smtpServer)
	loginAuthFallback := &loginAuth{username: account.Email, password: account.Password}

	fromHeader := account.FormatFromHeader()

	// Set top-level headers (From/To/Subject/Date/etc)
	headers := map[string]string{
		"From":         fromHeader,
		"To":           strings.Join(to, ", "),
		"Subject":      subject,
		"Date":         time.Now().Format(time.RFC1123Z),
		"Message-ID":   generateMessageID(account.GetSendAsEmail()),
		"MIME-Version": "1.0",
	}

	if len(cc) > 0 {
		headers["Cc"] = strings.Join(cc, ", ")
	}

	if inReplyTo != "" {
		headers["In-Reply-To"] = inReplyTo
		if len(references) > 0 {
			headers["References"] = strings.Join(references, " ") + " " + inReplyTo
		} else {
			headers["References"] = inReplyTo
		}
	}

	// prepare final message buffer and S/MIME payload placeholder
	var msg bytes.Buffer
	headerOrder := []string{"From", "To", "Cc", "Subject", "Date", "Message-ID", "MIME-Version", "In-Reply-To", "References"}
	for _, k := range headerOrder {
		if v, ok := headers[k]; ok {
			fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
		}
	}

	var payloadToEncrypt []byte
	var innerBodyBytes []byte
	var err error

	// Detect plaintext-only mode
	plaintextOnly := detectPlaintextOnly(plainBody, images, attachments)

	// If plaintext-only mode is requested, build a single text/plain part (or a multipart/signed wrapper when signing)
	if plaintextOnly {
		if len(images) > 0 || len(attachments) > 0 {
			return errors.New("plaintext-only messages cannot contain attachments or inline images")
		}

		// Build quoted-printable encoded body
		var encBody bytes.Buffer
		qp := quotedprintable.NewWriter(&encBody)
		fmt.Fprint(qp, plainBody)
		qp.Close()
		encodedBody := encBody.Bytes()

		// Build the canonical MIME part (headers + body) used for signing/encryption
		var partBuf bytes.Buffer
		fmt.Fprintf(&partBuf, "Content-Type: text/plain; charset=UTF-8; format=flowed\r\n")
		fmt.Fprintf(&partBuf, "Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		partBuf.Write(encodedBody)
		canonicalPart := partBuf.Bytes()

		if signSMIME {
			if account.SMIMECert == "" || account.SMIMEKey == "" {
				return errors.New("S/MIME certificate or key path is missing")
			}

			certData, err := os.ReadFile(account.SMIMECert)
			if err != nil {
				return err
			}
			keyData, err := os.ReadFile(account.SMIMEKey)
			if err != nil {
				return err
			}

			certBlock, _ := pem.Decode(certData)
			if certBlock == nil {
				return errors.New("failed to parse certificate PEM")
			}
			cert, err := x509.ParseCertificate(certBlock.Bytes)
			if err != nil {
				return err
			}

			keyBlock, _ := pem.Decode(keyData)
			if keyBlock == nil {
				return errors.New("failed to parse private key PEM")
			}
			privKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
			if err != nil {
				privKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
				if err != nil {
					return err
				}
			}

			// canonicalize the part (normalize newlines)
			canonicalBody := bytes.ReplaceAll(canonicalPart, []byte("\r\n"), []byte("\n"))
			canonicalBody = bytes.ReplaceAll(canonicalBody, []byte("\n"), []byte("\r\n"))

			signedData, err := pkcs7.NewSignedData(canonicalBody)
			if err != nil {
				return err
			}
			if err := signedData.AddSigner(cert, privKey, pkcs7.SignerInfoConfig{}); err != nil {
				return err
			}
			detachedSig, err := signedData.Finish()
			if err != nil {
				return err
			}

			var rb [12]byte
			var outerBoundary string
			if _, rerr := rand.Read(rb[:]); rerr == nil {
				outerBoundary = "signed-" + fmt.Sprintf("%x", rb[:])
			} else {
				// fallback to time-based boundary if crypto/rand fails
				outerBoundary = "signed-" + fmt.Sprintf("%d", time.Now().UnixNano())
			}
			var signedMsg bytes.Buffer
			fmt.Fprintf(&signedMsg, "Content-Type: multipart/signed; protocol=\"application/pkcs7-signature\"; micalg=\"sha-256\"; boundary=\"%s\"\r\n\r\n", outerBoundary)
			fmt.Fprintf(&signedMsg, "This is a cryptographically signed message in MIME format.\r\n\r\n")
			fmt.Fprintf(&signedMsg, "--%s\r\n", outerBoundary)
			signedMsg.Write(canonicalBody)
			fmt.Fprintf(&signedMsg, "\r\n--%s\r\n", outerBoundary)
			fmt.Fprintf(&signedMsg, "Content-Type: application/pkcs7-signature; name=\"smime.p7s\"\r\n")
			fmt.Fprintf(&signedMsg, "Content-Transfer-Encoding: base64\r\n")
			fmt.Fprintf(&signedMsg, "Content-Disposition: attachment; filename=\"smime.p7s\"\r\n\r\n")
			signedMsg.WriteString(clib.WrapBase64(base64.StdEncoding.EncodeToString(detachedSig)))
			fmt.Fprintf(&signedMsg, "\r\n--%s--\r\n", outerBoundary)

			if encryptSMIME {
				payloadToEncrypt = bytes.ReplaceAll(signedMsg.Bytes(), []byte("\r\n"), []byte("\n"))
				payloadToEncrypt = bytes.ReplaceAll(payloadToEncrypt, []byte("\n"), []byte("\r\n"))
			} else {
				msg.Write(signedMsg.Bytes())
			}
		} else {
			// Not signing: either encrypt the canonical part or send as plain single-part
			canonicalBody := bytes.ReplaceAll(canonicalPart, []byte("\r\n"), []byte("\n"))
			canonicalBody = bytes.ReplaceAll(canonicalBody, []byte("\n"), []byte("\r\n"))
			if encryptSMIME {
				payloadToEncrypt = canonicalBody
			} else {
				// Write Content-Type and body as top-level single part
				fmt.Fprintf(&msg, "Content-Type: text/plain; charset=UTF-8; format=flowed\r\n")
				fmt.Fprintf(&msg, "Content-Transfer-Encoding: quoted-printable\r\n\r\n")
				msg.Write(encodedBody)
			}
		}

	} else {
		// --- Non-plaintext path: build multipart/mixed with related/alternative, images and attachments ---
		var innerMsg bytes.Buffer
		innerWriter := multipart.NewWriter(&innerMsg)
		innerHeaders := fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", innerWriter.Boundary())

		// --- Body Part (multipart/related) ---
		relatedHeader := textproto.MIMEHeader{}
		relatedBoundary := "related-" + innerWriter.Boundary()
		relatedHeader.Set("Content-Type", "multipart/related; boundary=\""+relatedBoundary+"\"")
		relatedPartWriter, err := innerWriter.CreatePart(relatedHeader)
		if err != nil {
			return err
		}
		relatedWriter := multipart.NewWriter(relatedPartWriter)
		relatedWriter.SetBoundary(relatedBoundary)

		// --- Alternative Part (text and html) ---
		altHeader := textproto.MIMEHeader{}
		altBoundary := "alt-" + innerWriter.Boundary()
		altHeader.Set("Content-Type", "multipart/alternative; boundary=\""+altBoundary+"\"")
		altPartWriter, err := relatedWriter.CreatePart(altHeader)
		if err != nil {
			return err
		}
		altWriter := multipart.NewWriter(altPartWriter)
		altWriter.SetBoundary(altBoundary)

		// Plain text part
		textHeader := textproto.MIMEHeader{
			"Content-Type":              {"text/plain; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		}
		textPart, err := altWriter.CreatePart(textHeader)
		if err != nil {
			return err
		}
		qpText := quotedprintable.NewWriter(textPart)
		fmt.Fprint(qpText, plainBody)
		qpText.Close()

		// HTML part
		htmlHeader := textproto.MIMEHeader{
			"Content-Type":              {"text/html; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		}
		htmlPart, err := altWriter.CreatePart(htmlHeader)
		if err != nil {
			return err
		}
		qpHTML := quotedprintable.NewWriter(htmlPart)
		fmt.Fprint(qpHTML, htmlBody)
		qpHTML.Close()

		altWriter.Close() // Finish the alternative part

		// --- Inline Images ---
		for cid, data := range images {
			ext := filepath.Ext(strings.Split(cid, "@")[0])
			mimeType := mime.TypeByExtension(ext)
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}

			imgHeader := textproto.MIMEHeader{}
			imgHeader.Set("Content-Type", mimeType)
			imgHeader.Set("Content-Transfer-Encoding", "base64")
			imgHeader.Set("Content-ID", "<"+cid+">")
			imgHeader.Set("Content-Disposition", "inline; filename=\""+cid+"\"")

			imgPart, err := relatedWriter.CreatePart(imgHeader)
			if err != nil {
				return err
			}
			// Encode raw image bytes to base64, then wrap at 76 chars per MIME rules
			encodedImg := base64.StdEncoding.EncodeToString(data)
			imgPart.Write([]byte(clib.WrapBase64(encodedImg)))
		}

		relatedWriter.Close() // Finish the related part

		// --- Attachments ---
		for filename, data := range attachments {
			mimeType := mime.TypeByExtension(filepath.Ext(filename))
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}

			partHeader := textproto.MIMEHeader{}
			partHeader.Set("Content-Type", mimeType)
			partHeader.Set("Content-Transfer-Encoding", "base64")
			partHeader.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))

			attachmentPart, err := innerWriter.CreatePart(partHeader)
			if err != nil {
				return err
			}
			encodedData := base64.StdEncoding.EncodeToString(data)
			// MIME requires base64 to be line-wrapped at 76 characters
			attachmentPart.Write([]byte(clib.WrapBase64(encodedData)))
		}

		innerWriter.Close() // Finish the inner message

		innerBodyBytes = append([]byte(innerHeaders), innerMsg.Bytes()...)

		// If not signing, and not encrypting, write the multipart body now
		if !signSMIME && !encryptSMIME {
			fmt.Fprintf(&msg, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", innerWriter.Boundary())
			msg.Write(innerMsg.Bytes())
		}
	}

	// Handle S/MIME Detached Signing for non-plaintext messages
	if signSMIME && len(innerBodyBytes) > 0 {
		if account.SMIMECert == "" || account.SMIMEKey == "" {
			return errors.New("S/MIME certificate or key path is missing")
		}

		certData, err := os.ReadFile(account.SMIMECert)
		if err != nil {
			return err
		}
		keyData, err := os.ReadFile(account.SMIMEKey)
		if err != nil {
			return err
		}

		certBlock, _ := pem.Decode(certData)
		if certBlock == nil {
			return errors.New("failed to parse certificate PEM")
		}
		cert, err := x509.ParseCertificate(certBlock.Bytes)
		if err != nil {
			return err
		}

		keyBlock, _ := pem.Decode(keyData)
		if keyBlock == nil {
			return errors.New("failed to parse private key PEM")
		}
		privKey, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
		if err != nil {
			privKey, err = x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
			if err != nil {
				return err
			}
		}

		canonicalBody := bytes.ReplaceAll(innerBodyBytes, []byte("\r\n"), []byte("\n"))
		canonicalBody = bytes.ReplaceAll(canonicalBody, []byte("\n"), []byte("\r\n"))

		signedData, err := pkcs7.NewSignedData(canonicalBody)
		if err != nil {
			return err
		}
		if err := signedData.AddSigner(cert, privKey, pkcs7.SignerInfoConfig{}); err != nil {
			return err
		}
		detachedSig, err := signedData.Finish()
		if err != nil {
			return err
		}

		var rb [12]byte
		var outerBoundary string
		if _, rerr := rand.Read(rb[:]); rerr == nil {
			outerBoundary = "signed-" + fmt.Sprintf("%x", rb[:])
		} else {
			// fallback to time-based boundary if crypto/rand fails
			outerBoundary = "signed-" + fmt.Sprintf("%d", time.Now().UnixNano())
		}
		var signedMsg bytes.Buffer
		fmt.Fprintf(&signedMsg, "Content-Type: multipart/signed; protocol=\"application/pkcs7-signature\"; micalg=\"sha-256\"; boundary=\"%s\"\r\n\r\n", outerBoundary)
		fmt.Fprintf(&signedMsg, "This is a cryptographically signed message in MIME format.\r\n\r\n")
		fmt.Fprintf(&signedMsg, "--%s\r\n", outerBoundary)
		signedMsg.Write(canonicalBody)
		fmt.Fprintf(&signedMsg, "\r\n--%s\r\n", outerBoundary)
		fmt.Fprintf(&signedMsg, "Content-Type: application/pkcs7-signature; name=\"smime.p7s\"\r\n")
		fmt.Fprintf(&signedMsg, "Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&signedMsg, "Content-Disposition: attachment; filename=\"smime.p7s\"\r\n\r\n")
		signedMsg.WriteString(clib.WrapBase64(base64.StdEncoding.EncodeToString(detachedSig)))
		fmt.Fprintf(&signedMsg, "\r\n--%s--\r\n", outerBoundary)

		if encryptSMIME {
			payloadToEncrypt = bytes.ReplaceAll(signedMsg.Bytes(), []byte("\r\n"), []byte("\n"))
			payloadToEncrypt = bytes.ReplaceAll(payloadToEncrypt, []byte("\n"), []byte("\r\n"))
		} else {
			msg.Write(signedMsg.Bytes())
		}
	}

	// Handle S/MIME Encryption
	if encryptSMIME {
		// Include the sender's own email so it can be decrypted in the Sent folder
		allRecipients := append([]string{account.Email}, to...)
		allRecipients = append(allRecipients, cc...)
		allRecipients = append(allRecipients, bcc...)

		cfgDir, _ := config.GetConfigDir()
		certsDir := filepath.Join(cfgDir, "certs")
		var certs []*x509.Certificate

		for _, em := range allRecipients {
			em = strings.TrimSpace(em)
			if strings.Contains(em, "<") {
				parts := strings.Split(em, "<")
				if len(parts) == 2 {
					em = strings.TrimSuffix(parts[1], ">")
				}
			}

			var certPath string
			// If this is our own account, use the path from settings rather than requiring it in the certs folder
			if strings.EqualFold(em, account.Email) && account.SMIMECert != "" {
				certPath = account.SMIMECert
			} else {
				certPath = filepath.Join(certsDir, em+".pem")
			}

			if certData, err := os.ReadFile(certPath); err == nil {
				if block, _ := pem.Decode(certData); block != nil {
					if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
						certs = append(certs, cert)
					}
				}
			}
		}

		if len(certs) == 0 {
			return errors.New("cannot encrypt: no valid public certificates found for recipients")
		}

		encryptedDer, err := pkcs7.Encrypt(payloadToEncrypt, certs)
		if err != nil {
			return err
		}

		msg.WriteString("Content-Type: application/pkcs7-mime; smime-type=enveloped-data; name=\"smime.p7m\"\r\n")
		msg.WriteString("Content-Transfer-Encoding: base64\r\n")
		msg.WriteString("Content-Disposition: attachment; filename=\"smime.p7m\"\r\n\r\n")
		msg.WriteString(clib.WrapBase64(base64.StdEncoding.EncodeToString(encryptedDer)))
	}

	// Handle PGP Signing (if enabled and not already signed with S/MIME)
	var pgpPayload []byte
	if signPGP && !signSMIME {
		// Determine what to sign
		var toSign []byte
		if len(payloadToEncrypt) > 0 {
			// We have content prepared for encryption
			toSign = payloadToEncrypt
		} else {
			// Use what we've built so far
			toSign = msg.Bytes()
		}

		signed, err := signEmailPGP(toSign, account)
		if err != nil {
			return fmt.Errorf("PGP signing failed: %w", err)
		}

		if encryptPGP {
			// Will encrypt the signed message
			pgpPayload = signed
		} else {
			// Not encrypting, so write signed message now
			msg.Reset()
			msg.Write(signed)
		}
	}

	// Handle PGP Encryption (if enabled and not already encrypted with S/MIME)
	if encryptPGP && !encryptSMIME {
		allRecipients := append([]string{}, to...)
		allRecipients = append(allRecipients, cc...)
		allRecipients = append(allRecipients, bcc...)

		var toEncrypt []byte
		if len(pgpPayload) > 0 {
			// Encrypt the signed message
			toEncrypt = pgpPayload
		} else if len(payloadToEncrypt) > 0 {
			// Encrypt pre-prepared payload
			toEncrypt = payloadToEncrypt
		} else {
			// Encrypt what we've built so far
			toEncrypt = msg.Bytes()
		}

		encrypted, err := encryptEmailPGP(toEncrypt, allRecipients, account)
		if err != nil {
			return fmt.Errorf("PGP encryption failed: %w", err)
		}

		msg.Reset()
		msg.Write(encrypted)
	}

	// Combine all recipients for the envelope
	allRecipients := append([]string{}, to...)
	allRecipients = append(allRecipients, cc...)
	allRecipients = append(allRecipients, bcc...)

	addr := fmt.Sprintf("%s:%d", smtpServer, smtpPort)

	tlsConfig := &tls.Config{
		ServerName:         smtpServer,
		InsecureSkipVerify: account.Insecure,
	}

	var c *smtp.Client

	// Port 465 uses implicit TLS (the connection starts with TLS).
	// All other ports use plain TCP with optional STARTTLS upgrade.
	if smtpPort == 465 {
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return err
		}
		c, err = smtp.NewClient(conn, smtpServer)
		if err != nil {
			conn.Close()
			return err
		}
	} else {
		var err error
		c, err = smtp.Dial(addr)
		if err != nil {
			return err
		}
	}
	defer c.Close()

	if err = c.Hello("localhost"); err != nil {
		return err
	}

	// Trigger STARTTLS if supported (not needed for implicit TLS on port 465)
	if smtpPort != 465 {
		if ok, _ := c.Extension("STARTTLS"); ok {
			if err = c.StartTLS(tlsConfig); err != nil {
				return err
			}
		}
	}

	// Authenticate using the best available mechanism.
	// c.Extension("AUTH") returns the list of supported mechanisms.
	if ok, mechs := c.Extension("AUTH"); ok {
		mechList := strings.ToUpper(mechs)

		if account.IsOAuth2() {
			// Use XOAUTH2 for OAuth2-enabled accounts
			token, tokenErr := config.GetOAuth2Token(account.Email)
			if tokenErr != nil {
				return fmt.Errorf("oauth2: %w", tokenErr)
			}
			err = c.Auth(&xoauth2Auth{username: account.Email, token: token})
		} else if strings.Contains(mechList, "PLAIN") {
			err = c.Auth(plainAuth)
		} else if strings.Contains(mechList, "LOGIN") {
			err = c.Auth(loginAuthFallback)
		} else {
			// Fall back to PLAIN and let the server decide
			err = c.Auth(plainAuth)
		}
		if err != nil {
			return err
		}
	}

	// Send Envelope
	if err = c.Mail(account.GetFetchEmail()); err != nil {
		return err
	}
	for _, r := range allRecipients {
		if err = c.Rcpt(r); err != nil {
			return err
		}
	}

	// Write Data
	w, err := c.Data()
	if err != nil {
		return err
	}
	_, err = w.Write(msg.Bytes())
	if err != nil {
		return err
	}
	err = w.Close()
	if err != nil {
		return err
	}

	return c.Quit()
}

// signEmailPGP signs the message payload with PGP and returns a multipart/signed message.
// Supports both file-based keys and YubiKey hardware tokens.
func signEmailPGP(payload []byte, account *config.Account) ([]byte, error) {
	// Check if using YubiKey
	if account.PGPKeySource == "yubikey" {
		return signEmailPGPWithYubiKey(payload, account)
	}

	// Default to file-based signing
	if account.PGPPrivateKey == "" {
		return nil, errors.New("PGP private key path is missing")
	}

	// Load private key
	keyFile, err := os.ReadFile(account.PGPPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read PGP private key: %w", err)
	}

	// Try to parse as armored keyring first
	entityList, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(keyFile))
	if err != nil {
		// Try binary format
		entityList, err = openpgp.ReadKeyRing(bytes.NewReader(keyFile))
		if err != nil {
			return nil, fmt.Errorf("failed to parse PGP key: %w", err)
		}
	}

	if len(entityList) == 0 {
		return nil, errors.New("no PGP keys found in keyring")
	}

	// Decrypt the private key if it's encrypted
	entity := entityList[0]
	if entity.PrivateKey != nil && entity.PrivateKey.Encrypted {
		passphrase := []byte(account.PGPPIN) // reuse PIN field for passphrase
		if err := entity.DecryptPrivateKeys(passphrase); err != nil {
			return nil, fmt.Errorf("failed to decrypt PGP private key: %w", err)
		}
	}

	// Split payload into transport headers (From, To, Subject, etc.) and body.
	// pgpmail.Sign needs the transport headers in its header param so they
	// appear at the top level of the output, not inside the signed part.
	// Content headers (Content-Type, etc.) stay with the body as the signed part.
	var header messagetextproto.Header
	var bodyPayload []byte
	if idx := bytes.Index(payload, []byte("\r\n\r\n")); idx >= 0 {
		headerBytes := payload[:idx]
		rawBody := payload[idx+4:]

		var contentHeaders bytes.Buffer
		for _, line := range bytes.Split(headerBytes, []byte("\r\n")) {
			if len(line) == 0 {
				continue
			}
			parts := bytes.SplitN(line, []byte(": "), 2)
			if len(parts) != 2 {
				continue
			}
			key := string(parts[0])
			val := string(parts[1])
			upper := strings.ToUpper(key)
			if strings.HasPrefix(upper, "CONTENT-") || upper == "MIME-VERSION" {
				// Keep content headers with the body for the signed part
				contentHeaders.Write(line)
				contentHeaders.WriteString("\r\n")
			} else {
				// Transport headers go to the top-level message
				header.Set(key, val)
			}
		}

		// Reconstruct body payload: content headers + blank line + body
		contentHeaders.WriteString("\r\n")
		contentHeaders.Write(rawBody)
		bodyPayload = contentHeaders.Bytes()
	} else {
		bodyPayload = payload
	}

	// Create multipart/signed message using go-pgpmail
	var signed bytes.Buffer

	mw, err := pgpmail.Sign(&signed, header, entity, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create PGP signer: %w", err)
	}

	// Write the body (content headers + body) to be signed
	if _, err := mw.Write(bodyPayload); err != nil {
		return nil, fmt.Errorf("failed to write message for signing: %w", err)
	}

	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize PGP signature: %w", err)
	}

	return signed.Bytes(), nil
}

// signEmailPGPWithYubiKey signs the message payload using a YubiKey hardware token.
func signEmailPGPWithYubiKey(payload []byte, account *config.Account) ([]byte, error) {
	// Get PIN from account (loaded from keyring)
	pin := account.PGPPIN
	if pin == "" {
		return nil, fmt.Errorf("YubiKey PIN not configured - please set it in account settings")
	}

	if account.PGPPublicKey == "" {
		return nil, fmt.Errorf("PGP public key path is required for YubiKey signing")
	}

	// Use the pgp package to sign with YubiKey
	signed, err := pgp.BuildPGPSignedMessage(payload, pin, account.PGPPublicKey)
	if err != nil {
		return nil, fmt.Errorf("YubiKey signing failed: %w", err)
	}
	return signed, nil
}

// encryptEmailPGP encrypts the message payload with PGP and returns a multipart/encrypted message.
func encryptEmailPGP(payload []byte, recipients []string, account *config.Account) ([]byte, error) {
	var entityList openpgp.EntityList

	cfgDir, err := config.GetConfigDir()
	if err != nil {
		return nil, err
	}
	pgpDir := filepath.Join(cfgDir, "pgp")

	// Add recipient keys
	for _, recipient := range recipients {
		// Extract email address from "Name <email>" format
		email := strings.TrimSpace(recipient)
		if strings.Contains(email, "<") {
			parts := strings.Split(email, "<")
			if len(parts) == 2 {
				email = strings.TrimSuffix(parts[1], ">")
			}
		}

		// Try .asc (armored) first, then .gpg (binary)
		var keyData []byte
		keyPath := filepath.Join(pgpDir, email+".asc")
		keyData, err = os.ReadFile(keyPath)
		if err != nil {
			keyPath = filepath.Join(pgpDir, email+".gpg")
			keyData, err = os.ReadFile(keyPath)
			if err != nil {
				return nil, fmt.Errorf("missing PGP key for %s (tried .asc and .gpg): %w", email, err)
			}
		}

		// Try armored format first
		entities, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(keyData))
		if err != nil {
			// Try binary format
			entities, err = openpgp.ReadKeyRing(bytes.NewReader(keyData))
			if err != nil {
				return nil, fmt.Errorf("failed to parse PGP key for %s: %w", email, err)
			}
		}

		if len(entities) > 0 {
			entityList = append(entityList, entities[0])
		}
	}

	// Add sender's own key (to read in Sent folder)
	if account.PGPPublicKey != "" {
		senderKey, err := os.ReadFile(account.PGPPublicKey)
		if err == nil {
			entities, _ := openpgp.ReadArmoredKeyRing(bytes.NewReader(senderKey))
			if entities == nil {
				entities, _ = openpgp.ReadKeyRing(bytes.NewReader(senderKey))
			}
			if entities != nil && len(entities) > 0 {
				entityList = append(entityList, entities[0])
			}
		}
	}

	if len(entityList) == 0 {
		return nil, errors.New("cannot encrypt: no valid PGP public keys found for recipients")
	}

	// Encrypt using go-pgpmail
	var encrypted bytes.Buffer

	// Create a minimal header for the encrypted content
	var header messagetextproto.Header

	mw, err := pgpmail.Encrypt(&encrypted, header, entityList, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create PGP encryptor: %w", err)
	}

	if _, err := mw.Write(payload); err != nil {
		return nil, fmt.Errorf("failed to write message for encryption: %w", err)
	}

	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize PGP encryption: %w", err)
	}

	return encrypted.Bytes(), nil
}
