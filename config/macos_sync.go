package config

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/floatpane/matcha/clib/macos"
	"github.com/floatpane/matcha/internal/collections"
)

// SyncMacOSContacts fetches contacts from the macOS Contacts framework
// and merges them into the local contacts cache for all configured accounts.
//
// It reloads the configuration (re-running pass_cmd and keyring lookups);
// when a loaded config is at hand, call SyncMacOSContactsForAccounts with its
// account IDs instead.
func SyncMacOSContacts() error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	return SyncMacOSContactsForAccounts(cfg.GetAccountIDs())
}

// SyncMacOSContactsForAccounts fetches contacts from the macOS Contacts framework
// and merges them into the local contacts cache for the given accounts.
func SyncMacOSContactsForAccounts(accountIDs []string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}

	macContacts, err := macos.FetchContacts()
	if err != nil {
		return fmt.Errorf("failed to fetch macOS contacts: %w", err)
	}

	accountIDs = collections.UniqueNonEmpty(accountIDs)
	// One locked read-modify-write for the whole address book instead of one
	// per contact, email and account (thousands of full rewrites of
	// contacts.json at every launch).
	return updateContacts(func(cache *ContactsCache) (bool, error) {
		changed := false
		for _, mc := range macContacts {
			for _, email := range mc.Emails {
				email = normalizeContactEmail(email)
				if email == "" {
					continue
				}
				for _, accountID := range accountIDs {
					addContactLocked(cache, strings.TrimSpace(mc.Name), email, accountID)
					changed = true
				}
			}
		}
		return changed, nil
	})
}
