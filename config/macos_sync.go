package config

import (
	"fmt"
	"runtime"

	"github.com/floatpane/matcha/clib/macos"
)

// SyncMacOSContacts fetches contacts from the macOS Contacts framework
// and merges them into the local contacts cache.
func SyncMacOSContacts() error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	macContacts, err := macos.FetchContacts()
	if err != nil {
		return fmt.Errorf("failed to fetch macOS contacts: %w", err)
	}

	for _, mc := range macContacts {
		for _, email := range mc.Emails {
			// AddContact handles deduplication and name updates
			if err := AddContact(mc.Name, email); err != nil {
				// We continue even if one fails
				continue
			}
		}
	}

	return nil
}
