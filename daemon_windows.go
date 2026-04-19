//go:build windows

package main

import "syscall"

// daemonSysProcAttr returns SysProcAttr for detaching the daemon process on Windows.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // DETACHED_PROCESS
	}
}
