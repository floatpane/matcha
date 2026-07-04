package cli

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/gitmail"
	"github.com/floatpane/matcha/sender"
)

// RunSendPatch handles the "matcha send-patch" subcommand: generate a patch
// from a local git repository and send it via email.
func RunSendPatch(args []string) error {
	fs := flag.NewFlagSet("send-patch", flag.ExitOnError)
	repo := fs.String("repo", ".", "path to the git repository")
	to := fs.String("to", "", "recipient email address (required)")
	cc := fs.String("cc", "", "cc recipient(s), comma-separated")
	subject := fs.String("subject", "", "override patch subject (defaults to commit subject)")
	version := fs.Int("version", 1, "patch series version (e.g. 2 for v2)")
	from := fs.String("from", "", "sender account email (defaults to first configured account)")
	rangeArg := fs.String("range", "HEAD~1..HEAD", "git commit range (e.g. HEAD~3..HEAD, origin/main..HEAD)")
	help := fs.Bool("h", false, "show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		fmt.Println("Usage: matcha send-patch [flags]")
		fmt.Println("")
		fmt.Println("Generate a patch from a local git repository and send it via email.")
		fmt.Println("Uses `git format-patch --stdout` to generate the patch, then sends")
		fmt.Println("the resulting RFC 5322 email via your configured SMTP account.")
		fmt.Println("")
		fmt.Println("Flags:")
		fs.PrintDefaults()
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  matcha send-patch --to reviewer@example.com --repo ~/src/proj --range HEAD~1..HEAD")
		fmt.Println("  matcha send-patch --to list@example.org --repo . --range origin/main..HEAD --version 2")
		return nil
	}

	if *to == "" {
		return fmt.Errorf("--to is required")
	}

	// Generate the patch
	rawPatch, err := gitmail.GeneratePatch(*repo, *rangeArg)
	if err != nil {
		return fmt.Errorf("failed to generate patch: %w", err)
	}

	// If subject override is set, we need to reformat the patch email.
	// For simplicity, we just override the Subject header in the raw bytes.
	if *subject != "" {
		rawPatch = overrideSubject(rawPatch, *subject, *version)
	}

	// Load config to get the sending account
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if !cfg.HasAccounts() {
		return fmt.Errorf("no accounts configured. Run matcha to set up an account first")
	}

	account := cfg.GetAccountByEmail(*from)
	if account == nil {
		account = cfg.GetFirstAccount()
	}
	if account == nil {
		return fmt.Errorf("no account found matching %q", *from)
	}

	// Rewrite the From header to use the configured account's sending
	// identity. SMTP servers reject messages whose From address doesn't
	// match the authenticated account.
	fromAddr := account.SendAsEmail
	if fromAddr == "" {
		fromAddr = account.FetchEmail
	}
	if fromAddr == "" {
		fromAddr = account.Email
	}
	rawPatch = rewriteFromHeader(rawPatch, account.Name, fromAddr)

	// Inject the To and Cc headers from the CLI into the raw message so the
	// delivered and sent copies actually contain the recipients. git
	// format-patch --stdout does not emit To or Cc headers by default.
	rawPatch = rewriteToHeader(rawPatch, *to)
	rawPatch = rewriteCcHeader(rawPatch, *cc)

	// Parse the patch to extract recipients
	p, err := gitmail.ParsePatch(rawPatch)
	if err != nil {
		return fmt.Errorf("failed to parse generated patch: %w", err)
	}

	// Collect all recipients (To + Cc from the patch + CLI cc override)
	var recipients []string
	recipients = append(recipients, splitEmailAddrs(p.Header.Get("To"))...)
	recipients = append(recipients, splitEmailAddrs(p.Header.Get("Cc"))...)
	if *cc != "" {
		recipients = append(recipients, splitEmailAddrs(*cc)...)
	}
	// Add the --to recipient if not already in the list
	toInList := false
	for _, r := range recipients {
		if strings.EqualFold(strings.TrimSpace(r), strings.TrimSpace(*to)) {
			toInList = true
			break
		}
	}
	if !toInList {
		recipients = append(recipients, *to)
	}

	if len(recipients) == 0 {
		return fmt.Errorf("no recipients found")
	}

	// Send via SMTP
	if err := sender.DeliverRaw(account, recipients, rawPatch); err != nil {
		return fmt.Errorf("failed to send patch: %w", err)
	}

	// Append to sent folder. For Gmail this is skipped because Gmail
	// automatically saves a copy of messages sent through its SMTP servers.
	if account.ServiceProvider != "gmail" {
		if err := fetcher.AppendToSentMailbox(account, rawPatch); err != nil {
			log.Printf("Warning: patch sent, but could not copy to Sent folder: %v", err)
		}
	}

	fmt.Println("Patch sent successfully.")
	return nil
}

// overrideSubject replaces the Subject header in a raw patch email.
func overrideSubject(raw []byte, subject string, version int) []byte {
	lines := strings.Split(string(raw), "\r\n")
	prefix := fmt.Sprintf("[PATCH v%d] ", version)
	if version <= 1 {
		prefix = "[PATCH] "
	}
	newSubject := prefix + subject
	for i, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), "subject:") {
			lines[i] = "Subject: " + newSubject
			break
		}
	}
	return []byte(strings.Join(lines, "\r\n"))
}

// splitEmailAddrs splits a comma-separated list of email addresses.
func splitEmailAddrs(s string) []string {
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

// rewriteFromHeader replaces the From header in a raw RFC 5322 message with
// the given name and email address. Handles both \r\n and \n line endings.
func rewriteFromHeader(raw []byte, name, email string) []byte {
	var newFrom string
	if name != "" {
		newFrom = "From: " + name + " <" + email + ">"
	} else {
		newFrom = "From: " + email
	}
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
