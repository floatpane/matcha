package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
)

// FolderJSON represents the output schema for --json mode.
type FolderJSON struct {
	Name string `json:"name"`
}

// RunFolders implements the non-interactive CLI subcommand `matcha folders`.
func RunFolders(args []string) error {
	fs := flag.NewFlagSet("folders", flag.ContinueOnError)
	fs.SetOutput(ErrOut)

	from := fs.String("from", "", "Sender account email (defaults to first configured account)")
	jsonOut := fs.Bool("json", false, "Output in JSON format")

	fs.Usage = func() {
		fprintln(ErrOut, "Usage: matcha folders [flags]")
		fprintln(ErrOut, "")
		fprintln(ErrOut, "List all folders for a configured email account.")
		fprintln(ErrOut, "")
		fprintln(ErrOut, "Flags:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	cfg, account, err := resolveAccount(*from)
	if err != nil {
		return err
	}

	// Instantiate the client with autoStart = false
	svc := NewServiceFunc(cfg, false)
	defer func() { _ = svc.Close() }()

	folders, err := svc.FetchFolders(account.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch folders: %w", err)
	}

	if *jsonOut {
		output := make([]FolderJSON, len(folders))
		for i, f := range folders {
			output[i] = FolderJSON{Name: f.Name}
		}
		encoder := json.NewEncoder(Out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}
	for _, f := range folders {
		fprintln(Out, f.Name)
	}

	return nil
}
