package oauthcmd

import (
	"errors"
	"os"
	"os/exec"

	"github.com/floatpane/matcha/cli"
	"github.com/floatpane/matcha/internal/loglevel"
)

// Run implements the CLI entrypoint for `matcha oauth`.
// Usage:
//
//	matcha oauth auth   <email> [--provider gmail|outlook] [--client-id ID --client-secret SECRET]
//	matcha oauth token  <email>
//	matcha oauth revoke <email>
func Run(args []string) {
	if len(args) < 1 {
		loglevel.Infof("Usage: matcha oauth <auth|token|revoke> <email> [flags]")
		loglevel.Infof("")
		loglevel.Infof("Commands:")
		loglevel.Infof("  auth   <email>  Authorize an email account via OAuth2 (opens browser)")
		loglevel.Infof("  token  <email>  Print a fresh access token (refreshes automatically)")
		loglevel.Infof("  revoke <email>  Revoke and delete stored OAuth2 tokens")
		loglevel.Infof("")
		loglevel.Infof("Flags for auth:")
		loglevel.Infof("  --provider gmail|outlook  OAuth2 provider (auto-detected from email)")
		loglevel.Infof("  --client-id ID            OAuth2 client ID")
		loglevel.Infof("  --client-secret SECRET    OAuth2 client secret")
		loglevel.Infof("")
		loglevel.Infof("Credentials are stored per provider in:")
		loglevel.Infof("  Gmail:   ~/.config/matcha/oauth_client.json")
		loglevel.Infof("  Outlook: ~/.config/matcha/oauth_client_outlook.json")
		os.Exit(1)
	}

	script, err := cli.OAuthScriptPath()
	if err != nil {
		loglevel.Infof("Error: %v", err)
		os.Exit(1)
	}

	cmdArgs := append([]string{script}, args...)
	cmd := exec.Command("python3", cmdArgs...) //nolint:noctx
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		loglevel.Infof("Error: %v", err)
		os.Exit(1)
	}
}
