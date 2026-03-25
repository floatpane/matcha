package config

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed oauth_script.py
var embeddedOAuthScript []byte

// IsOAuth2 returns true if the account uses OAuth2 authentication.
func (a *Account) IsOAuth2() bool {
	return a.AuthMethod == "oauth2"
}

// OAuthScriptPath returns the path to the Gmail OAuth2 Python helper script.
// The script is embedded in the binary and extracted to ~/.config/matcha/oauth/
// on first use.
func OAuthScriptPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}

	scriptDir := filepath.Join(dir, "oauth")
	scriptPath := filepath.Join(scriptDir, "gmail_oauth.py")

	// Always overwrite with the embedded version to stay in sync with the binary
	if err := os.MkdirAll(scriptDir, 0700); err != nil {
		return "", fmt.Errorf("could not create oauth directory: %w", err)
	}
	if err := os.WriteFile(scriptPath, embeddedOAuthScript, 0700); err != nil {
		return "", fmt.Errorf("could not extract oauth script: %w", err)
	}

	return scriptPath, nil
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
