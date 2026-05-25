package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/floatpane/matcha/spellcheck"
)

// RunDict dispatches `matcha dict <subcommand>`.
func RunDict(args []string) error {
	if len(args) == 0 {
		return dictUsage()
	}
	switch args[0] {
	case "add":
		return RunDictAdd(args[1:])
	case "remove", "rm":
		return RunDictRemove(args[1:])
	case "list", "ls":
		return RunDictList()
	default:
		return dictUsage()
	}
}

func dictUsage() error {
	return fmt.Errorf("usage:\n  matcha dict add <language-code>\n  matcha dict remove <language-code>\n  matcha dict list")
}

// RunDictAdd downloads and installs a spellcheck dictionary.
func RunDictAdd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: matcha dict add <language-code>")
	}
	lang := strings.TrimSpace(args[0])
	if lang == "" {
		return fmt.Errorf("empty language code")
	}
	fmt.Printf("Downloading %s dictionary...\n", lang)
	path, err := spellcheck.Download(lang)
	if err != nil {
		return err
	}
	fmt.Printf("Installed %s -> %s\n", lang, path)
	return nil
}

// RunDictRemove deletes an installed dictionary.
func RunDictRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: matcha dict remove <language-code>")
	}
	lang := strings.TrimSpace(args[0])
	path, err := spellcheck.DictPath(lang)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("dictionary not installed: %s", lang)
		}
		return fmt.Errorf("remove %s: %w", lang, err)
	}
	fmt.Printf("Removed %s\n", lang)
	return nil
}

// RunDictList prints all installed dictionaries.
func RunDictList() error {
	dir, err := spellcheck.DictsDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read dicts dir: %w", err)
	}
	var langs []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".dic") {
			continue
		}
		langs = append(langs, strings.TrimSuffix(name, ".dic"))
	}
	sort.Strings(langs)
	if len(langs) == 0 {
		fmt.Println("No dictionaries installed.")
		fmt.Printf("Run `matcha dict add <code>` (e.g. en, en-GB, de, fr).\n")
		fmt.Printf("Dictionaries are stored in: %s\n", dir)
		return nil
	}
	for _, l := range langs {
		path := filepath.Join(dir, l+".dic")
		fmt.Println(l, "  ", path)
	}
	return nil
}
