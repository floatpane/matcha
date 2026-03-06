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

	"github.com/floatpane/matcha/config"
	"go.mozilla.org/pkcs7"
)

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

// SendEmail constructs a multipart message with plain text, HTML, embedded images, and attachments.
func SendEmail(account *config.Account, to, cc, bcc []string, subject, plainBody, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string, signSMIME bool, encryptSMIME bool) error {
	smtpServer := account.GetSMTPServer()
	smtpPort := account.GetSMTPPort()

	if smtpServer == "" {
		return fmt.Errorf("unsupported or missing service_provider: %s", account.ServiceProvider)
	}

	plainAuth := smtp.PlainAuth("", account.Email, account.Password, smtpServer)
	loginAuthFallback := &loginAuth{username: account.Email, password: account.Password}

	fromHeader := account.FetchEmail
	if account.Name != "" {
		fromHeader = fmt.Sprintf("%s <%s>", account.Name, account.FetchEmail)
	}

	// Main message buffer
	var innerMsg bytes.Buffer
	innerWriter := multipart.NewWriter(&innerMsg)
	innerHeaders := fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", innerWriter.Boundary())

	// Set top-level headers for a mixed message type to support content and attachments
	headers := map[string]string{
		"From":         fromHeader,
		"To":           strings.Join(to, ", "),
		"Subject":      subject,
		"Date":         time.Now().Format(time.RFC1123Z),
		"Message-ID":   generateMessageID(account.FetchEmail),
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

	// --- Body Part (multipart/related) ---
	// This part contains the multipart/alternative (text/html) and any inline images.
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
		// data is already base64 encoded, but needs MIME line wrapping (76 chars per line)
		imgPart.Write([]byte(wrapBase64(string(data))))
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
		attachmentPart.Write([]byte(wrapBase64(encodedData)))
	}

	innerWriter.Close() // Finish the inner message

	var msg bytes.Buffer
	for k, v := range headers {
		fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
	}

	innerBodyBytes := append([]byte(innerHeaders), innerMsg.Bytes()...)

	var payloadToEncrypt []byte

	// Handle S/MIME Detached Signing
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

		outerBoundary := "signed-" + innerWriter.Boundary()
		var signedMsg bytes.Buffer
		fmt.Fprintf(&signedMsg, "Content-Type: multipart/signed; protocol=\"application/pkcs7-signature\"; micalg=\"sha-256\"; boundary=\"%s\"\r\n\r\n", outerBoundary)
		fmt.Fprintf(&signedMsg, "This is a cryptographically signed message in MIME format.\r\n\r\n")
		fmt.Fprintf(&signedMsg, "--%s\r\n", outerBoundary)
		signedMsg.Write(canonicalBody)
		fmt.Fprintf(&signedMsg, "\r\n--%s\r\n", outerBoundary)
		fmt.Fprintf(&signedMsg, "Content-Type: application/pkcs7-signature; name=\"smime.p7s\"\r\n")
		fmt.Fprintf(&signedMsg, "Content-Transfer-Encoding: base64\r\n")
		fmt.Fprintf(&signedMsg, "Content-Disposition: attachment; filename=\"smime.p7s\"\r\n\r\n")
		signedMsg.WriteString(wrapBase64(base64.StdEncoding.EncodeToString(detachedSig)))
		fmt.Fprintf(&signedMsg, "\r\n--%s--\r\n", outerBoundary)

		if encryptSMIME {
			payloadToEncrypt = bytes.ReplaceAll(signedMsg.Bytes(), []byte("\r\n"), []byte("\n"))
			payloadToEncrypt = bytes.ReplaceAll(payloadToEncrypt, []byte("\n"), []byte("\r\n"))
		} else {
			msg.Write(signedMsg.Bytes())
		}
	} else {
		canonicalBody := bytes.ReplaceAll(innerBodyBytes, []byte("\r\n"), []byte("\n"))
		canonicalBody = bytes.ReplaceAll(canonicalBody, []byte("\n"), []byte("\r\n"))

		if encryptSMIME {
			payloadToEncrypt = canonicalBody
		} else {
			fmt.Fprintf(&msg, "Content-Type: multipart/mixed; boundary=\"%s\"\r\n\r\n", innerWriter.Boundary())
			msg.Write(innerMsg.Bytes())
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
		msg.WriteString(wrapBase64(base64.StdEncoding.EncodeToString(encryptedDer)))
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
		if strings.Contains(mechList, "PLAIN") {
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
	if err = c.Mail(account.Email); err != nil {
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

// wrapBase64 wraps base64-encoded data at 76 characters per line as required by MIME.
func wrapBase64(data string) string {
	const lineLength = 76
	var result strings.Builder
	for i := 0; i < len(data); i += lineLength {
		end := i + lineLength
		if end > len(data) {
			end = len(data)
		}
		result.WriteString(data[i:end])
		if end < len(data) {
			result.WriteString("\r\n")
		}
	}
	return result.String()
}
