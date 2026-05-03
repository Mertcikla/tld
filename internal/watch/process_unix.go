//go:build !windows

package watch

import (
	"os"
	"syscall"
)

var watchProcessIsRunning = processExists

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
