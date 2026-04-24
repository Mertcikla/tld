package localserver

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func PIDPath(dataDir string) string {
	return filepath.Join(dataDir, "tld.pid")
}

func LogPath(dataDir string) string {
	return filepath.Join(dataDir, "tld.log")
}

func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func WritePID(path string, pid int) error {
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

// IsRunning returns true if a process with the given PID exists and is alive.
func IsRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
