package macos

import (
	_ "embed"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
)

//go:embed badge.swift
var badgeSwift string

// maxBadgeFailures is how many consecutive helper failures are tolerated
// before SetBadge gives up for the rest of the process.
const maxBadgeFailures = 3

var badgeState struct {
	mu       sync.Mutex
	last     int
	haveLast bool
	failures int
	lastErr  error
}

// SetBadge updates the macOS Dock badge count.
//
// The Swift helper is compiled once and cached (see compiledHelper); calls are
// serialized, repeated counts are skipped, and after repeated failures the
// helper is no longer launched. It is safe to call from any goroutine, but it
// still spawns a process, so callers on the UI goroutine should run it in a
// command and debounce.
func SetBadge(count int) error {
	if runtime.GOOS != goosDarwin {
		return nil
	}

	badgeState.mu.Lock()
	defer badgeState.mu.Unlock()

	if badgeState.failures >= maxBadgeFailures {
		return fmt.Errorf("dock badge disabled after repeated failures: %w", badgeState.lastErr)
	}
	if badgeState.haveLast && badgeState.last == count {
		return nil
	}

	bin, err := compiledHelper("badge", badgeSwift)
	if err != nil {
		badgeState.failures = maxBadgeFailures // a missing toolchain will not appear mid-session
		badgeState.lastErr = err
		return err
	}

	if err := exec.Command(bin, strconv.Itoa(count)).Run(); err != nil { //nolint:noctx
		badgeState.failures++
		badgeState.lastErr = err
		return fmt.Errorf("failed to set badge: %w", err)
	}
	badgeState.failures = 0
	badgeState.lastErr = nil
	badgeState.last = count
	badgeState.haveLast = true
	return nil
}
