package cli

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/floatpane/matcha/gitmail"
)

// RunApply handles the "matcha apply" subcommand: apply a format-patch email
// (from a file or stdin) to a local git working tree.
func RunApply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	repo := fs.String("repo", ".", "path to the working tree to apply into")
	reverse := fs.Bool("reverse", false, "unapply the patch instead of applying it")
	check := fs.Bool("check", false, "validate only; do not write any files")
	series := fs.Bool("series", false, "treat the input as an mbox and apply the whole series")
	help := fs.Bool("h", false, "show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		fmt.Println("Usage: matcha apply [flags] [patch-file]")
		fmt.Println("")
		fmt.Println("Apply a patch received by email (git format-patch / send-email)")
		fmt.Println("to a local working tree. Reads the patch from the given file, or")
		fmt.Println("from stdin when no file is given. Never runs git.")
		fmt.Println("")
		fmt.Println("Flags:")
		fs.PrintDefaults()
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  matcha apply fix.patch --repo ~/src/proj")
		fmt.Println("  git format-patch -1 --stdout | matcha apply --repo .")
		fmt.Println("  matcha apply --check series.mbox --series   # dry-run a series")
		fmt.Println("  matcha apply --reverse fix.patch            # undo it")
		return nil
	}

	raw, err := readPatchInput(fs.Arg(0))
	if err != nil {
		return err
	}

	opts := gitmail.Options{Reverse: *reverse, DryRun: *check}

	if *series {
		summaries, err := gitmail.ApplySeries(raw, *repo, opts)
		printSummaries(summaries, *check)
		return err
	}

	summary, err := gitmail.Apply(raw, *repo, opts)
	if err != nil {
		return err
	}
	printSummaries([]*gitmail.Summary{summary}, *check)
	return nil
}

// readPatchInput reads the patch from path, or from stdin when path is empty.
func readPatchInput(path string) ([]byte, error) {
	if path == "" || path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func printSummaries(summaries []*gitmail.Summary, dryRun bool) {
	verb := "applied"
	if dryRun {
		verb = "would apply"
	}
	for _, s := range summaries {
		if s == nil {
			continue
		}
		if s.CoverLetter {
			fmt.Printf("cover letter: %s (nothing to apply)\n", s.Subject)
			continue
		}
		fmt.Printf("%s: %s — %s\n", verb, s.Subject, s.Author)
		for _, f := range s.Files {
			if f.OldPath != "" && f.OldPath != f.Path {
				fmt.Printf("  %-8s %s -> %s\n", f.Status, f.OldPath, f.Path)
			} else {
				fmt.Printf("  %-8s %s\n", f.Status, f.Path)
			}
		}
	}
}
