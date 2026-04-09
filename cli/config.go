package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// RunConfig handles `matcha config [plugin_name]`.
func RunConfig(args []string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot find home directory: %w", err)
	}

	var target string
	if len(args) == 0 {
		target = filepath.Join(home, ".config", "matcha", "config.json")
	} else {
		name := args[0]
		// Add .lua extension if not present
		if filepath.Ext(name) != ".lua" {
			name = name + ".lua"
		}
		target = filepath.Join(home, ".config", "matcha", "plugins", name)
	}

	if _, err := os.Stat(target); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", target)
	}

	cmd := exec.Command(editor, target)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
