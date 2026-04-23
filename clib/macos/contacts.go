package macos

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

//go:embed contacts.swift
var contactsSwift string

type MacOSContact struct {
	Name   string   `json:"name"`
	Emails []string `json:"emails"`
}

// FetchContacts calls the macOS Contacts framework via a compiled Swift helper.
func FetchContacts() ([]MacOSContact, error) {
	if runtime.GOOS != "darwin" {
		return nil, fmt.Errorf("FetchContacts is only supported on macOS")
	}

	tmpDir, err := os.MkdirTemp("", "matcha-macos")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	swiftFile := filepath.Join(tmpDir, "contacts.swift")
	if err := os.WriteFile(swiftFile, []byte(contactsSwift), 0644); err != nil {
		return nil, err
	}

	binFile := filepath.Join(tmpDir, "contacts")

	// Compile the Swift helper
	cmd := exec.Command("swiftc", swiftFile, "-o", binFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("failed to compile contacts helper: %w\n%s", err, string(out))
	}

	// Run the helper
	out, err := exec.Command(binFile).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to run contacts helper: %w", err)
	}

	var contacts []MacOSContact
	if err := json.Unmarshal(out, &contacts); err != nil {
		return nil, fmt.Errorf("failed to parse contacts JSON: %w\nOutput: %s", err, string(out))
	}

	return contacts, nil
}
