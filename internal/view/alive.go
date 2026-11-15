package view

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func ProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func IsScheduler(pid int) bool {
	if !ProcessAlive(pid) {
		return false
	}
	cmdline, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return true
	}
	return strings.Contains(string(cmdline), "serve")
}
