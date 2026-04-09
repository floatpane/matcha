package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunInstall handles `matcha install <url_or_file>`.
func RunInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: matcha install <url_or_file>")
	}

	source := args[0]
	var data []byte
	var filename string

	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		// Download from URL
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(source)
		if err != nil {
			return fmt.Errorf("failed to download: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download returned status %d", resp.StatusCode)
		}

		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		// Extract filename from URL path
		parts := strings.Split(strings.TrimRight(source, "/"), "/")
		filename = parts[len(parts)-1]
	} else {
		// Read from local file
		var err error
		data, err = os.ReadFile(source)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		filename = filepath.Base(source)
	}

	if !strings.HasSuffix(filename, ".lua") {
		return fmt.Errorf("plugin file must have a .lua extension")
	}

	pluginsDir, err := pluginsDir()
	if err != nil {
		return err
	}

	dest := filepath.Join(pluginsDir, filename)
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return fmt.Errorf("failed to write plugin: %w", err)
	}

	fmt.Printf("Installed %s to %s\n", filename, dest)
	return nil
}

func pluginsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "matcha", "plugins")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("cannot create plugins directory: %w", err)
	}
	return dir, nil
}
