package cli

import (
	"github.com/floatpane/matcha/config"
)

// OAuthScriptPath returns the path to the bundled OAuth helper script.
func OAuthScriptPath() (string, error) {
	return config.OAuthScriptPath()
}
