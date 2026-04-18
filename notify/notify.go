package notify

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"sync"
)

// ErrNotifierMissing is returned by Send when the platform's notification
// command (notify-send on Linux, osascript on macOS) is not installed or
// not on PATH. Callers that fire-and-forget still get a predictable error
// instead of whatever exec.ErrNotFound happens to unwrap to.
var ErrNotifierMissing = errors.New("notify: platform notifier not installed")

// warnOnce guarantees that a single "notifier not installed" warning is
// logged per process even when Send is called repeatedly from a goroutine
// whose error is discarded (go notify.Send(...) in main.go).
var warnOnce sync.Once

// Send delivers a desktop notification with the given title and body.
// On macOS it uses osascript; on Linux it uses notify-send. When the
// platform notifier is unavailable the function returns ErrNotifierMissing
// and logs a single warning per process so headless or minimal installs
// are not silently broken (#522).
func Send(title, body string) error {
	var bin string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		bin = "osascript"
		script := fmt.Sprintf(`display notification %q with title %q sound name "default"`, body, title)
		args = []string{"-e", script}
	case "linux":
		bin = "notify-send"
		args = []string{title, body}
	default:
		return nil
	}

	if _, err := exec.LookPath(bin); err != nil {
		warnOnce.Do(func() {
			log.Printf("matcha: desktop notifications disabled, %q is not installed or not on PATH (%v)", bin, err)
		})
		return fmt.Errorf("%w: %s", ErrNotifierMissing, bin)
	}

	return exec.Command(bin, args...).Run()
}
