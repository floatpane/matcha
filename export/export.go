// Package export provides utilities for exporting emails to HTML, Markdown,
// or EML format, including full message metadata and headers parsed from
// the raw RFC822 message.
package export

import (
	"bytes"
	"fmt"
	"html"
	"io"
	"mime"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"
	"github.com/floatpane/matcha/fetcher"
)

const extHTML = "html"

// EmailMetadata holds all parsed header fields from a raw RFC822 message.
type EmailMetadata struct {
	From        string
	To          []string
	Cc          []string
	Bcc         []string
	ReplyTo     []string
	Subject     string
	Date        time.Time
	MessageID   string
	InReplyTo   string
	References  []string
	HasDate     bool
	RawHeaders  []HeaderLine
	ContentType string
}

// HeaderLine is a single raw header key/value pair.
type HeaderLine struct {
	Key   string
	Value string
}

// ParseMetadata extracts all metadata from raw RFC822 message bytes.
func ParseMetadata(rawMsg []byte) (*EmailMetadata, error) {
	r := bytes.NewReader(rawMsg)
	mr, err := gomail.CreateReader(r)
	if err != nil {
		// Fallback: try net/mail for at least basic headers
		return parseMetadataFallback(rawMsg)
	}

	meta := &EmailMetadata{}
	header := mr.Header

	meta.Subject = decodeHeader(header, "Subject")
	meta.From = decodeHeader(header, "From")
	meta.MessageID = strings.TrimSpace(header.Get("Message-ID"))
	meta.InReplyTo = strings.TrimSpace(header.Get("In-Reply-To"))
	meta.ContentType = header.Get("Content-Type")

	if toHeader := header.Get("To"); toHeader != "" {
		meta.To = parseAddressList(toHeader)
	}
	if ccHeader := header.Get("Cc"); ccHeader != "" {
		meta.Cc = parseAddressList(ccHeader)
	}
	if bccHeader := header.Get("Bcc"); bccHeader != "" {
		meta.Bcc = parseAddressList(bccHeader)
	}
	if replyToHeader := header.Get("Reply-To"); replyToHeader != "" {
		meta.ReplyTo = parseAddressList(replyToHeader)
	}
	if refsHeader := header.Get("References"); refsHeader != "" {
		meta.References = parseMessageIDList(refsHeader)
	}

	if dateStr := header.Get("Date"); dateStr != "" {
		if parsed, err := mail.ParseDate(dateStr); err == nil {
			meta.Date = parsed
			meta.HasDate = true
		}
	}

	// Collect all raw headers
	meta.RawHeaders = extractRawHeaders(rawMsg)

	// Close the reader to release resources
	mr.Close() //nolint:errcheck,gosec

	return meta, nil
}

// parseMetadataFallback uses net/mail for basic header parsing when
// go-message can't parse the message.
func parseMetadataFallback(rawMsg []byte) (*EmailMetadata, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(rawMsg))
	if err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	meta := &EmailMetadata{}
	meta.Subject = decodeNetMailHeader(msg.Header, "Subject")
	meta.From = decodeNetMailHeader(msg.Header, "From")
	meta.MessageID = strings.TrimSpace(msg.Header.Get("Message-ID"))
	meta.InReplyTo = strings.TrimSpace(msg.Header.Get("In-Reply-To"))
	meta.ContentType = msg.Header.Get("Content-Type")

	if toHeader := msg.Header.Get("To"); toHeader != "" {
		meta.To = parseAddressList(toHeader)
	}
	if ccHeader := msg.Header.Get("Cc"); ccHeader != "" {
		meta.Cc = parseAddressList(ccHeader)
	}
	if replyToHeader := msg.Header.Get("Reply-To"); replyToHeader != "" {
		meta.ReplyTo = parseAddressList(replyToHeader)
	}
	if refsHeader := msg.Header.Get("References"); refsHeader != "" {
		meta.References = parseMessageIDList(refsHeader)
	}

	if dateStr := msg.Header.Get("Date"); dateStr != "" {
		if parsed, err := mail.ParseDate(dateStr); err == nil {
			meta.Date = parsed
			meta.HasDate = true
		}
	}

	meta.RawHeaders = extractRawHeaders(rawMsg)

	return meta, nil
}

// ExtractBodyHTML returns the HTML body from the raw RFC822 message.
// If the message is plain-text, it returns empty string.
func ExtractBodyHTML(rawMsg []byte) string {
	r := bytes.NewReader(rawMsg)
	mr, err := gomail.CreateReader(r)
	if err != nil {
		return ""
	}
	defer mr.Close() //nolint:errcheck

	var htmlBody string
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		ct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if ct == "text/html" {
			data, _ := io.ReadAll(p.Body)
			if htmlBody == "" {
				htmlBody = string(data)
			}
		}
	}
	return htmlBody
}

// ExtractBodyText returns the plain-text body from the raw RFC822 message.
func ExtractBodyText(rawMsg []byte) string {
	r := bytes.NewReader(rawMsg)
	mr, err := gomail.CreateReader(r)
	if err != nil {
		// Fallback: read everything as text
		msg, mErr := mail.ReadMessage(bytes.NewReader(rawMsg))
		if mErr != nil {
			return string(rawMsg)
		}
		body, _ := io.ReadAll(msg.Body)
		return string(body)
	}
	defer mr.Close() //nolint:errcheck

	var textBody string
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		ct, _, _ := mime.ParseMediaType(p.Header.Get("Content-Type"))
		if ct == "text/plain" {
			data, _ := io.ReadAll(p.Body)
			if textBody == "" {
				textBody = string(data)
			}
		}
	}
	return textBody
}

// EmailToHTML generates a self-contained HTML file from the raw RFC822 message
// and the parsed fetcher.Email. It includes full metadata, all headers, and
// the email body (sanitized). If the body is plain-text, it is converted to
// HTML.
func EmailToHTML(rawMsg []byte, email fetcher.Email) ([]byte, error) {
	meta, err := ParseMetadata(rawMsg)
	if err != nil {
		// Fall back to the fetcher.Email fields if we can't parse the raw message
		meta = &EmailMetadata{
			From:       email.From,
			To:         email.To,
			ReplyTo:    email.ReplyTo,
			Subject:    email.Subject,
			Date:       email.Date,
			HasDate:    !email.Date.IsZero(),
			MessageID:  email.MessageID,
			InReplyTo:  email.InReplyTo,
			References: email.References,
		}
	}

	bodyHTML := ExtractBodyHTML(rawMsg)
	if bodyHTML == "" {
		// Use the fetcher.Email body
		if email.BodyMIMEType == "text/html" {
			bodyHTML = email.Body
		} else {
			// Convert plain text to HTML
			bodyHTML = textToHTML(email.Body)
		}
	}

	if bodyHTML == "" {
		textBody := ExtractBodyText(rawMsg)
		if textBody == "" {
			textBody = email.Body
		}
		bodyHTML = textToHTML(textBody)
	}

	var buf bytes.Buffer

	// HTML head with inline CSS for a clean email view
	buf.WriteString(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
`)
	buf.WriteString("<title>")
	buf.WriteString(html.EscapeString(meta.Subject))
	buf.WriteString("</title>\n")
	buf.WriteString(`<style>
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 0; color: #1a1a1a; }
.meta-bar { background: #f5f5f5; border-bottom: 1px solid #ddd; padding: 16px 24px; }
.meta-bar h1 { margin: 0 0 12px 0; font-size: 1.4em; line-height: 1.3; }
.meta-table { width: 100%; border-collapse: collapse; font-size: 0.9em; }
.meta-table td { padding: 2px 8px 2px 0; vertical-align: top; }
.meta-table td:first-child { color: #666; font-weight: 600; white-space: nowrap; width: 80px; }
.meta-table td:last-child { color: #333; }
.headers-details { margin-top: 12px; }
.headers-details summary { cursor: pointer; color: #666; font-size: 0.85em; user-select: none; }
.headers-details pre { background: #fafafa; border: 1px solid #eee; padding: 12px; font-size: 0.8em; overflow-x: auto; white-space: pre-wrap; word-break: break-all; margin-top: 8px; border-radius: 4px; }
.body-content { padding: 24px; max-width: 900px; margin: 0 auto; }
.body-content img { max-width: 100%; height: auto; }
.attachments { margin: 16px 24px; padding: 12px 16px; background: #fafafa; border: 1px solid #eee; border-radius: 4px; }
.attachments h3 { margin: 0 0 8px 0; font-size: 0.9em; color: #666; }
.attachments ul { margin: 0; padding-left: 20px; }
.attachments li { font-size: 0.85em; color: #333; margin: 2px 0; }
</style>
</head>
<body>
`)

	// Metadata bar
	buf.WriteString(`<div class="meta-bar">` + "\n")
	buf.WriteString("<h1>" + html.EscapeString(meta.Subject) + "</h1>\n")
	buf.WriteString(`<table class="meta-table">` + "\n")

	writeMetaRow := func(label, value string) {
		if value == "" {
			return
		}
		buf.WriteString("  <tr><td>")
		buf.WriteString(html.EscapeString(label))
		buf.WriteString("</td><td>")
		buf.WriteString(html.EscapeString(value))
		buf.WriteString("</td></tr>\n")
	}

	writeMetaRow("From:", meta.From)
	if len(meta.To) > 0 {
		writeMetaRow("To:", strings.Join(meta.To, ", "))
	}
	if len(meta.Cc) > 0 {
		writeMetaRow("Cc:", strings.Join(meta.Cc, ", "))
	}
	if len(meta.Bcc) > 0 {
		writeMetaRow("Bcc:", strings.Join(meta.Bcc, ", "))
	}
	if len(meta.ReplyTo) > 0 {
		writeMetaRow("Reply-To:", strings.Join(meta.ReplyTo, ", "))
	}
	if meta.HasDate {
		writeMetaRow("Date:", meta.Date.Format("Mon, 02 Jan 2006 15:04:05 -0700"))
	}
	writeMetaRow("Message-ID:", meta.MessageID)
	if meta.InReplyTo != "" {
		writeMetaRow("In-Reply-To:", meta.InReplyTo)
	}
	if len(meta.References) > 0 {
		writeMetaRow("References:", strings.Join(meta.References, " "))
	}

	buf.WriteString("</table>\n")

	// Full headers in a collapsible section
	if len(meta.RawHeaders) > 0 {
		buf.WriteString(`<details class="headers-details">` + "\n")
		buf.WriteString("<summary>All Headers (" + fmt.Sprintf("%d)", len(meta.RawHeaders)) + "</summary>\n")
		buf.WriteString("<pre>")
		for _, h := range meta.RawHeaders {
			buf.WriteString(html.EscapeString(h.Key + ": " + h.Value + "\n"))
		}
		buf.WriteString("</pre>\n")
		buf.WriteString("</details>\n")
	}

	buf.WriteString("</div>\n") // close meta-bar

	// Attachments
	if len(email.Attachments) > 0 {
		buf.WriteString(`<div class="attachments">` + "\n")
		buf.WriteString("<h3>Attachments</h3>\n<ul>\n")
		for _, att := range email.Attachments {
			size := formatSize(len(att.Data))
			buf.WriteString("  <li>")
			buf.WriteString(html.EscapeString(att.Filename))
			if att.MIMEType != "" {
				buf.WriteString(" <em>(" + html.EscapeString(att.MIMEType) + ")</em>")
			}
			if size != "" {
				buf.WriteString(" &mdash; " + size)
			}
			buf.WriteString("</li>\n")
		}
		buf.WriteString("</ul>\n</div>\n")
	}

	// Body content
	buf.WriteString(`<div class="body-content">` + "\n")
	buf.WriteString(bodyHTML)
	buf.WriteString("\n</div>\n")

	buf.WriteString("</body>\n</html>\n")

	return buf.Bytes(), nil
}

// EmailToMarkdown generates a Markdown representation of the email with
// full metadata and all headers.
func EmailToMarkdown(rawMsg []byte, email fetcher.Email) ([]byte, error) {
	meta, err := ParseMetadata(rawMsg)
	if err != nil {
		meta = &EmailMetadata{
			From:       email.From,
			To:         email.To,
			ReplyTo:    email.ReplyTo,
			Subject:    email.Subject,
			Date:       email.Date,
			HasDate:    !email.Date.IsZero(),
			MessageID:  email.MessageID,
			InReplyTo:  email.InReplyTo,
			References: email.References,
		}
	}

	var buf bytes.Buffer

	buf.WriteString("# " + meta.Subject + "\n\n")

	buf.WriteString("| Field | Value |\n")
	buf.WriteString("|-------|-------|\n")
	writeMDRow := func(label, value string) {
		if value == "" {
			return
		}
		// Escape pipe characters in markdown table cells
		value = strings.ReplaceAll(value, "|", "\\|")
		value = strings.ReplaceAll(value, "\n", " ")
		buf.WriteString("| " + label + " | " + value + " |\n")
	}

	writeMDRow("From", meta.From)
	if len(meta.To) > 0 {
		writeMDRow("To", strings.Join(meta.To, ", "))
	}
	if len(meta.Cc) > 0 {
		writeMDRow("Cc", strings.Join(meta.Cc, ", "))
	}
	if len(meta.Bcc) > 0 {
		writeMDRow("Bcc", strings.Join(meta.Bcc, ", "))
	}
	if len(meta.ReplyTo) > 0 {
		writeMDRow("Reply-To", strings.Join(meta.ReplyTo, ", "))
	}
	if meta.HasDate {
		writeMDRow("Date", meta.Date.Format("Mon, 02 Jan 2006 15:04:05 -0700"))
	}
	writeMDRow("Message-ID", meta.MessageID)
	if meta.InReplyTo != "" {
		writeMDRow("In-Reply-To", meta.InReplyTo)
	}
	if len(meta.References) > 0 {
		writeMDRow("References", strings.Join(meta.References, " "))
	}

	buf.WriteString("\n")

	// Full headers
	if len(meta.RawHeaders) > 0 {
		buf.WriteString("<details>\n<summary>All Headers</summary>\n\n")
		buf.WriteString("```\n")
		for _, h := range meta.RawHeaders {
			buf.WriteString(h.Key + ": " + h.Value + "\n")
		}
		buf.WriteString("```\n\n")
		buf.WriteString("</details>\n\n")
	}

	// Attachments
	if len(email.Attachments) > 0 {
		buf.WriteString("## Attachments\n\n")
		for _, att := range email.Attachments {
			size := formatSize(len(att.Data))
			line := "- " + att.Filename
			if att.MIMEType != "" {
				line += " (" + att.MIMEType + ")"
			}
			if size != "" {
				line += " — " + size
			}
			buf.WriteString(line + "\n")
		}
		buf.WriteString("\n")
	}

	// Body
	buf.WriteString("## Body\n\n")

	// Try to get the body text
	bodyText := ExtractBodyText(rawMsg)
	if bodyText == "" {
		if email.BodyMIMEType == "text/plain" {
			bodyText = email.Body
		}
	}

	if bodyText != "" {
		buf.WriteString(bodyText)
		buf.WriteString("\n")
	} else {
		// If only HTML is available, include the raw HTML in a code block
		htmlBody := ExtractBodyHTML(rawMsg)
		if htmlBody == "" && email.BodyMIMEType == "text/html" {
			htmlBody = email.Body
		}
		if htmlBody != "" {
			buf.WriteString("```html\n")
			buf.WriteString(htmlBody)
			buf.WriteString("\n```\n")
		} else {
			buf.WriteString("(no body content)\n")
		}
	}

	return buf.Bytes(), nil
}

// textToHTML converts plain text to basic HTML, preserving line breaks.
func textToHTML(text string) string {
	escaped := html.EscapeString(text)
	// Convert line breaks to <br> and wrap in a <pre> for fidelity
	return "<pre style=\"white-space: pre-wrap; font-family: monospace;\">" + escaped + "</pre>"
}

// parseAddressList parses a comma-separated address header into a list of
// email addresses.
func parseAddressList(header string) []string {
	addrs, err := mail.ParseAddressList(header)
	if err != nil {
		return []string{header}
	}
	var result []string
	for _, addr := range addrs {
		if addr.Name != "" {
			result = append(result, fmt.Sprintf("%s <%s>", addr.Name, addr.Address))
		} else {
			result = append(result, addr.Address)
		}
	}
	return result
}

// parseMessageIDList extracts message IDs from a References header.
func parseMessageIDList(header string) []string {
	fields := strings.Fields(header)
	var result []string
	for _, f := range fields {
		f = strings.Trim(f, "<>")
		if f != "" {
			result = append(result, "<"+f+">")
		}
	}
	return result
}

// decodeHeader decodes a MIME-encoded header value using mime.WordDecoder.
func decodeHeader(header gomail.Header, key string) string {
	val := header.Get(key)
	if val == "" {
		return ""
	}
	dec := new(mime.WordDecoder)
	if decoded, err := dec.DecodeHeader(val); err == nil {
		return decoded
	}
	return val
}

// decodeNetMailHeader decodes a MIME-encoded header from a net/mail.Header.
func decodeNetMailHeader(header mail.Header, key string) string {
	val := header.Get(key)
	if val == "" {
		return ""
	}
	dec := new(mime.WordDecoder)
	if decoded, err := dec.DecodeHeader(val); err == nil {
		return decoded
	}
	return val
}

// extractRawHeaders extracts all raw header key-value pairs from the raw
// message bytes. It reads until the first blank line (end of headers).
func extractRawHeaders(rawMsg []byte) []HeaderLine {
	var headers []HeaderLine
	lines := strings.Split(string(rawMsg), "\n")

	var currentKey, currentValue string
	for _, line := range lines {
		// End of headers
		if strings.TrimSpace(line) == "" {
			if currentKey != "" {
				headers = append(headers, HeaderLine{Key: currentKey, Value: currentValue})
			}
			break
		}
		// Continuation line (starts with whitespace)
		if (len(line) > 0 && (line[0] == ' ' || line[0] == '\t')) && currentKey != "" {
			currentValue += " " + strings.TrimSpace(line)
			continue
		}
		// New header
		if currentKey != "" {
			headers = append(headers, HeaderLine{Key: currentKey, Value: currentValue})
		}
		idx := strings.Index(line, ":")
		if idx > 0 {
			currentKey = strings.TrimSpace(line[:idx])
			currentValue = strings.TrimSpace(line[idx+1:])
		} else {
			currentKey = ""
			currentValue = ""
		}
	}
	if currentKey != "" {
		headers = append(headers, HeaderLine{Key: currentKey, Value: currentValue})
	}

	return headers
}

// formatSize returns a human-readable file size string.
func formatSize(bytes int) string {
	if bytes == 0 {
		return ""
	}
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(bytes)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(bytes)/(1024*1024))
}

// SuggestFilename generates a safe filename suggestion from an email subject
// and the desired format extension.
func SuggestFilename(subject, format string) string {
	ext := extHTML
	switch format {
	case "markdown", "md":
		ext = "md"
	case "eml":
		ext = "eml"
	}

	// Sanitize the subject for use as a filename
	name := subject
	if name == "" {
		name = "email"
	}
	// Replace characters that are invalid in filenames
	name = strings.ReplaceAll(name, "/", "-")
	name = strings.ReplaceAll(name, "\\", "-")
	name = strings.ReplaceAll(name, ":", "-")
	name = strings.ReplaceAll(name, "*", "-")
	name = strings.ReplaceAll(name, "?", "-")
	name = strings.ReplaceAll(name, "\"", "-")
	name = strings.ReplaceAll(name, "<", "-")
	name = strings.ReplaceAll(name, ">", "-")
	name = strings.ReplaceAll(name, "|", "-")
	name = strings.ReplaceAll(name, "\n", " ")
	name = strings.ReplaceAll(name, "\r", " ")
	// Trim and limit length
	name = strings.TrimSpace(name)
	if len(name) > 80 {
		name = name[:80]
	}
	name = strings.TrimSpace(name)

	return name + "." + ext
}

// WriteToFile writes data to the given file path, creating the file
// exclusively (not overwriting existing files).
func WriteToFile(path string, data []byte) error {
	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0644)
}
