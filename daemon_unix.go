//go:build !windows

package main

import "syscall"

// daemonSysProcAttr returns SysProcAttr for detaching the daemon process.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true, // Create new session, detach from terminal.
	}
}
