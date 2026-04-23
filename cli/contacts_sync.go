package cli

import (
	"fmt"
	"runtime"

	"github.com/floatpane/matcha/config"
)

// RunContactsSync handles `matcha contacts sync`.
func RunContactsSync(args []string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("contacts sync is only supported on macOS")
	}

	fmt.Println("Syncing contacts from macOS Contacts framework...")
	if err := config.SyncMacOSContacts(); err != nil {
		return err
	}
	fmt.Println("Successfully synced macOS contacts.")
	return nil
}
