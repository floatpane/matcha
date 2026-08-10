package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/floatpane/matcha/internal/httpclient"
	"github.com/floatpane/matcha/internal/marketplace"
	"github.com/floatpane/matcha/plugins"
)

const (
	rawThemeBaseURL = "https://raw.githubusercontent.com/floatpane/matcha-themes/master/themes/"

	installKindPlugin = "plugin"
	installKindTheme  = "theme"
)

// RunInstall handles `matcha install [plugin|theme] <name_or_url_or_file>`.
func RunInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: matcha install [plugin|theme] <name_or_url_or_file>")
	}

	kind, source := detectKind(args)

	switch kind {
	case installKindTheme:
		return installTheme(source)
	case installKindPlugin:
		return installPlugin(source)
	default:
		return fmt.Errorf("unknown install kind: %s", kind)
	}
}

// detectKind resolves the install kind and source from the command arguments.
// It returns ("plugin"|"theme", source).
func detectKind(args []string) (string, string) {
	if len(args) >= 2 {
		switch args[0] {
		case installKindPlugin:
			return installKindPlugin, args[1]
		case installKindTheme:
			return installKindTheme, args[1]
		}
	}

	source := args[0]

	// Explicit URLs or local files are downloaded as-is; infer from extension.
	if isURL(source) || isFilePath(source) {
		if strings.HasSuffix(source, ".json") {
			return installKindTheme, source
		}
		return installKindPlugin, source
	}

	// Ambiguous bare name: check for collisions between plugin/theme names.
	isPluginName := isKnownPluginName(source)
	isThemeName := isKnownThemeName(source)

	if isPluginName && !isThemeName {
		return installKindPlugin, source
	}
	if isThemeName && !isPluginName {
		return installKindTheme, source
	}
	if isPluginName && isThemeName {
		return "", source
	}

	// Unknown name: assume plugin to preserve existing behavior.
	return installKindPlugin, source
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

func isFilePath(s string) bool {
	return strings.Contains(s, string(filepath.Separator)) || strings.HasPrefix(s, ".") || filepath.IsAbs(s)
}

func isKnownPluginName(name string) bool {
	entries, err := plugins.FetchRegistry()
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name, name) || strings.EqualFold(strings.TrimSuffix(e.File, ".lua"), name) {
			return true
		}
	}
	return false
}

func isKnownThemeName(name string) bool {
	client := httpclient.New(httpclient.RegistryFetchTimeout)
	resp, err := client.Get("https://api.github.com/repos/floatpane/matcha-themes/contents/themes?ref=master") //nolint:noctx
	if err != nil {
		return false
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return false
	}

	var items []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return false
	}

	for _, item := range items {
		if strings.EqualFold(strings.TrimSuffix(item.Name, ".json"), name) {
			return true
		}
	}
	return false
}

func installPlugin(source string) error {
	// Check if source is a plugin name from marketplace
	if !isURL(source) && !isFilePath(source) {
		return installFromMarketplace(source)
	}

	data, filename, err := resolvePluginSource(source)
	if err != nil {
		return err
	}

	if !strings.HasSuffix(filename, ".lua") {
		filename += ".lua"
	}

	dir, err := pluginsDir()
	if err != nil {
		return err
	}

	dest := filepath.Join(dir, filename)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("failed to write plugin: %w", err)
	}

	fmt.Printf("Installed plugin %s to %s\n", filename, dest)
	return nil
}

// installFromMarketplace installs a plugin from the new marketplace with security checks
func installFromMarketplace(name string) error {
	// Fetch plugin info from marketplace API
	plugin, err := marketplace.FetchPluginInfo(name)
	if err != nil {
		// Fallback to old registry system
		return installFromOldRegistry(name)
	}

	// Check verification status
	if !plugin.IsVerifiedSafe() {
		fmt.Printf("⚠️  Warning: Plugin '%s' has not been verified as safe\n", plugin.Title)
		fmt.Printf("   Verification status: %s\n", plugin.VerificationStatus)
		fmt.Println()
		fmt.Print("Do you want to continue? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			return fmt.Errorf("installation cancelled by user")
		}
	}

	// Check if author is trusted
	if !plugin.IsTrustedAuthor() {
		fmt.Printf("⚠️  Warning: Plugin author '%s' is not verified\n", plugin.Author.DisplayName)
		fmt.Printf("   GitHub: https://github.com/%s\n", plugin.Author.GitHubUsername)
		fmt.Println()
		fmt.Print("Do you trust this author? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			return fmt.Errorf("installation cancelled by user")
		}
	}

	// Download plugin file
	client := httpclient.New(httpclient.RegistryFetchTimeout)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, plugin.FileURL, nil)
	if err != nil {
		return fmt.Errorf("failed to build download request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download plugin: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download plugin: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read plugin content: %w", err)
	}

	// Verify SHA256
	computedHash := sha256.Sum256(data)
	computedHashStr := hex.EncodeToString(computedHash[:])

	if computedHashStr != plugin.SHA256 {
		fmt.Printf("⚠️  Warning: SHA256 mismatch!\n")
		fmt.Printf("   Expected: %s\n", plugin.SHA256)
		fmt.Printf("   Got:      %s\n", computedHashStr)
		fmt.Println()
		fmt.Print("Do you want to continue anyway? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response != "y" && response != "yes" {
			return fmt.Errorf("installation cancelled due to SHA256 mismatch")
		}
	}

	filename := plugin.Name + ".lua"
	dir, err := pluginsDir()
	if err != nil {
		return err
	}

	dest := filepath.Join(dir, filename)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("failed to write plugin: %w", err)
	}

	fmt.Printf("✓ Installed plugin %s v%s\n", plugin.Title, plugin.Version)
	fmt.Printf("  Location: %s\n", dest)

	if plugin.IsTrustedAuthor() && plugin.IsVerifiedSafe() {
		fmt.Println("  Status: Verified safe ✓")
	}

	return nil
}

// installFromOldRegistry falls back to the old registry system
func installFromOldRegistry(name string) error {
	entries, err := plugins.FetchRegistry()
	if err != nil {
		return fmt.Errorf("failed to fetch plugin registry: %w", err)
	}

	for _, e := range entries {
		if strings.EqualFold(e.Name, name) || strings.EqualFold(strings.TrimSuffix(e.File, ".lua"), name) {
			data, err := plugins.FetchPlugin(e)
			if err != nil {
				return err
			}

			filename := e.File
			if !strings.HasSuffix(filename, ".lua") {
				filename += ".lua"
			}

			dir, err := pluginsDir()
			if err != nil {
				return err
			}

			dest := filepath.Join(dir, filename)
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				return fmt.Errorf("failed to write plugin: %w", err)
			}

			fmt.Printf("Installed plugin %s to %s\n", filename, dest)
			return nil
		}
	}

	return fmt.Errorf("plugin '%s' not found", name)
}

func resolvePluginSource(source string) ([]byte, string, error) {
	if isURL(source) {
		data, err := download(source)
		if err != nil {
			return nil, "", err
		}
		parts := strings.Split(strings.TrimRight(source, "/"), "/")
		return data, parts[len(parts)-1], nil
	}

	// Local file fallback.
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, "", fmt.Errorf("plugin not found: %s", source)
	}
	return data, filepath.Base(source), nil
}

func installTheme(source string) error {
	data, filename, err := resolveThemeSource(source)
	if err != nil {
		return err
	}

	if !strings.HasSuffix(filename, ".json") {
		filename += ".json"
	}

	if err := validateThemeJSON(data); err != nil {
		return err
	}

	dir, err := themesDir()
	if err != nil {
		return err
	}

	dest := filepath.Join(dir, filename)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("failed to write theme: %w", err)
	}

	fmt.Printf("Installed theme %s to %s\n", filename, dest)
	return nil
}

func resolveThemeSource(source string) ([]byte, string, error) {
	if isURL(source) {
		data, err := download(source)
		if err != nil {
			return nil, "", err
		}
		parts := strings.Split(strings.TrimRight(source, "/"), "/")
		return data, parts[len(parts)-1], nil
	}

	// GitHub repo shorthand: owner/repo:path/to/file.json
	if repoRef, path, ok := splitRepoRef(source); ok {
		url := fmt.Sprintf("https://raw.githubusercontent.com/%s/master/%s", repoRef, path)
		data, err := download(url)
		if err != nil {
			return nil, "", err
		}
		return data, filepath.Base(path), nil
	}

	// Bare name from official matcha-themes registry.
	filename := source
	if !strings.HasSuffix(filename, ".json") {
		filename += ".json"
	}
	data, err := download(rawThemeBaseURL + filename)
	if err != nil {
		return nil, "", fmt.Errorf("theme not found: %s", source)
	}
	return data, filename, nil
}

// splitRepoRef parses "owner/repo:path/to/file" into (owner/repo, path, true).
func splitRepoRef(s string) (string, string, bool) {
	if strings.Count(s, "/") < 1 || strings.Count(s, ":") != 1 {
		return "", "", false
	}
	colon := strings.Index(s, ":")
	if colon <= 0 {
		return "", "", false
	}
	repoRef := s[:colon]
	path := s[colon+1:]
	if strings.Count(repoRef, "/") != 1 || repoRef[0] == '/' || repoRef[len(repoRef)-1] == '/' {
		return "", "", false
	}
	if path == "" {
		return "", "", false
	}
	return repoRef, path, true
}

func validateThemeJSON(data []byte) error {
	var theme struct {
		Name       string `json:"name"`
		Accent     string `json:"accent"`
		AccentDark string `json:"accent_dark"`
		AccentText string `json:"accent_text"`
		Secondary  string `json:"secondary"`
		SubtleText string `json:"subtle_text"`
		MutedText  string `json:"muted_text"`
		DimText    string `json:"dim_text"`
		Danger     string `json:"danger"`
		Warning    string `json:"warning"`
		Tip        string `json:"tip"`
		Link       string `json:"link"`
		Directory  string `json:"directory"`
		Contrast   string `json:"contrast"`
	}
	if err := json.Unmarshal(data, &theme); err != nil {
		return fmt.Errorf("theme file is not valid JSON: %w", err)
	}
	if theme.Name == "" {
		return fmt.Errorf("theme is missing required field: name")
	}
	if theme.Accent == "" || theme.AccentDark == "" || theme.Contrast == "" {
		return fmt.Errorf("theme is missing required color fields")
	}
	return nil
}

func download(url string) ([]byte, error) {
	client := httpclient.New(httpclient.InstallTimeout)
	resp, err := client.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func pluginsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "matcha", "plugins")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("cannot create plugins directory: %w", err)
	}
	return dir, nil
}

func themesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	dir := filepath.Join(home, ".config", "matcha", "themes")
	if err := os.MkdirAll(dir, 0750); err != nil {
		return "", fmt.Errorf("cannot create themes directory: %w", err)
	}
	return dir, nil
}
