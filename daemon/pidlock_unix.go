//go:build !windows

package daemon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	udsrpc "github.com/floatpane/go-uds-jsonrpc"
)

// A running daemon holds an exclusive lock on daemon.lock, next to the PID
// file, for as long as it lives, so a daemon that died without cleaning up
// never blocks the next one, and a PID the system has since given to an
// unrelated process never makes that process look like the daemon. The lock
// is on its own file because daemons from older versions delete daemon.pid
// when they exit, even after a newer daemon has taken it over. daemon.pid is
// still written, for older matcha binaries.

// lockPath returns the lock file that goes with the PID file at pidPath.
func lockPath(pidPath string) string {
	return filepath.Join(filepath.Dir(pidPath), "daemon.lock")
}

// LockPID makes this process the running daemon: it takes the daemon lock
// and records its PID in the lock file and in the PID file at pidPath.
// release drops the lock and removes the PID file if it still names this
// process.
func LockPID(pidPath string) (release func(), err error) {
	f, err := os.OpenFile(lockPath(pidPath), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			pid, _ := ReadPID(lockPath(pidPath))
			return nil, fmt.Errorf("daemon already running (PID %d)", pid)
		}
		return nil, fmt.Errorf("lock daemon: %w", err)
	}
	pid := strconv.Itoa(os.Getpid())
	if err := f.Truncate(0); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := f.WriteAt([]byte(pid), 0); err != nil {
		_ = f.Close()
		return nil, err
	}
	if err := os.WriteFile(pidPath, []byte(pid), 0o600); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("write PID file: %w", err)
	}
	return func() {
		if owner, err := ReadPID(pidPath); err == nil && owner == os.Getpid() {
			_ = os.Remove(pidPath)
		}
		// The lock file stays: removing it would let a daemon that opened
		// it just before lock a file nobody else can see.
		_ = f.Close()
	}, nil
}

// IsRunning reports whether a daemon is running, and its PID, given the PID
// file at pidPath. A running daemon holds the daemon lock. A daemon from an
// older version, which did not take it, counts as running only while its
// process is alive and its socket next to the PID file accepts connections.
func IsRunning(pidPath string) (int, bool) {
	if pid, locked := lockHolder(lockPath(pidPath)); locked {
		if pid == 0 {
			pid, _ = ReadPID(pidPath) // the holder is still writing its PID
		}
		return pid, true
	}
	pid, err := ReadPID(pidPath)
	if err != nil {
		return 0, false
	}
	if _, alive := udsrpc.IsRunning(pidPath); !alive {
		return pid, false
	}
	return pid, socketAccepts(filepath.Join(filepath.Dir(pidPath), "daemon.sock"))
}

// lockHolder reports whether some process holds the lock file at path, and
// the PID it recorded there.
func lockHolder(path string) (int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close() //nolint:errcheck
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
	if err == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		return 0, false
	}
	if !errors.Is(err, syscall.EWOULDBLOCK) {
		return 0, false
	}
	data := make([]byte, 32)
	n, _ := f.ReadAt(data, 0)
	pid, _ := strconv.Atoi(strings.TrimSpace(string(data[:n])))
	return pid, true
}

// socketAccepts reports whether something listens on the unix socket.
func socketAccepts(path string) bool {
	dialer := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := dialer.DialContext(context.Background(), "unix", path)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
