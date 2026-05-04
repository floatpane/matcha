// Package httpclient centralizes HTTP timeout defaults so the rest of the
// codebase doesn't sprinkle magic numbers across packages.
package httpclient

import (
	"fmt"
	"net/http"
	"time"
)

// Named timeouts. Each constant documents the call site it covers so
// future contributors don't have to grep for callers.
const (
	// PluginCallTimeout bounds Lua-driven plugin HTTP calls (plugin/http.go).
	PluginCallTimeout = 10 * time.Second
	// RegistryFetchTimeout bounds plugin registry / plugin file fetches (plugins/embed.go).
	RegistryFetchTimeout = 10 * time.Second
	// RemoteImageTimeout bounds inline image fetches (view/html.go).
	// Kept short so message rendering doesn't stall.
	RemoteImageTimeout = 5 * time.Second
	// InstallTimeout bounds CLI install downloads (cli/install.go).
	InstallTimeout = 30 * time.Second
	// UpdateCheckTimeout bounds version checks and asset downloads from main (main.go).
	UpdateCheckTimeout = 30 * time.Second
	// IMAPBatchActionTimeout bounds bulk IMAP operations (delete/archive/move) from main (main.go).
	IMAPBatchActionTimeout = 60 * time.Second
	// IMAPSearchTimeout bounds server-side IMAP search queries from main (main.go).
	IMAPSearchTimeout = 60 * time.Second
)

// New returns an http.Client preconfigured with the given timeout.
func New(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// NewWithRedirectCap returns an http.Client with the given timeout and a
// hard cap on the number of redirects it will follow before giving up.
// Used by the main update / asset download client to avoid infinite chains.
func NewWithRedirectCap(timeout time.Duration, maxRedirects int) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("stopped after %d redirects", maxRedirects)
			}
			return nil
		},
	}
}
