package backend

import (
	"fmt"

	"github.com/floatpane/matcha/config"
)

// NewFunc is the constructor signature for backend providers.
type NewFunc func(account *config.Account) (Provider, error)

var registry = map[string]NewFunc{}

// RegisterBackend registers a backend constructor for a protocol name.
func RegisterBackend(protocol string, fn NewFunc) {
	registry[protocol] = fn
}

// New creates a Provider for the given account based on its Protocol field.
// An empty protocol defaults to "imap".
func New(account *config.Account) (Provider, error) {
	protocol := account.Protocol
	if protocol == "" {
		protocol = "imap"
	}
	fn, ok := registry[protocol]
	if !ok {
		return nil, fmt.Errorf("unknown email protocol: %q", protocol)
	}
	return fn(account)
}
