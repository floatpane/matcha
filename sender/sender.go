package sender

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mime"
	"mime/multipart"
	"net/smtp"
	"net/textproto"
	"path/filepath"
	"strings"
	"time"

	"github.com/floatpane/matcha/config"
)

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
func SendEmail(account *config.Account, to, cc, bcc []string, subject, plainBody, htmlBody string, images map[string][]byte, attachments map[string][]byte, inReplyTo string, references []string) error {
	smtpServer := account.GetSMTPServer()
	smtpPort := account.GetSMTPPort()

	if smtpServer == "" {
		return fmt.Errorf("unsupported or missing service_provider: %s", account.ServiceProvider)
	}

	auth := smtp.PlainAuth("", account.Email, account.Password, smtpServer)

	fromHeader := account.FetchEmail
	if account.Name != "" {
		fromHeader = fmt.Sprintf("%s <%s>", account.Name, account.FetchEmail)
	}

	// Main message buffer
	var msg bytes.Buffer
	mainWriter := multipart.NewWriter(&msg)

	// Set top-level headers for a mixed message type to support content and attachments
	headers := map[string]string{
		"From":         fromHeader,
		"To":           strings.Join(to, ", "),
		"Subject":      subject,
		"Date":         time.Now().Format(time.RFC1123Z),
		"Message-ID":   generateMessageID(account.FetchEmail),
		"Content-Type": "multipart/mixed; boundary=" + mainWriter.Boundary(),
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

	for k, v := range headers {
		fmt.Fprintf(&msg, "%s: %s\r\n", k, v)
	}
	fmt.Fprintf(&msg, "\r\n") // End of headers

	// --- Body Part (multipart/related) ---
	// This part contains the multipart/alternative (text/html) and any inline images.
	relatedHeader := textproto.MIMEHeader{}
	relatedBoundary := "related-" + mainWriter.Boundary()
	relatedHeader.Set("Content-Type", "multipart/related; boundary="+relatedBoundary)
	relatedPartWriter, err := mainWriter.CreatePart(relatedHeader)
	if err != nil {
		return err
	}
	relatedWriter := multipart.NewWriter(relatedPartWriter)
	relatedWriter.SetBoundary(relatedBoundary)

	// --- Alternative Part (text and html) ---
	altHeader := textproto.MIMEHeader{}
	altBoundary := "alt-" + mainWriter.Boundary()
	altHeader.Set("Content-Type", "multipart/alternative; boundary="+altBoundary)
	altPartWriter, err := relatedWriter.CreatePart(altHeader)
	if err != nil {
		return err
	}
	altWriter := multipart.NewWriter(altPartWriter)
	altWriter.SetBoundary(altBoundary)

	// Plain text part
	textPart, err := altWriter.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/plain; charset=UTF-8"}})
	if err != nil {
		return err
	}
	fmt.Fprint(textPart, plainBody)

	// HTML part
	htmlPart, err := altWriter.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/html; charset=UTF-8"}})
	if err != nil {
		return err
	}
	fmt.Fprint(htmlPart, htmlBody)

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

		attachmentPart, err := mainWriter.CreatePart(partHeader)
		if err != nil {
			return err
		}
		encodedData := base64.StdEncoding.EncodeToString(data)
		// MIME requires base64 to be line-wrapped at 76 characters
		attachmentPart.Write([]byte(wrapBase64(encodedData)))
	}

	mainWriter.Close() // Finish the main message

	// Combine all recipients for the envelope
	allRecipients := append([]string{}, to...)
	allRecipients = append(allRecipients, cc...)
	allRecipients = append(allRecipients, bcc...)

	addr := fmt.Sprintf("%s:%d", smtpServer, smtpPort)
	return smtp.SendMail(addr, auth, account.Email, allRecipients, msg.Bytes())
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
