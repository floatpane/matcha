package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/i18n"

	// Register the bundled i18n translation catalogs via their init functions.
	_ "github.com/floatpane/matcha/i18n/languages"
)

// fprintln writes a line to w. Failed writes to the CLI's stdout/stderr are
// not actionable, so the error is intentionally discarded.
func fprintln(w io.Writer, a ...any) {
	_, _ = fmt.Fprintln(w, a...)
}

// fprintf is the fmt.Fprintf counterpart of fprintln; see its note on errors.
func fprintf(w io.Writer, format string, a ...any) {
	_, _ = fmt.Fprintf(w, format, a...)
}

// resolveAccount centralizes the logic for loading the config and selecting an account.
// It returns the loaded config and the selected account, or an error.
func resolveAccount(fromEmail string) (*config.Config, *config.Account, error) {
	cfg, err := configLoadConfig()
	if err != nil {
		return nil, nil, fmt.Errorf("could not load config: %w", err)
	}

	// Initialize i18n if not already initialized, set language based on configuration/env
	// GetManager() auto-initializes with English if not already initialized
	if i18n.GetManager() == nil {
		if err := i18n.Init("en"); err != nil {
			fprintf(ErrOut, "warning: failed to initialize i18n: %v\n", err)
		}
	}

	if manager := i18n.GetManager(); manager != nil {
		lang := i18n.DetectLanguage(cfg)
		if err := manager.SetLanguage(lang); err != nil {
			fprintf(ErrOut, "warning: failed to set i18n language: %v\n", err)
		}
	}

	if !cfg.HasAccounts() {
		return nil, nil, fmt.Errorf("no accounts configured")
	}

	var account *config.Account
	if fromEmail != "" {
		// Try matching against login Email case-insensitively first
		for i := range cfg.Accounts {
			if strings.EqualFold(cfg.Accounts[i].Email, fromEmail) {
				account = &cfg.Accounts[i]
				break
			}
		}
		if account == nil {
			// Try matching against FetchEmail as a fallback case-insensitively
			for i := range cfg.Accounts {
				if strings.EqualFold(cfg.Accounts[i].FetchEmail, fromEmail) {
					account = &cfg.Accounts[i]
					break
				}
			}
		}
		if account == nil {
			return nil, nil, fmt.Errorf("no account found matching %q", fromEmail)
		}
	} else {
		account = cfg.GetFirstAccount()
	}

	return cfg, account, nil
}
