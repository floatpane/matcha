package fetcher

import (
	"bufio"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/floatpane/matcha/config"
)

// imapQuote wraps a string in double quotes, escaping any backslash and
// double-quote characters, for use in raw IMAP commands.
func imapQuote(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return "\"" + s + "\""
}

// isGmailAccount reports whether the account is configured for Gmail.
func isGmailAccount(account *config.Account) bool {
	return account != nil && strings.EqualFold(account.ServiceProvider, config.ProviderGmail)
}

// fetchGmailLabelsForBatch fetches X-GM-LABELS for the UIDs in the given batch
// of emails. It returns a map of UID to labels.
func fetchGmailLabelsForBatch(account *config.Account, mailbox string, emails []Email) (map[uint32][]string, error) {
	if len(emails) == 0 {
		return nil, nil
	}
	var uidSet imap.UIDSet
	for _, e := range emails {
		uidSet.AddNum(imap.UID(e.UID))
	}
	return fetchGmailLabels(account, mailbox, uidSet)
}

// AddGmailLabel adds a Gmail label to the specified email on the server.
func AddGmailLabel(account *config.Account, mailbox string, uid uint32, label string) error {
	if !isGmailAccount(account) {
		return fmt.Errorf("gmail labels: not a Gmail account")
	}
	return storeGmailLabels(account, mailbox, uid, "+", []string{label})
}

// RemoveGmailLabel removes a Gmail label from the specified email on the server.
func RemoveGmailLabel(account *config.Account, mailbox string, uid uint32, label string) error {
	if !isGmailAccount(account) {
		return fmt.Errorf("gmail labels: not a Gmail account")
	}
	return storeGmailLabels(account, mailbox, uid, "-", []string{label})
}

// isGmailSystemLabel reports whether the label name (without backslash) is a
// Gmail system label that requires a \ prefix in IMAP commands.
var gmailSystemLabels = map[string]bool{
	"Inbox":     true,
	"Starred":   true,
	"Important": true,
	"Sent":      true,
	"Drafts":    true,
	"Trash":     true,
	"Spam":      true,
	"All":       true,
}

func isGmailSystemLabel(label string) bool {
	return gmailSystemLabels[label]
}

// fetchGmailLabels connects to the IMAP server, selects the given mailbox, and
// issues a raw UID FETCH ... X-GM-LABELS command. It returns a map of UID to
// the list of Gmail labels for that message.
//
// The go-imap/v2 library does not understand the X-GM-LABELS extension, so we
// perform a raw IMAP session over a separate TLS connection. This keeps the
// label fetch self-contained and avoids modifying the library.
func fetchGmailLabels(account *config.Account, mailbox string, uidSet imap.UIDSet) (map[uint32][]string, error) { //nolint:gocyclo
	conn, br, bw, err := rawIMAPConnect(account)
	if err != nil {
		return nil, err
	}
	defer conn.Close() //nolint:errcheck

	// Select mailbox
	selectResp, err := rawIMAPCommand(br, bw, "SELECT "+imapQuote(mailbox))
	if err != nil {
		return nil, fmt.Errorf("gmail labels: SELECT failed: %w", err)
	}
	_ = selectResp // we don't need the response data

	// UID FETCH <set> (UID X-GM-LABELS)
	fetchResp, err := rawIMAPCommand(br, bw, "UID FETCH "+uidSet.String()+" (UID X-GM-LABELS)")
	if err != nil {
		return nil, fmt.Errorf("gmail labels: FETCH failed: %w", err)
	}

	labelsByUID := make(map[uint32][]string)
	for _, line := range fetchResp.untagged {
		uid, labels, ok := parseGmailLabelsLine(line)
		if ok {
			labelsByUID[uid] = labels
		}
	}

	// Logout
	_, _ = rawIMAPCommand(br, bw, "LOGOUT")
	return labelsByUID, nil
}

// storeGmailLabels connects to the IMAP server, selects the mailbox, and issues
// a raw UID STORE command to add or remove Gmail labels.
// op is "+" to add labels or "-" to remove labels.
func storeGmailLabels(account *config.Account, mailbox string, uid uint32, op string, labels []string) error {
	conn, br, bw, err := rawIMAPConnect(account)
	if err != nil {
		return err
	}
	defer conn.Close() //nolint:errcheck

	if _, err := rawIMAPCommand(br, bw, "SELECT "+imapQuote(mailbox)); err != nil {
		return fmt.Errorf("gmail labels: SELECT failed: %w", err)
	}

	quoted := make([]string, len(labels))
	for i, l := range labels {
		// Restore backslash prefix for Gmail system labels
		if isGmailSystemLabel(l) {
			l = "\\" + l
		}
		quoted[i] = imapQuote(l)
	}
	labelList := "(" + strings.Join(quoted, " ") + ")"

	cmd := fmt.Sprintf("UID STORE %d %sX-GM-LABELS %s", uid, op, labelList)
	if _, err := rawIMAPCommand(br, bw, cmd); err != nil {
		return fmt.Errorf("gmail labels: STORE failed: %w", err)
	}

	_, _ = rawIMAPCommand(br, bw, "LOGOUT")
	return nil
}

// --- raw IMAP protocol helpers ---

var rawIMAPTagCounter atomic.Uint64

type rawIMAPResponse struct {
	tagged   string
	untagged []string
}

func rawIMAPConnect(account *config.Account) (net.Conn, *bufio.Reader, *bufio.Writer, error) {
	imapServer := account.GetIMAPServer()
	imapPort := account.GetIMAPPort()
	if imapServer == "" {
		return nil, nil, nil, fmt.Errorf("unsupported service_provider: %s", account.ServiceProvider)
	}

	addr := net.JoinHostPort(imapServer, strconv.Itoa(imapPort))
	tlsConfig := &tls.Config{
		ServerName:         imapServer,
		InsecureSkipVerify: account.Insecure, //nolint:gosec
		MinVersion:         tls.VersionTLS12,
	}

	var conn net.Conn
	var err error

	if imapPort == 1143 || imapPort == 143 {
		// Plain TCP + STARTTLS
		rawConn, err := net.DialTimeout("tcp", addr, 30*time.Second)
		if err != nil {
			return nil, nil, nil, err
		}
		br := bufio.NewReader(rawConn)
		bw := bufio.NewWriter(rawConn)

		// Read greeting
		greeting, err := br.ReadString('\n')
		if err != nil {
			rawConn.Close() //nolint:errcheck
			return nil, nil, nil, fmt.Errorf("gmail labels: failed to read greeting: %w", err)
		}
		if !strings.HasPrefix(greeting, "* OK") && !strings.HasPrefix(greeting, "* PREAUTH") {
			rawConn.Close() //nolint:errcheck
			return nil, nil, nil, fmt.Errorf("gmail labels: bad greeting: %s", strings.TrimSpace(greeting))
		}

		// Send STARTTLS
		if _, err := rawIMAPCommand(br, bw, "STARTTLS"); err != nil {
			rawConn.Close() //nolint:errcheck
			return nil, nil, nil, fmt.Errorf("gmail labels: STARTTLS failed: %w", err)
		}

		// Upgrade to TLS
		tlsConn := tls.Client(rawConn, tlsConfig)
		if err := tlsConn.Handshake(); err != nil {
			rawConn.Close() //nolint:errcheck
			return nil, nil, nil, fmt.Errorf("gmail labels: TLS handshake failed: %w", err)
		}

		conn = tlsConn
		br = bufio.NewReader(conn)
		bw = bufio.NewWriter(conn)

		// Re-read greeting is not sent after STARTTLS; proceed to auth
		// We need to authenticate now
		return rawIMAPAuthAndReturn(conn, br, bw, account)
	}

	conn, err = tls.Dial("tcp", addr, tlsConfig)
	if err != nil {
		return nil, nil, nil, err
	}

	br := bufio.NewReader(conn)
	bw := bufio.NewWriter(conn)

	// Read greeting
	greeting, err := br.ReadString('\n')
	if err != nil {
		conn.Close() //nolint:errcheck
		return nil, nil, nil, fmt.Errorf("gmail labels: failed to read greeting: %w", err)
	}
	if !strings.HasPrefix(greeting, "* OK") && !strings.HasPrefix(greeting, "* PREAUTH") {
		conn.Close() //nolint:errcheck
		return nil, nil, nil, fmt.Errorf("gmail labels: bad greeting: %s", strings.TrimSpace(greeting))
	}

	return rawIMAPAuthAndReturn(conn, br, bw, account)
}

// rawIMAPAuthAndReturn authenticates the connection and returns the ready-to-use
// reader/writer.
func rawIMAPAuthAndReturn(conn net.Conn, br *bufio.Reader, bw *bufio.Writer, account *config.Account) (net.Conn, *bufio.Reader, *bufio.Writer, error) {
	if account.IsOAuth2() {
		token, err := config.GetOAuth2Token(account.Email)
		if err != nil {
			conn.Close() //nolint:errcheck
			return nil, nil, nil, fmt.Errorf("gmail labels: oauth2: %w", err)
		}
		authStr := fmt.Sprintf("user=%s\x01auth=Bearer %s\x01\x01", account.Email, token)
		encoded := base64.StdEncoding.EncodeToString([]byte(authStr))
		if _, err := rawIMAPCommand(br, bw, "AUTHENTICATE XOAUTH2 "+encoded); err != nil {
			conn.Close() //nolint:errcheck
			return nil, nil, nil, fmt.Errorf("gmail labels: XOAUTH2 failed: %w", err)
		}
	} else {
		if _, err := rawIMAPCommand(br, bw, fmt.Sprintf("LOGIN %s %s", imapQuote(account.Email), imapQuote(account.Password))); err != nil {
			conn.Close() //nolint:errcheck
			return nil, nil, nil, fmt.Errorf("gmail labels: LOGIN failed: %w", err)
		}
	}
	return conn, br, bw, nil
}

// rawIMAPCommand sends a tagged IMAP command and reads all responses until the
// tagged OK/NO/BAD response.
func rawIMAPCommand(br *bufio.Reader, bw *bufio.Writer, cmd string) (*rawIMAPResponse, error) {
	tag := fmt.Sprintf("GML%d", rawIMAPTagCounter.Add(1))
	fullCmd := tag + " " + cmd + "\r\n"

	if _, err := bw.WriteString(fullCmd); err != nil {
		return nil, err
	}
	if err := bw.Flush(); err != nil {
		return nil, err
	}

	resp := &rawIMAPResponse{}
	for {
		line, err := readIMAPLine(br)
		if err != nil {
			return nil, err
		}

		if strings.HasPrefix(line, tag+" ") {
			resp.tagged = line
			if strings.Contains(line, " OK ") || strings.Contains(line, " OK\r") || strings.HasSuffix(line, " OK") {
				return resp, nil
			}
			return resp, fmt.Errorf("IMAP error: %s", strings.TrimSpace(line))
		}

		if strings.HasPrefix(line, "* ") {
			resp.untagged = append(resp.untagged, line)
		}

		// Continuation requests (e.g. during AUTHENTICATE) — just send empty line
		if strings.HasPrefix(line, "+ ") {
			if _, err := bw.WriteString("\r\n"); err != nil {
				return nil, err
			}
			if err := bw.Flush(); err != nil {
				return nil, err
			}
		}
	}
}

// readIMAPLine reads a single line from the IMAP response, handling literal
// strings ({NNN} octet counts) by continuing to read until the full literal is
// consumed.
func readIMAPLine(br *bufio.Reader) (string, error) {
	line, err := br.ReadString('\n')
	if err != nil {
		return "", err
	}

	// Handle literal strings: if the line ends with {NNN}\r\n, read that many bytes
	// and append them, then continue reading the rest of the line.
	for {
		idx := strings.LastIndex(line, "{")
		if idx < 0 {
			break
		}
		closeIdx := strings.Index(line[idx:], "}")
		if closeIdx < 0 {
			break
		}
		numStr := line[idx+1 : idx+closeIdx]
		n, err := strconv.Atoi(numStr)
		if err != nil {
			break
		}
		// Check if the {NNN} is at the end of the line (before \r\n)
		afterBrace := idx + closeIdx + 1
		remainder := strings.TrimRight(line[afterBrace:], "\r\n")
		if remainder != "" {
			break // {NNN} is not at end of line, not a literal
		}

		// Read the literal bytes
		literal := make([]byte, n)
		if _, err := io.ReadFull(br, literal); err != nil {
			return "", err
		}
		line = line[:afterBrace] + string(literal)

		// Continue reading the rest of the line after the literal
		rest, err := br.ReadString('\n')
		if err != nil {
			return "", err
		}
		line += rest
	}

	return line, nil
}

// parseGmailLabelsLine parses a single untagged FETCH response line and
// extracts the UID and X-GM-LABELS values.
// Example line: * 123 FETCH (UID 456 X-GM-LABELS (\Inbox "My Label"))
func parseGmailLabelsLine(line string) (uint32, []string, bool) {
	// Find "UID " followed by a number
	uidIdx := strings.Index(line, "UID ")
	if uidIdx < 0 {
		return 0, nil, false
	}
	uidStart := uidIdx + 4
	uidEnd := uidStart
	for uidEnd < len(line) && line[uidEnd] >= '0' && line[uidEnd] <= '9' {
		uidEnd++
	}
	if uidEnd == uidStart {
		return 0, nil, false
	}
	uid, err := strconv.ParseUint(line[uidStart:uidEnd], 10, 32)
	if err != nil {
		return 0, nil, false
	}

	// Find X-GM-LABELS
	labelsIdx := strings.Index(line, "X-GM-LABELS")
	if labelsIdx < 0 {
		return uint32(uid), nil, true
	}

	// Find the opening paren after X-GM-LABELS
	parenStart := strings.Index(line[labelsIdx:], "(")
	if parenStart < 0 {
		return uint32(uid), nil, true
	}
	parenStart += labelsIdx

	// Find the matching closing paren (labels can contain parens inside quotes)
	depth := 0
	parenEnd := -1
	inQuote := false
	for i := parenStart; i < len(line); i++ {
		ch := line[i]
		if inQuote {
			if ch == '\\' && i+1 < len(line) {
				i++ // skip escaped char
				continue
			}
			if ch == '"' {
				inQuote = false
			}
			continue
		}
		if ch == '"' {
			inQuote = true
		} else if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				parenEnd = i
				break
			}
		}
	}
	if parenEnd < 0 {
		return uint32(uid), nil, true
	}

	labelsContent := line[parenStart+1 : parenEnd]
	labels := parseLabelList(labelsContent)
	return uint32(uid), labels, true
}

// parseLabelList parses the space-separated list of labels inside the
// X-GM-LABELS parentheses. Labels can be quoted strings or atoms (system
// labels like \Inbox).
func parseLabelList(s string) []string {
	var labels []string
	i := 0
	for i < len(s) {
		// Skip whitespace
		for i < len(s) && (s[i] == ' ' || s[i] == '\r' || s[i] == '\n') {
			i++
		}
		if i >= len(s) {
			break
		}

		if s[i] == '"' {
			// Quoted string
			i++
			start := i
			for i < len(s) {
				if s[i] == '\\' && i+1 < len(s) {
					i += 2
					continue
				}
				if s[i] == '"' {
					break
				}
				i++
			}
			labels = append(labels, s[start:i])
			if i < len(s) {
				i++ // skip closing quote
			}
		} else {
			// Atom (e.g. \Inbox, \Sent) — strip leading backslash
			start := i
			if s[i] == '\\' {
				i++
			}
			atomStart := i
			for i < len(s) && s[i] != ' ' && s[i] != '\r' && s[i] != '\n' {
				i++
			}
			_ = start
			labels = append(labels, s[atomStart:i])
		}
	}
	return labels
}
