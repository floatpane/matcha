package macos

import (
	_ "embed"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

//go:embed appearance.swift
var appearanceSwift string

const goosDarwin = "darwin"

type MacOSAppearance struct {
	DarkMode    bool
	AccentColor string
}

// GetAppearance fetches the current macOS appearance (dark mode and accent color).
func GetAppearance() (*MacOSAppearance, error) {
	if runtime.GOOS != goosDarwin {
		return nil, fmt.Errorf("GetAppearance is only supported on macOS")
	}

	binFile, err := compiledHelper("appearance", appearanceSwift)
	if err != nil {
		return nil, err
	}

	// Run
	out, err := exec.Command(binFile).Output() //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("failed to run appearance helper: %w", err)
	}

	parts := strings.Fields(string(out))
	if len(parts) < 2 {
		return nil, fmt.Errorf("unexpected output from appearance helper: %s", string(out))
	}

	return &MacOSAppearance{
		DarkMode:    parts[0] == "true",
		AccentColor: parts[1],
	}, nil
}
