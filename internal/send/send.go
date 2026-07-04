package send

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"net/mail"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	calendar "github.com/floatpane/go-icalendar"
	mailpatch "github.com/floatpane/go-mailpatch"
	patchapply "github.com/floatpane/go-patchapply"
	"github.com/floatpane/matcha/clib"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/gitmail"
	"github.com/floatpane/matcha/sender"
	"github.com/floatpane/matcha/tui"
	"github.com/google/uuid"
)

// Dependencies groups the runtime services and configuration needed by
// the TUI send paths so callers can pass them explicitly.
type Dependencies struct {
	Service daemonclient.Service
	Config  *config.Config
}

// StringSliceFlag implements flag.Value to allow repeated flags such as --attach.
type StringSliceFlag []string

func (s *StringSliceFlag) String() string { return strings.Join(*s, ", ") }
func (s *StringSliceFlag) Set(val string) error {
	*s = append(*s, val)
	return nil
}

// ParseEmailAddress parses "Name <email>" or just "email" format.
func ParseEmailAddress(addr string) (name, email string) {
	addr = strings.TrimSpace(addr)
	if idx := strings.Index(addr, "<"); idx != -1 {
		name = strings.TrimSpace(addr[:idx])
		endIdx := strings.Index(addr, ">")
		if endIdx > idx {
			email = strings.TrimSpace(addr[idx+1 : endIdx])
		} else {
			email = strings.TrimSpace(addr[idx+1:])
		}
	} else {
		email = addr
	}
	return name, email
}

// SplitEmails splits a comma-separated list of addresses and trims whitespace.
func SplitEmails(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var res []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

// MarkdownToHTML converts Markdown to HTML using the existing clib helper.
func MarkdownToHTML(md []byte) []byte {
	return clib.MarkdownToHTML(md)
}

// IsFlagSet returns true if the named flag was explicitly provided on the command line.
func IsFlagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// BuildSendBody composes the final plain text body from the message body,
// optional signature, and optional quoted reply text.
func BuildSendBody(body, signature, quotedText string) string {
	if signature != "" {
		body = body + "\n\n" + signature
	}
	if quotedText != "" {
		body += quotedText
	}
	return body
}

// InlineImages is a base64-encoded inline image keyed by its Content-ID.
type InlineImages map[string][]byte

// ExtractInlineImages finds markdown image references in body, reads the files,
// replaces the references with cid: URLs, and returns the updated body and the
// images map. Errors reading individual files are logged and skipped.
func ExtractInlineImages(body string) (string, InlineImages) {
	images := make(InlineImages)
	re := regexp.MustCompile(`!\[.*?\]\((.*?)\)`)
	matches := re.FindAllStringSubmatch(body, -1)

	for _, match := range matches {
		imgPath := match[1]
		imgData, err := os.ReadFile(imgPath)
		if err != nil {
			log.Printf("Could not read image file %s: %v", imgPath, err)
			continue
		}
		cid := fmt.Sprintf("%s%s@%s", uuid.NewString(), filepath.Ext(imgPath), "matcha")
		images[cid] = []byte(base64.StdEncoding.EncodeToString(imgData))
		body = strings.Replace(body, imgPath, "cid:"+cid, 1)
	}

	return body, images
}

// SendEmailCmd returns a tea.Cmd that queues an email through the provided service.
// It applies the from override, signature, quoted text, inline images, and
// attachments, then queues the email via the daemonclient service.
func SendEmailCmd(deps *Dependencies, msg tui.SendEmailMsg) tea.Cmd {
	return func() tea.Msg {
		var account *config.Account
		if msg.AccountID != "" && deps.Config != nil {
			account = deps.Config.GetAccountByID(msg.AccountID)
		}
		if account == nil && deps.Config != nil {
			account = deps.Config.GetFirstAccount()
		}
		if account == nil {
			return tui.EmailResultMsg{Err: fmt.Errorf("no account configured")}
		}

		if msg.FromOverride != "" {
			acc := *account
			acc.SendAsEmail = msg.FromOverride
			account = &acc
		}

		recipients := SplitEmails(msg.To)
		cc := SplitEmails(msg.Cc)
		bcc := SplitEmails(msg.Bcc)

		body := BuildSendBody(msg.Body, msg.Signature, msg.QuotedText)
		body, images := ExtractInlineImages(body)
		htmlBody := MarkdownToHTML([]byte(body))

		attachments := make(map[string][]byte)
		for _, attachPath := range msg.AttachmentPaths {
			fileData, err := os.ReadFile(attachPath)
			if err != nil {
				log.Printf("Could not read attachment file %s: %v", attachPath, err)
				continue
			}
			_, filename := filepath.Split(attachPath)
			attachments[filename] = fileData
		}

		delaySeconds := deps.Config.GetUndoDelaySeconds()

		var prebuiltRaw []byte
		if msg.SignPGP || msg.EncryptPGP {
			var buildErr error
			prebuiltRaw, buildErr = sender.BuildEmail(account, recipients, cc, bcc, msg.Subject, body, string(htmlBody), images, attachments, msg.InReplyTo, msg.References, msg.SignSMIME, msg.EncryptSMIME, msg.SignPGP, msg.EncryptPGP)
			if buildErr != nil {
				log.Printf("Failed to build PGP email: %v", buildErr)
				return tui.EmailResultMsg{Err: buildErr}
			}
		}

		jobID, err := deps.Service.QueueEmail(account.ID, recipients, cc, bcc, msg.Subject, body, string(htmlBody), images, attachments, msg.InReplyTo, msg.References, msg.SignSMIME, msg.EncryptSMIME, false, false, delaySeconds, prebuiltRaw)
		if err != nil {
			log.Printf("Failed to queue email: %v", err)
			return tui.EmailResultMsg{Err: err}
		}

		return tui.EmailQueuedMsg{JobID: jobID, DelaySeconds: delaySeconds}
	}
}

// SendRSVP returns a tea.Cmd that sends a calendar RSVP reply for the given account.
func SendRSVP(account *config.Account, msg tui.SendRSVPMsg) tea.Cmd {
	return func() tea.Msg {
		if account == nil {
			return tui.EmailResultMsg{Err: fmt.Errorf("no account configured")}
		}

		rsvpICS, err := calendar.GenerateRSVP(msg.OriginalICS, account.Email, msg.Response)
		if err != nil {
			return tui.EmailResultMsg{Err: fmt.Errorf("generate RSVP: %w", err)}
		}

		subject := fmt.Sprintf("Re: %s", msg.Event.Summary)
		bodyText := fmt.Sprintf("%s: %s\n\n%s",
			msg.Response,
			msg.Event.Summary,
			msg.Event.Start.Local().Format("Mon Jan 2, 2006 3:04 PM"))
		if msg.Event.Location != "" {
			bodyText += " at " + msg.Event.Location
		}

		references := append(msg.References, msg.InReplyTo) //nolint:gocritic
		rawMsg, err := sender.SendCalendarReply(
			account,
			[]string{msg.Event.Organizer},
			subject,
			bodyText,
			rsvpICS,
			msg.InReplyTo,
			references,
		)

		if err != nil {
			return tui.RSVPResultMsg{Err: fmt.Errorf("send RSVP: %w", err), Response: msg.Response, Organizer: msg.Event.Organizer}
		}

		if account.ServiceProvider != "gmail" {
			if err := fetcher.AppendToSentMailbox(account, rawMsg); err != nil {
				log.Printf("Failed to append RSVP to Sent folder: %v", err)
			}
		}

		return tui.RSVPResultMsg{Response: msg.Response, Organizer: msg.Event.Organizer}
	}
}

// ResolveAccount selects an account from config based on the CLI --from flag.
func ResolveAccount(cfg *config.Config, from string) *config.Account {
	if from == "" {
		return cfg.GetFirstAccount()
	}
	account := cfg.GetAccountByEmail(from)
	if account == nil {
		for i := range cfg.Accounts {
			if strings.EqualFold(cfg.Accounts[i].FetchEmail, from) {
				return &cfg.Accounts[i]
			}
		}
	}
	return account
}

// RunSendCLI implements the CLI entrypoint for `matcha send`.
// It sends an email non-interactively using a configured account.
func RunSendCLI(args []string, exitFn func(int)) {
	fs := flag.NewFlagSet("send", flag.ExitOnError)

	to := fs.String("to", "", "Recipient(s), comma-separated (required)")
	cc := fs.String("cc", "", "CC recipient(s), comma-separated")
	bcc := fs.String("bcc", "", "BCC recipient(s), comma-separated")
	subject := fs.String("subject", "", "Email subject (required)")
	body := fs.String("body", "", `Email body (Markdown supported). Use "-" to read from stdin`)
	from := fs.String("from", "", "Sender account email (defaults to first configured account)")
	withSignature := fs.Bool("signature", true, "Append default signature")
	signSMIME := fs.Bool("sign-smime", false, "Sign with S/MIME")
	encryptSMIME := fs.Bool("encrypt-smime", false, "Encrypt with S/MIME")
	signPGP := fs.Bool("sign-pgp", false, "Sign with PGP")

	var attachments StringSliceFlag
	fs.Var(&attachments, "attach", "Attachment file path (can be repeated)")

	fs.Usage = func() {
		log.Println("Usage: matcha send [flags]")
		log.Println()
		log.Println("Send an email non-interactively using a configured account.")
		log.Println()
		log.Println("Flags:")
		fs.PrintDefaults()
		log.Println()
		log.Println("Examples:")
		log.Println(`  matcha send --to user@example.com --subject "Hello" --body "Hi there"`)
		log.Println(`  echo "Body text" | matcha send --to user@example.com --subject "Hello" --body -`)
		log.Println(`  matcha send --to user@example.com --subject "Report" --body "See attached" --attach report.pdf`)
	}

	if err := fs.Parse(args); err != nil {
		exitFn(1)
		return
	}

	if *to == "" || *subject == "" {
		log.Println("Error: --to and --subject are required")
		fs.Usage()
		exitFn(1)
		return
	}

	emailBody := *body
	if emailBody == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Printf("Error reading stdin: %v", err)
			exitFn(1)
			return
		}
		emailBody = string(data)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Error loading config: %v", err)
		exitFn(1)
		return
	}
	if !cfg.HasAccounts() {
		log.Println("Error: no accounts configured. Run matcha to set up an account first.")
		exitFn(1)
		return
	}

	account := ResolveAccount(cfg, *from)
	if account == nil {
		log.Printf("Error: no account found matching %q", *from)
		exitFn(1)
		return
	}

	if !IsFlagSet(fs, "sign-smime") {
		*signSMIME = account.SMIMESignByDefault
	}
	if !IsFlagSet(fs, "sign-pgp") {
		*signPGP = account.PGPSignByDefault
	}

	if *withSignature {
		if sig, err := config.LoadSignature(); err == nil && sig != "" {
			emailBody = emailBody + "\n\n" + sig
		}
	}

	emailBody, images := ExtractInlineImages(emailBody)
	htmlBody := MarkdownToHTML([]byte(emailBody))

	attachMap := make(map[string][]byte)
	for _, attachPath := range attachments {
		fileData, err := os.ReadFile(attachPath)
		if err != nil {
			log.Printf("Error reading attachment %s: %v", attachPath, err)
			exitFn(1)
			return
		}
		attachMap[filepath.Base(attachPath)] = fileData
	}

	recipients := SplitEmails(*to)
	ccList := SplitEmails(*cc)
	bccList := SplitEmails(*bcc)

	rawMsg, sendErr := sender.SendEmail(account, recipients, ccList, bccList, *subject, emailBody, string(htmlBody), images, attachMap, "", nil, *signSMIME, *encryptSMIME, *signPGP, false)
	if sendErr != nil {
		log.Printf("Error: %v", sendErr)
		exitFn(1)
		return
	}

	if account.ServiceProvider != "gmail" {
		if err := fetcher.AppendToSentMailbox(account, rawMsg); err != nil {
			log.Printf("Failed to append sent message to Sent folder: %v", err)
		}
	}

	log.Println("Email sent successfully.")
}

// ApplyPatchCmd returns a tea.Cmd that applies a patch email to a local repo.
func ApplyPatchCmd(repoDir string, msg tui.ApplyPatchMsg) tea.Cmd {
	return func() tea.Msg {
		// Strip \r from \r\n line endings before splitting. The mailpatch
		// parser's "---" separator detection fails on \r\n, causing the
		// diffstat block to leak into the commit message.
		cleanBody := strings.ReplaceAll(msg.RawEmail, "\r", "")
		commitMsg, diff := mailpatch.SplitBodyDiff(cleanBody)
		if diff == "" {
			return tui.PatchApplyResultMsg{Subject: msg.Subject, Err: fmt.Errorf("no diff found in patch email")}
		}

		files, err := mailpatch.ParseDiff(diff)
		if err != nil {
			return tui.PatchApplyResultMsg{Subject: msg.Subject, Err: fmt.Errorf("parse diff: %w", err)}
		}

		fsys := patchapply.NewDirFS(repoDir)
		result, err := patchapply.Apply(fsys, files, nil)
		if err != nil {
			return tui.PatchApplyResultMsg{Subject: msg.Subject, Err: fmt.Errorf("apply: %w", err)}
		}

		var fileNames []string
		if result != nil {
			for _, f := range result.Files {
				fileNames = append(fileNames, f.Path)
			}
		}

		// Stage all changes (new, modified, deleted files).
		add := exec.CommandContext(context.Background(), "git", "-C", repoDir, "add", "-A")
		if err := add.Run(); err != nil {
			return tui.PatchApplyResultMsg{
				Subject:  msg.Subject,
				Files:    fileNames,
				Warnings: []string{fmt.Sprintf("patch applied but git add failed: %v", err)},
			}
		}

		// Check whether there is anything staged to commit.
		status := exec.CommandContext(context.Background(), "git", "-C", repoDir, "status", "--porcelain", "--untracked-files=no")
		statusOut, err := status.Output()
		if err != nil {
			return tui.PatchApplyResultMsg{
				Subject:  msg.Subject,
				Files:    fileNames,
				Warnings: []string{fmt.Sprintf("patch applied but git status failed: %v", err)},
			}
		}
		if len(strings.TrimSpace(string(statusOut))) == 0 {
			return tui.PatchApplyResultMsg{Subject: msg.Subject, Files: fileNames}
		}

		return tui.PatchStagedMsg{
			Subject:   msg.Subject,
			From:      msg.From,
			CommitMsg: commitMsg,
			Files:     fileNames,
		}
	}
}

// CommitPatchCmd returns a tea.Cmd that runs git commit via tea.ExecProcess.
// This releases the terminal so that GPG pinentry (curses or GUI) can run
// cleanly, then restores the TUI when the commit finishes.
func CommitPatchCmd(repoDir string, msg tui.PatchStagedMsg) tea.Cmd {
	authorName, authorEmail := parseAuthor(msg.From)
	message := buildCommitMessage(msg.Subject, msg.CommitMsg)

	commit := exec.CommandContext(context.Background(), "git", "-C", repoDir, "commit", "-m", message)
	if authorName != "" {
		commit.Env = appendEnv(commit.Env, "GIT_AUTHOR_NAME", authorName)
		commit.Env = appendEnv(commit.Env, "GIT_COMMITTER_NAME", authorName)
	}
	if authorEmail != "" {
		commit.Env = appendEnv(commit.Env, "GIT_AUTHOR_EMAIL", authorEmail)
		commit.Env = appendEnv(commit.Env, "GIT_COMMITTER_EMAIL", authorEmail)
	}

	return tea.ExecProcess(commit, func(err error) tea.Msg {
		if err != nil {
			return tui.PatchApplyResultMsg{
				Subject:  msg.Subject,
				Files:    msg.Files,
				Warnings: []string{fmt.Sprintf("patch applied but git commit failed: %v", err)},
			}
		}
		return tui.PatchApplyResultMsg{Subject: msg.Subject, Files: msg.Files}
	})
}

// appendEnv sets key=value in the environment list, replacing any existing
// entry. If env is nil, os.Environ() is used as the base.
func appendEnv(env []string, key, value string) []string {
	if env == nil {
		env = os.Environ()
	}
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

// stripPatchPrefix removes the [PATCH ...] or [RFC PATCH ...] prefix from a
// subject line, returning the clean subject.
func stripPatchPrefix(subject string) string {
	subject = strings.TrimSpace(subject)
	if !strings.HasPrefix(subject, "[") {
		return subject
	}
	end := strings.Index(subject, "]")
	if end < 0 {
		return subject
	}
	inner := subject[1:end]
	for _, t := range strings.Fields(inner) {
		if t == "PATCH" || t == "RFC" {
			return strings.TrimSpace(subject[end+1:])
		}
	}
	return subject
}

// buildCommitMessage constructs the full git commit message from the patch
// email. The first line is the clean subject (without [PATCH...] prefix),
// followed by a blank line and the original commit message body (which
// includes the description and trailers such as Signed-off-by,
// Co-developed-by, etc.).
func buildCommitMessage(subject, commitMsg string) string {
	cleanSubject := stripPatchPrefix(subject)
	commitMsg = strings.TrimSpace(commitMsg)
	if commitMsg == "" {
		return cleanSubject
	}
	return cleanSubject + "\n\n" + commitMsg
}

// parseAuthor extracts a name and email from a "From" header value.
// e.g. "John Doe <john@example.com>" -> ("John Doe", "john@example.com")
func parseAuthor(from string) (name, email string) {
	from = strings.TrimSpace(from)
	if from == "" {
		return "", ""
	}
	// Try standard RFC 5322 format: Name <email>
	if addr, err := mail.ParseAddress(from); err == nil {
		return addr.Name, addr.Address
	}
	// Fallback: bare email address
	if strings.Contains(from, "@") && !strings.ContainsAny(from, "<>") {
		return "", from
	}
	return from, ""
}

// SendPatchCmd returns a tea.Cmd that generates a patch from a local repo
// and sends it via email.
func SendPatchCmd(deps *Dependencies, msg tui.SendPatchMsg) tea.Cmd {
	return func() tea.Msg {
		rawPatch, err := gitmail.GeneratePatch(msg.RepoDir, msg.CommitRange)
		if err != nil {
			return tui.PatchGeneratedMsg{SendPatchMsg: msg, Err: err}
		}
		return tui.PatchGeneratedMsg{SendPatchMsg: msg, RawPatch: rawPatch}
	}
}

// SendRawPatchCmd returns a tea.Cmd that sends a raw format-patch email
// (already generated by git format-patch) via the configured SMTP account.
func SendRawPatchCmd(deps *Dependencies, msg tui.SendPatchMsg, rawPatch []byte) tea.Cmd {
	return func() tea.Msg {
		if deps == nil || deps.Config == nil {
			return tui.EmailResultMsg{Err: fmt.Errorf("no config available")}
		}
		account := deps.Config.GetFirstAccount()
		if account == nil {
			return tui.EmailResultMsg{Err: fmt.Errorf("no account configured")}
		}

		// Rewrite the From header to use the configured account's sending
		// identity. SMTP servers reject messages whose From address doesn't
		// match the authenticated account. The original author is preserved
		// in the patch body by git format-patch's "From: " line.
		fromAddr := account.SendAsEmail
		if fromAddr == "" {
			fromAddr = account.FetchEmail
		}
		if fromAddr == "" {
			fromAddr = account.Email
		}
		fromName := account.Name
		rawPatch = rewriteFromHeader(rawPatch, fromName, fromAddr)

		// Inject the To and Cc headers from the TUI form into the raw message so
		// the delivered and sent copies actually contain the recipients. git
		// format-patch --stdout does not emit To or Cc headers by default.
		rawPatch = rewriteToHeader(rawPatch, msg.To)
		rawPatch = rewriteCcHeader(rawPatch, msg.Cc)

		// Parse the patch email to extract recipients for the SMTP envelope.
		p, err := gitmail.ParsePatch(rawPatch)
		if err != nil {
			return tui.EmailResultMsg{Err: fmt.Errorf("failed to parse generated patch: %w", err)}
		}

		// Collect all recipients from the To and Cc headers.
		toAddrs := SplitEmails(p.Header.Get("To"))
		ccAddrs := SplitEmails(p.Header.Get("Cc"))
		recipients := make([]string, 0, len(toAddrs)+len(ccAddrs))
		recipients = append(recipients, toAddrs...)
		recipients = append(recipients, ccAddrs...)
		if len(recipients) == 0 {
			return tui.EmailResultMsg{Err: fmt.Errorf("no recipients found in patch email")}
		}

		// Deliver the pre-built raw patch email via SMTP.
		if err := sender.DeliverRaw(account, recipients, rawPatch); err != nil {
			return tui.EmailResultMsg{Err: fmt.Errorf("failed to send patch: %w", err)}
		}

		// Append to sent folder. For Gmail this is skipped because Gmail
		// automatically saves a copy of messages sent through its SMTP servers.
		if account.ServiceProvider != "gmail" {
			if err := fetcher.AppendToSentMailbox(account, rawPatch); err != nil {
				return tui.EmailResultMsg{Warning: fmt.Sprintf("Sent, but could not copy to Sent folder: %v", err)}
			}
		}

		return tui.EmailResultMsg{}
	}
}

// rewriteFromHeader replaces the From header in a raw RFC 5322 message with
// the given name and email address. It handles both \r\n and \n line endings.
func rewriteFromHeader(raw []byte, name, email string) []byte {
	var newFrom string
	if name != "" {
		newFrom = "From: " + name + " <" + email + ">"
	} else {
		newFrom = "From: " + email
	}

	// Try \r\n first (standard RFC 5322), then fall back to \n.
	for _, sep := range []string{"\r\n", "\n"} {
		lines := strings.Split(string(raw), sep)
		for i, line := range lines {
			if strings.HasPrefix(strings.ToLower(line), "from:") {
				lines[i] = newFrom
				return []byte(strings.Join(lines, sep))
			}
		}
	}
	return raw
}

// rewriteToHeader replaces or inserts a To header in a raw RFC 5322 message.
func rewriteToHeader(raw []byte, to string) []byte {
	if to == "" {
		return raw
	}
	return rewriteHeader(raw, "To", "To: "+to)
}

// rewriteCcHeader replaces or inserts a Cc header in a raw RFC 5322 message.
func rewriteCcHeader(raw []byte, cc string) []byte {
	if cc == "" {
		return raw
	}
	return rewriteHeader(raw, "Cc", "Cc: "+cc)
}

// rewriteHeader replaces the first header matching name (case-insensitive) in
// raw, or inserts it after the From header if it does not exist. Preserves the
// line ending style (\r\n or \n) used in the input.
func rewriteHeader(raw []byte, name, value string) []byte {
	lowerName := strings.ToLower(name)
	s := string(raw)
	sep := "\n"
	if strings.Contains(s, "\r\n") {
		sep = "\r\n"
	}
	lines := strings.Split(s, sep)

	found := false
	for i, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), lowerName+":") {
			lines[i] = value
			found = true
			break
		}
	}
	if !found {
		// Insert after the From header so the new header stays in the
		// message header block.
		for i, line := range lines {
			if strings.HasPrefix(strings.ToLower(line), "from:") {
				lines = append(lines[:i+1], append([]string{value}, lines[i+1:]...)...)
				break
			}
		}
	}
	return []byte(strings.Join(lines, sep))
}
