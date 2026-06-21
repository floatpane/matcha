package fetchcmd

import (
	"github.com/floatpane/matcha/backend"
	"github.com/floatpane/matcha/config"
)

// ProviderResolver resolves a backend provider for a given account.
// It is the caller's responsibility (e.g. mainModel) to look up the
// cached or live provider and to handle any locking needed.
type ProviderResolver func(*config.Account) backend.Provider
