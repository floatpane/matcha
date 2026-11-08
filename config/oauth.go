package config

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"io"
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
// on first use, and replaced whenever it differs from the embedded version.
func OAuthScriptPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}

	scriptDir := filepath.Join(dir, "oauth")
	scriptPath := filepath.Join(scriptDir, "oauth.py")

	if err := os.MkdirAll(scriptDir, 0700); err != nil {
		return "", fmt.Errorf("could not create oauth directory: %w", err)
	}

	// This runs on every token request, possibly while another request is
	// executing the script, so don't touch a script that is already current.
	if current, err := os.ReadFile(scriptPath); err == nil && bytes.Equal(current, embeddedOAuthScript) {
		return scriptPath, nil
	}
	if err := writeOAuthScript(scriptPath); err != nil {
		return "", fmt.Errorf("could not extract oauth script: %w", err)
	}

	return scriptPath, nil
}

// writeOAuthScript installs the embedded script at path atomically. Truncating
// the file in place would let a concurrent python3 read it empty, exit 0 and
// print no token.
func writeOAuthScript(path string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".oauth-*.py")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) //nolint:errcheck

	if _, err := tmp.Write(embeddedOAuthScript); err != nil {
		tmp.Close() //nolint:errcheck,gosec
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0700); err != nil { // #nosec G302 -- the script is executable
		return err
	}
	return os.Rename(tmpPath, path)
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
// onOutput, if non-nil, receives each progress line the script writes to stderr
// (including the authorization URL) as it happens.
// The subprocess is killed after 5 minutes to prevent indefinite hangs.
func RunOAuth2Flow(email, provider, clientID, clientSecret string, onOutput func(string)) error {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", args...)
	cmd.Stdout = io.Discard

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("oauth2 flow failed: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("oauth2 flow failed: %w", err)
	}

	var stderrBuf bytes.Buffer
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()
		stderrBuf.WriteString(line + "\n")
		if onOutput != nil {
			onOutput(line)
		}
	}
	if err := cmd.Wait(); err != nil {
		if stderrBuf.Len() > 0 {
			return fmt.Errorf("oauth2 flow failed: %w: %s", err, strings.TrimSpace(stderrBuf.String()))
		}
		return fmt.Errorf("oauth2 flow failed: %w", err)
	}
	return nil
}
