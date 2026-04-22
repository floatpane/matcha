package cli

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

	"github.com/floatpane/matcha/clib"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/sender"
	"github.com/google/uuid"
)

// StringSliceFlag implements flag.Value to allow repeated flags.
type StringSliceFlag []string

func (s *StringSliceFlag) String() string { return strings.Join(*s, ", ") }

// Set appends a value to the slice.
func (s *StringSliceFlag) Set(val string) error {
	*s = append(*s, val)
	return nil
}

// RunSend implements the CLI entrypoint for `matcha send`.
func RunSend(args []string) {
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
		fmt.Fprintln(os.Stderr, "Usage: matcha send [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Send an email non-interactively using a configured account.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, `  matcha send --to user@example.com --subject "Hello" --body "Hi there"`)
		fmt.Fprintln(os.Stderr, `  echo "Body text" | matcha send --to user@example.com --subject "Hello" --body -`)
		fmt.Fprintln(os.Stderr, `  matcha send --to user@example.com --subject "Report" --body "See attached" --attach report.pdf`)
	}

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *to == "" || *subject == "" {
		fmt.Fprintln(os.Stderr, "Error: --to and --subject are required")
		fs.Usage()
		os.Exit(1)
	}

	// Read body from stdin if "-"
	emailBody := *body
	if emailBody == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		emailBody = string(data)
	}

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}
	if !cfg.HasAccounts() {
		fmt.Fprintln(os.Stderr, "Error: no accounts configured. Run matcha to set up an account first.")
		os.Exit(1)
	}

	// Resolve account
	var account *config.Account
	if *from != "" {
		account = cfg.GetAccountByEmail(*from)
		if account == nil {
			for i := range cfg.Accounts {
				if strings.EqualFold(cfg.Accounts[i].FetchEmail, *from) {
					account = &cfg.Accounts[i]
					break
				}
			}
		}
		if account == nil {
			fmt.Fprintf(os.Stderr, "Error: no account found matching %q\n", *from)
			os.Exit(1)
		}
	} else {
		account = cfg.GetFirstAccount()
	}

	// Use account S/MIME/PGP defaults unless explicitly set
	if !isFlagSet(fs, "sign-smime") {
		*signSMIME = account.SMIMESignByDefault
	}
	if !isFlagSet(fs, "sign-pgp") {
		*signPGP = account.PGPSignByDefault
	}

	// Append signature
	if *withSignature {
		if sig, err := config.LoadSignature(); err == nil && sig != "" {
			emailBody = emailBody + "\n\n" + sig
		}
	}

	// Process inline images
	images := make(map[string][]byte)
	re := regexp.MustCompile(`!\[.*?\]\((.*?)\)`)
	matches := re.FindAllStringSubmatch(emailBody, -1)
	for _, match := range matches {
		imgPath := match[1]
		imgData, err := os.ReadFile(imgPath)
		if err != nil {
			log.Printf("Could not read image file %s: %v", imgPath, err)
			continue
		}
		cid := fmt.Sprintf("%s%s@%s", uuid.NewString(), filepath.Ext(imgPath), "matcha")
		images[cid] = []byte(base64.StdEncoding.EncodeToString(imgData))
		emailBody = strings.Replace(emailBody, imgPath, "cid:"+cid, 1)
	}

	htmlBody := clib.MarkdownToHTML([]byte(emailBody))

	// Process attachments
	attachMap := make(map[string][]byte)
	for _, attachPath := range attachments {
		fileData, err := os.ReadFile(attachPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading attachment %s: %v\n", attachPath, err)
			os.Exit(1)
		}
		attachMap[filepath.Base(attachPath)] = fileData
	}

	// Send
	recipients := clib.SplitEmails(*to)
	ccList := clib.SplitEmails(*cc)
	bccList := clib.SplitEmails(*bcc)

	rawMsg, sendErr := sender.SendEmail(account, recipients, ccList, bccList, *subject, emailBody, string(htmlBody), images, attachMap, "", nil, *signSMIME, *encryptSMIME, *signPGP, false)
	if sendErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", sendErr)
		os.Exit(1)
	}

	// Append to Sent folder via IMAP (Gmail auto-saves, so skip it)
	if account.ServiceProvider != "gmail" {
		if err := fetcher.AppendToSentMailbox(account, rawMsg); err != nil {
			log.Printf("Failed to append sent message to Sent folder: %v", err)
		}
	}

	fmt.Println("Email sent successfully.")
}

// isFlagSet returns true if the named flag was explicitly provided on the command line.
func isFlagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
