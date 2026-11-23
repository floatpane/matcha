package macos

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
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
	if runtime.GOOS != goosDarwin {
		return nil, fmt.Errorf("FetchContacts is only supported on macOS")
	}

	binFile, err := compiledHelper("contacts", contactsSwift)
	if err != nil {
		return nil, err
	}

	// Run the helper
	out, err := exec.Command(binFile).Output() //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("failed to run contacts helper: %w", err)
	}

	var contacts []MacOSContact
	if err := json.Unmarshal(out, &contacts); err != nil {
		return nil, fmt.Errorf("failed to parse contacts JSON: %w\nOutput: %s", err, string(out))
	}

	return contacts, nil
}
