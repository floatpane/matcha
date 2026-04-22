package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/floatpane/matcha/config"
)

// RunOAuth handles the "matcha oauth" subcommand for OAuth2 management.
// Usage:
//
//	matcha oauth auth   <email> [--provider gmail|outlook] [--client-id ID --client-secret SECRET]
//	matcha oauth token  <email>
//	matcha oauth revoke <email>
func RunOAuth(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: matcha oauth <auth|token|revoke> <email> [flags]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  auth   <email>  Authorize an email account via OAuth2 (opens browser)")
		fmt.Fprintln(os.Stderr, "  token  <email>  Print a fresh access token (refreshes automatically)")
		fmt.Fprintln(os.Stderr, "  revoke <email>  Revoke and delete stored OAuth2 tokens")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags for auth:")
		fmt.Fprintln(os.Stderr, "  --provider gmail|outlook  OAuth2 provider (auto-detected from email)")
		fmt.Fprintln(os.Stderr, "  --client-id ID            OAuth2 client ID")
		fmt.Fprintln(os.Stderr, "  --client-secret SECRET    OAuth2 client secret")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Credentials are stored per provider in:")
		fmt.Fprintln(os.Stderr, "  Gmail:   ~/.config/matcha/oauth_client.json")
		fmt.Fprintln(os.Stderr, "  Outlook: ~/.config/matcha/oauth_client_outlook.json")
		os.Exit(1)
	}

	script, err := config.OAuthScriptPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	cmdArgs := append([]string{script}, args...)
	cmd := exec.Command("python3", cmdArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
