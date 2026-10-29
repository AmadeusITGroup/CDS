//go:build !windows

package bootstrap

import "syscall"

// detachedProcAttr makes the agent independent from the CLI process group.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
