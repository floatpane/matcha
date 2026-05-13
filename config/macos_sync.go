package config

import (
	"fmt"
	"runtime"

	"github.com/floatpane/matcha/clib/macos"
	"github.com/floatpane/matcha/internal/collections"
)

// SyncMacOSContacts fetches contacts from the macOS Contacts framework
// and merges them into the local contacts cache for all configured accounts.
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
	for _, mc := range macContacts {
		for _, email := range mc.Emails {
			for _, accountID := range accountIDs {
				if err := AddContactForAccount(mc.Name, email, accountID); err != nil {
					// We continue even if one fails
					continue
				}
			}
		}
	}

	return nil
}
