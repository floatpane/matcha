package cli

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
)

// Management subcommand names.
const (
	cmdArchive    = "archive"
	cmdDelete     = "delete"
	cmdMarkRead   = "mark-read"
	cmdMarkUnread = "mark-unread"
	cmdMove       = "move"
)

func parseUIDs(s string) ([]uint32, error) {
	s = strings.ReplaceAll(s, ",", " ")
	parts := strings.Fields(s)
	uids := make([]uint32, 0, len(parts))
	for _, part := range parts {
		uid64, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid uid %q: %w", part, err)
		}
		uids = append(uids, uint32(uid64))
	}
	return uids, nil
}

// pluralizeEmails renders a count with the correctly pluralized noun,
// e.g. "1 email" or "3 emails".
func pluralizeEmails(n int) string {
	if n == 1 {
		return "1 email"
	}
	return fmt.Sprintf("%d emails", n)
}

// RunManage implements all management subcommands.
func RunManage(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("subcommand is required: archive, delete, mark-read, mark-unread, move")
	}
	subcommand := args[0]
	rest := args[1:]

	fs := flag.NewFlagSet(subcommand, flag.ExitOnError)
	fs.SetOutput(ErrOut)
	from := fs.String("from", "", "Email address of the account to use")
	fs.Usage = func() {
		fprintln(ErrOut, "Usage:")
		fprintln(ErrOut, "  matcha archive     [--from <email>] <folder> <uids>")
		fprintln(ErrOut, "  matcha delete      [--from <email>] <folder> <uids>")
		fprintln(ErrOut, "  matcha mark-read   [--from <email>] <folder> <uids>")
		fprintln(ErrOut, "  matcha mark-unread [--from <email>] <folder> <uids>")
		fprintln(ErrOut, "  matcha move        [--from <email>] <src_folder> <dst_folder> <uids>")
		fprintln(ErrOut, "")
		fprintln(ErrOut, "<uids> is a comma- or space-separated list of UIDs (e.g. 1,2,3).")
		fs.PrintDefaults()
	}
	positionals, err := parseInterspersed(fs, rest)
	if err != nil {
		return err
	}

	cfg, account, err := resolveAccount(*from)
	if err != nil {
		return err
	}

	svc := NewServiceFunc(cfg, false)
	defer func() { _ = svc.Close() }()

	switch subcommand {
	case cmdArchive, cmdDelete, cmdMarkRead, cmdMarkUnread:
		if len(positionals) < 2 {
			return fmt.Errorf("folder and uids are required")
		}
		folder := NormalizeFolder(positionals[0])
		uids, err := parseUIDs(positionals[1])
		if err != nil {
			return err
		}
		if len(uids) == 0 {
			return fmt.Errorf("no valid UIDs provided")
		}

		switch subcommand {
		case cmdArchive:
			err = svc.ArchiveEmails(account.ID, folder, uids)
			if err == nil {
				fprintf(Out, "Success: Archived %s.\n", pluralizeEmails(len(uids)))
			}
		case cmdDelete:
			err = svc.DeleteEmails(account.ID, folder, uids)
			if err == nil {
				fprintf(Out, "Success: Deleted %s.\n", pluralizeEmails(len(uids)))
			}
		case cmdMarkRead:
			err = svc.MarkRead(account.ID, folder, uids)
			if err == nil {
				fprintf(Out, "Success: Marked %s as read.\n", pluralizeEmails(len(uids)))
			}
		case cmdMarkUnread:
			err = svc.MarkUnread(account.ID, folder, uids)
			if err == nil {
				fprintf(Out, "Success: Marked %s as unread.\n", pluralizeEmails(len(uids)))
			}
		}
		return err

	case cmdMove:
		if len(positionals) < 3 {
			return fmt.Errorf("source_folder, destination_folder, and uids are required")
		}
		srcFolder := NormalizeFolder(positionals[0])
		dstFolder := NormalizeFolder(positionals[1])
		uids, err := parseUIDs(positionals[2])
		if err != nil {
			return err
		}
		if len(uids) == 0 {
			return fmt.Errorf("no valid UIDs provided")
		}
		err = svc.MoveEmails(account.ID, uids, srcFolder, dstFolder)
		if err == nil {
			fprintf(Out, "Success: Moved %s to %s.\n", pluralizeEmails(len(uids)), dstFolder)
		}
		return err

	default:
		return fmt.Errorf("unknown subcommand %q", subcommand)
	}
}
