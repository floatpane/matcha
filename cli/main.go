package cli

import (
	"io"
	"os"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
)

// Package-level hookable variables for unit testing
var (
	configLoadConfig = config.LoadConfig
	NewServiceFunc   = func(cfg *config.Config, autoStart bool) daemonclient.Service {
		return daemonclient.NewCLIClient(cfg, autoStart)
	}
	Out    io.Writer = os.Stdout
	ErrOut io.Writer = os.Stderr
)
