package cli

import (
	"flag"
	"fmt"
	"time"
)

// ListJSON defines the structure for a single email in the JSON output.
type ListJSON struct {
	UID     uint32    `json:"uid"`
	From    string    `json:"from"`
	Subject string    `json:"subject"`
	Date    time.Time `json:"date"`
	IsRead  bool      `json:"is_read"`
}

// RunList implements the "list" subcommand.
func RunList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	fs.SetOutput(ErrOut)
	from := fs.String("from", "", "Email address of the account to use")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")
	fs.Usage = func() {
		fmt.Fprintln(ErrOut, "Usage: matcha list [folder] [--from <email>] [--json]")
		fs.PrintDefaults()
	}
	positionals, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}

	cfg, account, err := resolveAccount(*from)
	if err != nil {
		return err
	}

	folder := "INBOX"
	if len(positionals) > 0 {
		folder = positionals[0]
	}
	folder = NormalizeFolder(folder)

	// Per design constraints, autoStart is always false for CLI commands.
	svc := NewServiceFunc(cfg, false)
	defer svc.Close()

	// Using a hardcoded limit of 50 as per the spec.
	emails, err := svc.FetchEmails(account.ID, folder, 50, 0)
	if err != nil {
		return fmt.Errorf("could not fetch emails: %w", err)
	}

	return printEmails(emails, *jsonOutput)
}
