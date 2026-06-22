package send

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	calendar "github.com/floatpane/go-icalendar"
	"github.com/floatpane/matcha/clib"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
	"github.com/floatpane/matcha/fetcher"
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
