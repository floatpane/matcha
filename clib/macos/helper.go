package macos

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// The macOS integrations are small Swift programs compiled with swiftc on
// first use. Compiling takes hundreds of milliseconds to seconds, so each
// helper is compiled once and the binary is reused across calls and runs. The
// cache key includes a hash of the Swift source, so a new matcha version with
// a changed helper recompiles automatically.

var (
	helperMu sync.Mutex
	// helperFailures remembers compile failures for the life of the process
	// so a missing toolchain is not retried (and re-prompted) on every call.
	helperFailures = map[string]error{}
)

// helperDir is where compiled helpers are kept (~/.cache/matcha/helpers,
// next to the rest of matcha's cache).
func helperDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "matcha", "helpers"), nil
}

// compiledHelper returns the path of the compiled helper for source, building
// it only when no binary for this exact source exists yet. It is safe for
// concurrent use.
func compiledHelper(name, source string) (string, error) {
	sum := sha256.Sum256([]byte(source))
	key := name + "-" + hex.EncodeToString(sum[:8])

	helperMu.Lock()
	defer helperMu.Unlock()

	if err, failed := helperFailures[key]; failed {
		return "", err
	}

	dir, err := helperDir()
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, key)
	if info, err := os.Stat(bin); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o100 != 0 {
		return bin, nil
	}

	if err := buildHelper(dir, name, source, bin); err != nil {
		helperFailures[key] = err
		return "", err
	}
	return bin, nil
}

func buildHelper(dir, name, source, bin string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmpDir, err := os.MkdirTemp(dir, "build-"+name+"-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir) //nolint:errcheck

	swiftFile := filepath.Join(tmpDir, name+".swift")
	if err := os.WriteFile(swiftFile, []byte(source), 0o600); err != nil {
		return err
	}
	out := filepath.Join(tmpDir, name)
	cmd := exec.Command("swiftc", swiftFile, "-o", out) //nolint:noctx
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to compile %s helper: %w\n%s", name, err, string(output))
	}
	// Rename into place so a concurrently starting process never runs a
	// partially written binary.
	return os.Rename(out, bin)
}
