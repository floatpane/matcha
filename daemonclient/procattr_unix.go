//go:build !windows

package daemonclient

import "syscall"

// DaemonProcAttr returns SysProcAttr for detaching the daemon process.
func DaemonProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
