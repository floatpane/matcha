package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// fileLocks holds one mutex per data file. Read-modify-write helpers (contacts,
// drafts, folder caches, body caches) hold the file's lock for the whole
// load → modify → save cycle so concurrent callers (Bubble Tea commands, the
// macOS contacts sync, background cache writers) cannot lose each other's
// updates. Plain reads need no lock: writes are atomic renames, so a reader
// always sees either the old or the new file.
var fileLocks sync.Map // cleaned absolute path -> *sync.Mutex

// lockFile locks the mutex for path and returns the unlock function.
func lockFile(path string) func() {
	key := filepath.Clean(path)
	mu, _ := fileLocks.LoadOrStore(key, &sync.Mutex{})
	m := mu.(*sync.Mutex)
	m.Lock()
	return m.Unlock
}

// writeFileAtomic replaces path with data so that readers never observe a
// truncated or interleaved file: the data is written to a temporary file in
// the same directory, synced, and renamed over the target. Like os.WriteFile,
// an existing file keeps its permission bits and perm only applies to new
// files. A symlinked target is resolved so the link itself is preserved.
func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	if resolved, rerr := filepath.EvalSymlinks(path); rerr == nil {
		path = resolved
	}
	if info, serr := os.Stat(path); serr == nil {
		perm = info.Mode().Perm()
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return renameReplacing(tmpName, path)
}

// renameReplacing retries on Windows, where replacing a file fails with
// "Access is denied" while a lock-free reader still has it open.
func renameReplacing(from, to string) error {
	err := os.Rename(from, to)
	if runtime.GOOS != "windows" {
		return err
	}
	for delay := time.Millisecond; err != nil && errors.Is(err, fs.ErrPermission) && delay < time.Second; delay *= 2 {
		time.Sleep(delay)
		err = os.Rename(from, to)
	}
	return err
}

// loadJSONForUpdate reads and decodes path at the start of a read-modify-write
// cycle (the caller must hold the file's lock). A missing file is reported as
// found=false with a nil error.
//
// Any other failure is returned so the caller does not overwrite data it
// could not read. When resetCorrupt is set (pure caches whose content can be
// re-fetched from the server) a file that cannot be decoded is logged and
// treated as missing instead, so a cache corrupted by an older version heals
// itself on the next write. User data (contacts, drafts) must never use it.
func loadJSONForUpdate(path string, v any, resetCorrupt bool) (found bool, err error) {
	data, err := SecureReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(data, v); err != nil {
		if resetCorrupt && cacheResettable() {
			log.Printf("config: discarding unreadable cache %s: %v", path, err)
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// cacheResettable reports whether a cache that fails to decode may be
// rewritten from scratch. With secure mode on but the vault locked, the file
// is ciphertext we simply cannot read yet; replacing it would write plaintext
// over encrypted data.
func cacheResettable() bool {
	return !IsSecureModeEnabled() || GetSessionKey() != nil
}
