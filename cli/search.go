package cli

import (
	"flag"
	"fmt"
	"strings"

	"github.com/floatpane/matcha/backend"
)

// RunSearch implements the "search" subcommand.
func RunSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	fs.SetOutput(ErrOut)
	from := fs.String("from", "", "Email address of the account to use")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")
	fs.Usage = func() {
		fprintln(ErrOut, "Usage: matcha search [--from <email>] [--json] <folder> <query>")
		fs.PrintDefaults()
	}
	positionals, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}

	if len(positionals) < 2 {
		return fmt.Errorf("folder and query_string are required")
	}

	folder := positionals[0]
	// Join the remaining positionals so unquoted multi-term queries are not
	// silently truncated (e.g. `matcha search INBOX from:a subject:b`).
	queryString := strings.Join(positionals[1:], " ")

	cfg, account, err := resolveAccount(*from)
	if err != nil {
		return err
	}
	folder = NormalizeFolder(folder)

	svc := NewServiceFunc(cfg, false)
	defer func() { _ = svc.Close() }()

	query := backend.ParseSearchQuery(queryString)
	emails, err := svc.Search(account.ID, folder, query)
	if err != nil {
		return fmt.Errorf("could not search emails: %w", err)
	}

	return printEmails(emails, *jsonOutput)
}
