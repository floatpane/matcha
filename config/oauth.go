package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsOAuth2 returns true if the account uses OAuth2 authentication.
func (a *Account) IsOAuth2() bool {
	return a.AuthMethod == "oauth2"
}

// OAuthScriptPath returns the path to the Gmail OAuth2 Python helper script.
// It checks for the script bundled with the binary first, then falls back to
// the user's config directory.
func OAuthScriptPath() (string, error) {
	// Check next to the running binary
	exe, err := os.Executable()
	if err == nil {
		bundled := filepath.Join(filepath.Dir(exe), "oauth", "gmail_oauth.py")
		if _, err := os.Stat(bundled); err == nil {
			return bundled, nil
		}
	}

	// Check in the config directory
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	configScript := filepath.Join(dir, "oauth", "gmail_oauth.py")
	if _, err := os.Stat(configScript); err == nil {
		return configScript, nil
	}

	return "", fmt.Errorf("gmail_oauth.py not found; install it next to the matcha binary or in ~/.config/matcha/oauth/")
}

// GetOAuth2Token retrieves a fresh OAuth2 access token for the account by
// invoking the Python helper script. The script handles token refresh
// automatically.
func GetOAuth2Token(email string) (string, error) {
	script, err := OAuthScriptPath()
	if err != nil {
		return "", err
	}

	cmd := exec.Command("python3", script, "token", email)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("oauth2 token retrieval failed: %w", err)
	}

	token := strings.TrimSpace(string(out))
	if token == "" {
		return "", fmt.Errorf("oauth2: empty access token returned")
	}

	return token, nil
}

// RunOAuth2Flow launches the OAuth2 authorization flow by invoking the Python
// helper script. It opens the user's browser for authorization.
// clientID and clientSecret are optional — if empty, the script uses stored credentials.
func RunOAuth2Flow(email, clientID, clientSecret string) error {
	script, err := OAuthScriptPath()
	if err != nil {
		return err
	}

	args := []string{script, "auth", email}
	if clientID != "" && clientSecret != "" {
		args = append(args, "--client-id", clientID, "--client-secret", clientSecret)
	}

	cmd := exec.Command("python3", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
