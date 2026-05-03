//go:build windows

package watch

import "syscall"

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

var watchProcessIsRunning = processExists

func processExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == stillActive
}
