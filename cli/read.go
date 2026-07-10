package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"strconv"

	"github.com/floatpane/matcha/backend"
)

// ReadJSON defines the structure for the --json output of the read command.
type ReadJSON struct {
	Body        string               `json:"body"`
	MIMEType    string               `json:"mime_type"`
	Attachments []backend.Attachment `json:"attachments,omitempty"`
}

// RunRead implements the "read" subcommand.
func RunRead(args []string) error {
	fs := flag.NewFlagSet("read", flag.ExitOnError)
	fs.SetOutput(ErrOut)
	from := fs.String("from", "", "Email address of the account to use")
	jsonOutput := fs.Bool("json", false, "Output in JSON format")
	fs.Usage = func() {
		fmt.Fprintln(ErrOut, "Usage: matcha read [--from <email>] [--json] <folder> <uid>")
		fs.PrintDefaults()
	}
	positionals, err := parseInterspersed(fs, args)
	if err != nil {
		return err
	}

	var folder string
	var uidStr string

	switch len(positionals) {
	case 1:
		// A single positional is treated as a UID in INBOX, but only if it
		// parses as a number; otherwise the folder was given without a UID.
		if _, err := strconv.ParseUint(positionals[0], 10, 32); err == nil {
			folder = "INBOX"
			uidStr = positionals[0]
		} else {
			return fmt.Errorf("folder and uid are required")
		}
	case 0:
		return fmt.Errorf("folder and uid are required")
	default:
		folder = positionals[0]
		uidStr = positionals[1]
	}

	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil {
		return fmt.Errorf("invalid uid %q: %w", uidStr, err)
	}
	uid := uint32(uid64)

	cfg, account, err := resolveAccount(*from)
	if err != nil {
		return err
	}
	folder = NormalizeFolder(folder)

	svc := NewServiceFunc(cfg, false)
	defer svc.Close()

	body, mime, attachments, err := svc.FetchEmailBody(account.ID, folder, uid)
	if err != nil {
		return fmt.Errorf("could not fetch email body: %w", err)
	}

	if *jsonOutput {
		output := ReadJSON{
			Body:        body,
			MIMEType:    mime,
			Attachments: attachments,
		}
		encoder := json.NewEncoder(Out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}

	fmt.Fprint(Out, body)
	return nil
}
