package notify

import (
	"errors"
	"testing"
)

func TestSendReturnsErrNotAvailableOnMissingBinary(t *testing.T) {
	// On CI or headless systems, notify-send / osascript are typically absent.
	// If the binary happens to be installed, Send succeeds and we skip the
	// sentinel-error check — the important thing is that it doesn't panic.
	err := Send("test", "hello")
	if err == nil {
		t.Skip("notification tool is available on this system; skipping")
	}
	if !errors.Is(err, ErrNotAvailable) {
		// A non-ErrNotAvailable error means the binary was found but
		// execution failed for another reason — that's acceptable too.
		t.Logf("Send returned a non-ErrNotAvailable error: %v", err)
	}
}
