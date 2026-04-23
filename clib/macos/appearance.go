package macos

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed appearance.swift
var appearanceSwift string

type MacOSAppearance struct {
	DarkMode    bool
	AccentColor string
}

// GetAppearance fetches the current macOS appearance (dark mode and accent color).
func GetAppearance() (*MacOSAppearance, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("GetAppearance is only supported on macOS")
	}

	tmpDir, err := os.MkdirTemp("", "matcha-appearance")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	swiftFile := filepath.Join(tmpDir, "appearance.swift")
	if err := os.WriteFile(swiftFile, []byte(appearanceSwift), 0644); err != nil {
		return nil, err
	}

	binFile := filepath.Join(tmpDir, "appearance")

	// Compile
	cmd := exec.Command("swiftc", swiftFile, "-o", binFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to compile appearance helper: %w\n%s", err, string(out))
	}

	// Run
	out, err := exec.Command(binFile).Output()
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
