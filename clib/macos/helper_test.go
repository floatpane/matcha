package macos

import (
	"os/exec"
	"runtime"
	"sync"
	"testing"
	"time"
)

// SetBadge used to compile its Swift helper on every call (~265ms on the UI
// goroutine) and the helper then crashed. It must now compile once, succeed,
// and be safe to call concurrently.
func TestSetBadgeCompilesOnceAndIsConcurrencySafe(t *testing.T) {
	if runtime.GOOS != goosDarwin {
		t.Skip("macOS only")
	}
	if _, err := exec.LookPath("swiftc"); err != nil {
		t.Skip("swiftc not installed")
	}
	t.Setenv("HOME", t.TempDir())

	if err := SetBadge(3); err != nil {
		t.Fatalf("first SetBadge: %v", err)
	}
	start := time.Now()
	if _, err := compiledHelper("badge", badgeSwift); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 50*time.Millisecond {
		t.Fatalf("cached helper lookup took %v; helper was recompiled", d)
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if err := SetBadge(n % 2); err != nil {
				t.Errorf("SetBadge(%d): %v", n%2, err)
			}
		}(i)
	}
	wg.Wait()
}
