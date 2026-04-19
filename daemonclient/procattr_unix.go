//go:build !windows

package daemonclient

import "syscall"

func daemonProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
