package notify

import (
	"fmt"
	"os/exec"
	"runtime"
)

// Send delivers a desktop notification with the given title and body.
// On macOS it uses osascript; on Linux it uses notify-send.
func Send(title, body string) error {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(`display notification %q with title %q sound name "default"`, body, title)
		return exec.Command("osascript", "-e", script).Run()
	case "linux":
		return exec.Command("notify-send", title, body).Run()
	default:
		return nil
	}
}
