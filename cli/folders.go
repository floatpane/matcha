package cli

import (
	"encoding/json"
	"flag"
	"fmt"
)

// FolderJSON represents the output schema for --json mode.
type FolderJSON struct {
	Name string `json:"name"`
}

// RunFolders implements the non-interactive CLI subcommand `matcha folders`.
func RunFolders(args []string) error {
	fs := flag.NewFlagSet("folders", flag.ExitOnError)
	fs.SetOutput(ErrOut)

	from := fs.String("from", "", "Sender account email (defaults to first configured account)")
	jsonOut := fs.Bool("json", false, "Output in JSON format")

	fs.Usage = func() {
		fmt.Fprintln(ErrOut, "Usage: matcha folders [flags]")
		fmt.Fprintln(ErrOut, "")
		fmt.Fprintln(ErrOut, "List all folders for a configured email account.")
		fmt.Fprintln(ErrOut, "")
		fmt.Fprintln(ErrOut, "Flags:")
		fs.PrintDefaults()
	}

	fs.Parse(args)

	cfg, account, err := resolveAccount(*from)
	if err != nil {
		return err
	}

	// Instantiate the client with autoStart = false
	svc := NewServiceFunc(cfg, false)
	defer svc.Close()

	folders, err := svc.FetchFolders(account.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch folders: %w", err)
	}

	if *jsonOut {
		output := []FolderJSON{}
		for _, f := range folders {
			output = append(output, FolderJSON{Name: f.Name})
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("json marshal: %w", err)
		}
		fmt.Fprintln(Out, string(data))
	} else {
		for _, f := range folders {
			fmt.Fprintln(Out, f.Name)
		}
	}

	return nil
}
