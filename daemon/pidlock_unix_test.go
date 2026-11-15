//go:build !windows

package daemon

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// shortTempDir returns a directory whose paths fit in a unix socket address.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mpid")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestStalePIDOfAnotherProcessIsNotRunning(t *testing.T) {
	// A daemon died without cleaning up, and the system gave its PID to an
	// unrelated (alive) process: here, the test itself.
	dir := shortTempDir(t)
	path := filepath.Join(dir, "daemon.pid")
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Create(filepath.Join(dir, "daemon.sock")); err != nil { // stale socket file
		t.Fatal(err)
	}
	if _, running := IsRunning(path); running {
		t.Fatal("a stale PID file naming an unrelated live process counts as a running daemon")
	}
	release, err := LockPID(path)
	if err != nil {
		t.Fatalf("a new daemon could not start over the stale PID file: %v", err)
	}
	release()
}

func TestOlderDaemonWithoutLockIsStillDetected(t *testing.T) {
	dir := shortTempDir(t)
	path := filepath.Join(dir, "daemon.pid")
	if err := WritePID(path); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", filepath.Join(dir, "daemon.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close() //nolint:errcheck
	if pid, running := IsRunning(path); !running || pid != os.Getpid() {
		t.Fatalf("IsRunning = %d, %v; a live daemon answering on its socket must count", pid, running)
	}
}
