package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/i18n"
	_ "github.com/floatpane/matcha/i18n/languages"
)

// resolveAccount centralizes the logic for loading the config and selecting an account.
// It returns the loaded config and the selected account, or an error.
func resolveAccount(fromEmail string) (*config.Config, *config.Account, error) {
	cfg, err := configLoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("could not load config: %w", err)
	}

	// Initialize i18n if not already initialized, set language based on configuration/env
	// GetManager() auto-initializes with English if not already initialized
	if i18n.GetManager() == nil {
		if err := i18n.Init("en"); err != nil {
			fmt.Fprintf(ErrOut, "warning: failed to initialize i18n: %v\n", err)
		}
	}

	if manager := i18n.GetManager(); manager != nil {
		lang := i18n.DetectLanguage(cfg)
		if err := manager.SetLanguage(lang); err != nil {
			fmt.Fprintf(ErrOut, "warning: failed to set i18n language: %v\n", err)
		}
	}

	if !cfg.HasAccounts() {
		return nil, nil, fmt.Errorf("no accounts configured")
	}

	var account *config.Account
	if fromEmail != "" {
		// Try matching against login Email case-insensitively first
		for i := range cfg.Accounts {
			if strings.EqualFold(cfg.Accounts[i].Email, fromEmail) {
				account = &cfg.Accounts[i]
				break
			}
		}
		if account == nil {
			// Try matching against FetchEmail as a fallback case-insensitively
			for i := range cfg.Accounts {
				if strings.EqualFold(cfg.Accounts[i].FetchEmail, fromEmail) {
					account = &cfg.Accounts[i]
					break
				}
			}
		}
		if account == nil {
			return nil, nil, fmt.Errorf("no account found matching %q", fromEmail)
		}
	} else {
		account = cfg.GetFirstAccount()
	}

	return cfg, account, nil
}

// printEmails centralizes the logic for rendering a list of emails.
func printEmails(emails []backend.Email, jsonOutput bool) error {
	if jsonOutput {
		jsonEmails := make([]ListJSON, len(emails))
		for i, e := range emails {
			jsonEmails[i] = ListJSON{
				UID:     e.UID,
				From:    e.From,
				Subject: e.Subject,
				Date:    e.Date,
				IsRead:  e.IsRead,
			}
		}
		encoder := json.NewEncoder(Out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(jsonEmails)
	}

	// Text output
	w := tabwriter.NewWriter(Out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "UID\tFROM\tSUBJECT\tDATE\tREAD")
	for _, email := range emails {
		readStatus := " "
		if email.IsRead {
			readStatus = "✔"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n",
			email.UID,
			sanitizeTextForTable(email.From),
			sanitizeTextForTable(email.Subject),
			email.Date.Format("2006-01-02 15:04"),
			readStatus,
		)
	}
	return w.Flush()
}

// NormalizeFolder normalizes a folder name, mapping empty or case-insensitive "inbox" to "INBOX".
func NormalizeFolder(folder string) string {
	if folder == "" || strings.EqualFold(folder, "inbox") {
		return "INBOX"
	}
	return folder
}

// parseInterspersed parses a flag set with interspersed positional arguments.
// It returns the collected positional arguments in the order they were encountered.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	argsToParse := args
	for {
		if err := fs.Parse(argsToParse); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			break
		}
		positionals = append(positionals, fs.Arg(0))
		argsToParse = fs.Args()[1:]
	}
	return positionals, nil
}

// sanitizeTextForTable replaces tabs and newlines with spaces to avoid breaking tabwriter formatting.
func sanitizeTextForTable(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return s
}
