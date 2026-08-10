package marketplace

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/floatpane/matcha/internal/httpclient"
)

const MarketplaceAPI = "https://marketplace.matcha.email/api"

type PluginInfo struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Version            string   `json:"version"`
	Author             Author   `json:"author"`
	Maintainer         Author   `json:"maintainer"`
	RepositoryURL      string   `json:"repository_url"`
	FileURL            string   `json:"file_url"`
	SHA256             string   `json:"sha256"`
	Status             string   `json:"status"`
	VerificationStatus string   `json:"verification_status"`
	Downloads          int      `json:"downloads"`
	Tags               []string `json:"tags"`
}

type Author struct {
	GitHubUsername string `json:"github_username"`
	DisplayName    string `json:"display_name"`
	IsVerified     bool   `json:"is_verified"`
}

// FetchPluginInfo retrieves plugin metadata from the marketplace API
func FetchPluginInfo(name string) (*PluginInfo, error) {
	client := httpclient.New(httpclient.RegistryFetchTimeout)
	url := fmt.Sprintf("%s/plugins?id=%s", MarketplaceAPI, name)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plugin info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("plugin '%s' not found in marketplace", name)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("marketplace API returned status %d", resp.StatusCode)
	}

	var plugin PluginInfo
	if err := json.NewDecoder(resp.Body).Decode(&plugin); err != nil {
		return nil, fmt.Errorf("failed to decode plugin info: %w", err)
	}

	return &plugin, nil
}

// ListPlugins retrieves all approved plugins from the marketplace
func ListPlugins() ([]PluginInfo, error) {
	client := httpclient.New(httpclient.RegistryFetchTimeout)
	url := fmt.Sprintf("%s/plugins", MarketplaceAPI)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch plugins list: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("marketplace API returned status %d", resp.StatusCode)
	}

	var plugins []PluginInfo
	if err := json.NewDecoder(resp.Body).Decode(&plugins); err != nil {
		return nil, fmt.Errorf("failed to decode plugins list: %w", err)
	}

	return plugins, nil
}

// IsTrustedAuthor checks if the author is verified/trusted
func (p *PluginInfo) IsTrustedAuthor() bool {
	return p.Author.IsVerified && p.Maintainer.IsVerified
}

// IsVerifiedSafe checks if the plugin has passed security verification
func (p *PluginInfo) IsVerifiedSafe() bool {
	return p.VerificationStatus == "clean"
}
