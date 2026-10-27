package cli

import (
	"io"
	"os"

	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/daemonclient"
)

// Package-level hookable variables for unit testing
var (
	configLoadConfig           = config.LoadConfig
	NewServiceFunc             = daemonclient.NewCLIClient
	Out              io.Writer = os.Stdout
	ErrOut           io.Writer = os.Stderr
)
