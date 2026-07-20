package config

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

//go:embed oauth_script.py
var embeddedOAuthScript []byte

// IsOAuth2 returns true if the account uses OAuth2 authentication.
func (a *Account) IsOAuth2() bool {
	return a.AuthMethod == "oauth2"
}

// OAuthScriptPath returns the path to the OAuth2 Python helper script.
// The script is embedded in the binary and extracted to ~/.config/matcha/oauth/
// on first use.
func OAuthScriptPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}

	scriptDir := filepath.Join(dir, "oauth")
	scriptPath := filepath.Join(scriptDir, "oauth.py")

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
// automatically. The subprocess is killed after 30 seconds to prevent hangs.
func GetOAuth2Token(email string) (string, error) {
	script, err := OAuthScriptPath()
	if err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", script, "token", email)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	out, err := cmd.Output()
	if err != nil {
		if stderrBuf.Len() > 0 {
			return "", fmt.Errorf("oauth2 token retrieval failed: %w: %s", err, strings.TrimSpace(stderrBuf.String()))
		}
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
// provider should be "gmail" or "outlook". If empty, the script auto-detects from the email.
// clientID and clientSecret are optional — if empty, the script uses stored credentials.
// The subprocess is killed after 5 minutes to prevent indefinite hangs.
func RunOAuth2Flow(email, provider, clientID, clientSecret string) error {
	script, err := OAuthScriptPath()
	if err != nil {
		return err
	}

	args := []string{script, "auth", email}
	if provider != "" {
		args = append(args, "--provider", provider)
	}
	if clientID != "" && clientSecret != "" {
		args = append(args, "--client-id", clientID, "--client-secret", clientSecret)
	}

	// 5-minute timeout: the user needs time to complete the browser-based
	// OAuth consent flow, but the process must not hang forever.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", args...)
	cmd.Stdout = os.Stdout

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		if stderrBuf.Len() > 0 {
			return fmt.Errorf("oauth2 flow failed: %w: %s", err, strings.TrimSpace(stderrBuf.String()))
		}
		return fmt.Errorf("oauth2 flow failed: %w", err)
	}
	return nil
}
