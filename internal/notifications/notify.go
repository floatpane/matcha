package notifications

import (
	"fmt"

	"github.com/floatpane/matcha/clib/macos"
	"github.com/floatpane/matcha/config"
	"github.com/floatpane/matcha/fetcher"
	"github.com/floatpane/matcha/notify"
)

// SendNewMail sends a desktop notification for new mail in a folder.
func SendNewMail(cfg *config.Config, accountID, folderName string) {
	if cfg != nil && cfg.DisableNotifications {
		return
	}
	accountName := accountID
	if cfg != nil {
		if acc := cfg.GetAccountByID(accountID); acc != nil {
			accountName = acc.Email
		}
	}
	go notify.Send("Matcha", fmt.Sprintf("New mail in %s (%s)", folderName, accountName)) //nolint:errcheck
}

// SyncUnreadBadge sets the macOS dock badge to the unread count, if enabled.
func SyncUnreadBadge(cfg *config.Config, store interface{ CountUnread() int }) {
	if cfg != nil && cfg.DisableNotifications {
		return
	}
	count := store.CountUnread()
	_ = macos.SetBadge(count)
}

// CountUnread counts unread emails across all stores, deduplicated by UID.
func CountUnread(emailsByAcct, folderEmails map[string][]fetcher.Email) int {
	count := 0
	seen := make(map[string]struct{})
	for _, emails := range emailsByAcct {
		for _, e := range emails {
			if e.IsRead {
				continue
			}
			key := fmt.Sprintf("%s:%d", e.AccountID, e.UID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			count++
		}
	}
	for _, emails := range folderEmails {
		for _, e := range emails {
			if e.IsRead {
				continue
			}
			key := fmt.Sprintf("%s:%d", e.AccountID, e.UID)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			count++
		}
	}
	return count
}
