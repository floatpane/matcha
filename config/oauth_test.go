package config

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// Token requests call OAuthScriptPath concurrently and then run the script, so
// every caller must find the complete script at the returned path.
func TestOAuthScriptPathConcurrentCallersSeeCompleteScript(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	if _, err := OAuthScriptPath(); err != nil {
		t.Fatalf("OAuthScriptPath() error = %v", err)
	}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 200 {
				path, err := OAuthScriptPath()
				if err != nil {
					t.Errorf("OAuthScriptPath() error = %v", err)
					return
				}
				got, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("ReadFile() error = %v", err)
					return
				}
				if !bytes.Equal(got, embeddedOAuthScript) {
					t.Errorf("read %d bytes of the script, want %d", len(got), len(embeddedOAuthScript))
					return
				}
			}
		})
	}
	wg.Wait()
}

func TestOAuthScriptPathLeavesCurrentScriptAlone(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	path, err := OAuthScriptPath()
	if err != nil {
		t.Fatalf("OAuthScriptPath() error = %v", err)
	}

	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("Chtimes() error = %v", err)
	}

	if _, err := OAuthScriptPath(); err != nil {
		t.Fatalf("OAuthScriptPath() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.ModTime().Equal(old) {
		t.Errorf("script was rewritten: mtime = %v, want %v", info.ModTime(), old)
	}
}

func TestOAuthScriptPathReplacesStaleScript(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)

	scriptDir := filepath.Join(dir, ".config", "matcha", "oauth")
	if err := os.MkdirAll(scriptDir, 0700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(scriptDir, "oauth.py"), []byte("# old version\n"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	path, err := OAuthScriptPath()
	if err != nil {
		t.Fatalf("OAuthScriptPath() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, embeddedOAuthScript) {
		t.Errorf("stale script was not replaced: got %d bytes, want %d", len(got), len(embeddedOAuthScript))
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if mode := info.Mode().Perm(); mode != 0700 {
			t.Errorf("script mode = %o, want 700", mode)
		}
	}

	entries, err := os.ReadDir(scriptDir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("oauth dir has %d entries, want only oauth.py: %v", len(entries), entries)
	}
}

func TestWriteOAuthScriptReadersNeverSeePartialFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot rename over a file that is open for reading")
	}

	path := filepath.Join(t.TempDir(), "oauth.py")
	if err := writeOAuthScript(path); err != nil {
		t.Fatalf("writeOAuthScript() error = %v", err)
	}

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				got, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("ReadFile() error = %v", err)
					return
				}
				if !bytes.Equal(got, embeddedOAuthScript) {
					t.Errorf("read %d bytes of the script, want %d", len(got), len(embeddedOAuthScript))
					return
				}
			}
		})
	}

	for range 500 {
		if err := writeOAuthScript(path); err != nil {
			t.Errorf("writeOAuthScript() error = %v", err)
			break
		}
	}
	close(stop)
	readers.Wait()
}
