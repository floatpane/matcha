package cliutil

import (
	"os"

	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/internal/loglevel"
)

// ParseGlobalFlags extracts root-level debug/log flags from the command line.
// It returns the remaining args, the log level to use, and whether the in-app
// log panel should be shown.
func ParseGlobalFlags(args []string) ([]string, loglevel.Level, bool) {
	level := loglevel.LevelInfo
	showLogPanel := false
	if len(args) <= 1 {
		return args, level, showLogPanel
	}

	filtered := make([]string, 0, len(args))
	filtered = append(filtered, args[0])

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--debug":
			level = loglevel.LevelDebug
		case "--verbose", "-V":
			if level < loglevel.LevelVerbose {
				level = loglevel.LevelVerbose
			}
		case "--logs":
			showLogPanel = true
		default:
			filtered = append(filtered, args[i:]...)
			return filtered, level, showLogPanel
		}
	}

	return filtered, level, showLogPanel
}

// Exit flushes any debug output and exits with the given code.
func Exit(code int) {
	fetcher.CloseDebugFiles()
	os.Exit(code)
}
