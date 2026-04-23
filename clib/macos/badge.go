package macos

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
)

//go:embed badge.swift
var badgeSwift string

// SetBadge updates the macOS Dock badge count.
func SetBadge(count int) error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	tmpDir, err := os.MkdirTemp("", "matcha-badge")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	swiftFile := filepath.Join(tmpDir, "badge.swift")
	if err := os.WriteFile(swiftFile, []byte(badgeSwift), 0644); err != nil {
		return err
	}

	binFile := filepath.Join(tmpDir, "badge")

	// Compile
	cmd := exec.Command("swiftc", swiftFile, "-o", binFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to compile badge helper: %w\n%s", err, string(out))
	}

	// Run
	// If we want to target our specific app, we might need a different approach,
	// but for now, this will set the badge for the process that runs it.
	// To set it for the 'MatchaMail.app', we'd need that app to be running and
	// listen for a notification, OR we run this compiled tool *inside* the app bundle context.

	err = exec.Command(binFile, strconv.Itoa(count)).Run()
	if err != nil {
		return fmt.Errorf("failed to set badge: %w", err)
	}

	return nil
}
