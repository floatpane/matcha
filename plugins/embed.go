package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const RegistryURL = "https://raw.githubusercontent.com/floatpane/matcha/master/plugins/registry.json"
const RawPluginBaseURL = "https://raw.githubusercontent.com/floatpane/matcha/master/plugins/"

// PluginEntry represents a single plugin in the registry.
type PluginEntry struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	File        string `json:"file"`
	URL         string `json:"url,omitempty"`
}

// FetchRegistry fetches the plugin registry from GitHub.
func FetchRegistry() ([]PluginEntry, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(RegistryURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read registry: %w", err)
	}

	var entries []PluginEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse registry: %w", err)
	}
	return entries, nil
}

// FetchPlugin downloads a plugin file. If the entry has a URL, it downloads
// from there; otherwise it falls back to the default repo location.
func FetchPlugin(entry PluginEntry) ([]byte, error) {
	url := entry.URL
	if url == "" {
		url = RawPluginBaseURL + entry.File
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plugin: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin download returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
