//go:build !windows

package daemon

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	udsrpc "github.com/floatpane/go-uds-jsonrpc"
)

// LockPID makes this process the running daemon: it takes an exclusive lock
// on the PID file at path and writes its PID there. The lock lasts as long
// as the process, so a daemon that died without cleaning up never blocks
// the next one. release removes the file and drops the lock.
func LockPID(path string) (release func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			pid, _ := ReadPID(path)
			return nil, fmt.Errorf("daemon already running (PID %d)", pid)
		}
		return nil, fmt.Errorf("lock PID file: %w", err)
	}
	if err := f.Truncate(0); err != nil {
		_ = f.Close()
		return nil, err
	}
	if _, err := f.WriteAt([]byte(strconv.Itoa(os.Getpid())), 0); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = os.Remove(path)
		_ = f.Close()
	}, nil
}

// IsRunning reports whether a daemon is running, and its PID, from the PID
// file at path. A running daemon holds the file's lock. A process that
// merely has the recorded PID is not enough: the system reuses PIDs, so a
// stale file can name an unrelated process (which must neither block the
// daemon from starting nor be signalled by "daemon stop"). A daemon from an
// older version, which did not take the lock, counts as running only while
// its process is alive and its socket next to the PID file accepts
// connections.
func IsRunning(path string) (int, bool) {
	pid, err := ReadPID(path)
	if err != nil {
		return 0, false
	}
	f, err := os.Open(path)
	if err != nil {
		return pid, false
	}
	defer f.Close() //nolint:errcheck
	switch err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); {
	case errors.Is(err, syscall.EWOULDBLOCK):
		return pid, true
	case err == nil:
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	}
	if _, alive := udsrpc.IsRunning(path); !alive {
		return pid, false
	}
	return pid, socketAccepts(filepath.Join(filepath.Dir(path), "daemon.sock"))
}

// socketAccepts reports whether something listens on the unix socket.
func socketAccepts(path string) bool {
	conn, err := net.DialTimeout("unix", path, 500*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
