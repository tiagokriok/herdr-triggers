//go:build !linux

package runtime

import "syscall"

func sysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{} }
