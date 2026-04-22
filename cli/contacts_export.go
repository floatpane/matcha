package cli

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/floatpane/matcha/config"
	"golang.org/x/term"
)

func RunContactsExport(args []string) error {
	fs := flag.NewFlagSet("contacts export", flag.ExitOnError)
	format := fs.String("f", "json", "output format: json or csv")
	output := fs.String("o", "", "output file path (default: stdout)")
	noHeader := fs.Bool("no-header", false, "omit CSV header row")
	help := fs.Bool("h", false, "show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		fmt.Println("Usage: matcha contacts export [flags]")
		fmt.Println("")
		fmt.Println("Export contacts from cache to JSON or CSV format.")
		fmt.Println("")
		fmt.Println("Flags:")
		fs.PrintDefaults()
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  matcha contacts export              # JSON to stdout")
		fmt.Println("  matcha contacts export -f csv       # CSV to stdout")
		fmt.Println("  matcha contacts export -o out.json  # JSON to file")
		fmt.Println("  matcha contacts export -f csv --no-header  # CSV without headers")
		return nil
	}

	formatStr := strings.ToLower(*format)
	if formatStr != "json" && formatStr != "csv" {
		return fmt.Errorf("invalid format '%s': must be 'json' or 'csv'", *format)
	}

	return runExportContacts(formatStr, *output, *noHeader)
}

func runExportContacts(format, outputPath string, noHeader bool) error {
	var contacts []config.Contact
	var err error

	if config.IsSecureModeEnabled() {
		password, err := promptForPassword()
		if err != nil {
			return fmt.Errorf("password prompt failed: %w", err)
		}

		key, err := config.VerifyPassword(password)
		if err != nil {
			return fmt.Errorf("incorrect password")
		}

		config.SetSessionKey(key)
	}

	contactsCache, err := config.LoadContactsCache()
	if err != nil {
		path, pathErr := config.GetContactsCachePath()
		if pathErr != nil {
			return fmt.Errorf("contacts cache not found")
		}
		return fmt.Errorf("contacts cache not found at %s", path)
	}

	contacts = contactsCache.Contacts

	if len(contacts) == 0 {
		fmt.Fprintln(os.Stderr, "No contacts found in cache")
		return nil
	}

	var outputData []byte
	if format == "json" {
		if noHeader {
			return fmt.Errorf("the --no-header flag is only valid with CSV format")
		}
		outputData, err = exportToJSON(contacts)
		if err != nil {
			return fmt.Errorf("failed to export to JSON: %w", err)
		}
	} else {
		outputData, err = exportToCSV(contacts, noHeader)
		if err != nil {
			return fmt.Errorf("failed to export to CSV: %w", err)
		}
	}

	if outputPath != "" {
		dir := filepath.Dir(outputPath)
		if dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
		}
		if err := os.WriteFile(outputPath, outputData, 0644); err != nil {
			return fmt.Errorf("failed to write output file: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Exported %d contacts to %s\n", len(contacts), outputPath)
	} else {
		fmt.Println(string(outputData))
	}

	return nil
}

func exportToJSON(contacts []config.Contact) ([]byte, error) {
	return json.MarshalIndent(contacts, "", "  ")
}

func exportToCSV(contacts []config.Contact, noHeader bool) ([]byte, error) {
	var buf strings.Builder
	writer := csv.NewWriter(&buf)

	if !noHeader {
		if err := writer.Write([]string{"name", "email", "last_used", "use_count"}); err != nil {
			return nil, err
		}
	}

	for _, c := range contacts {
		record := []string{
			escapeCSV(c.Name),
			escapeCSV(c.Email),
			c.LastUsed.Format("2006-01-02T15:04:05Z07:00"),
			fmt.Sprintf("%d", c.UseCount),
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}

	writer.Flush()
	return []byte(buf.String()), writer.Error()
}

func escapeCSV(s string) string {
	if strings.ContainsAny(s, `,"`) {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}

func promptForPassword() (string, error) {
	fmt.Print("Enter your password: ")
	return readPassword()
}

func readPassword() (string, error) {
	fmt.Print("(masked input) ")
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println() // newline after password
	if err != nil {
		return "", err
	}
	return string(password), nil
}
