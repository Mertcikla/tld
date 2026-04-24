package cmdutil

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenBrowser opens the given URL in the default system browser.
func OpenBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd, args = "open", []string{url}
	case "windows":
		cmd, args = "cmd", []string{"/c", "start", url}
	default:
		cmd, args = "xdg-open", []string{url}
	}
	if err := exec.Command(cmd, args...).Start(); err != nil {
		return fmt.Errorf("open browser: %w", err)
	}
	return nil
}
