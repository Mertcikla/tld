//go:build windows

package stop

import (
	"os"
	"strings"
	"syscall"
)

func signalProcess(pid int, sig os.Signal) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	// On Windows, os.Process.Signal only supports os.Interrupt.
	// If SIGTERM was requested, we try os.Interrupt.
	if sig == syscall.SIGTERM {
		sig = os.Interrupt
	}

	err = proc.Signal(sig)
	if err != nil {
		// On Windows, signaling a process often fails if it's not a console process
		// or not in the same console. We ignore these errors to allow the
		// graceful shutdown wait loop to proceed (especially useful for 'watch'
		// which uses a database-based shutdown flag).
		errMsg := err.Error()
		if strings.Contains(errMsg, "not supported") || strings.Contains(errMsg, "invalid argument") {
			return nil
		}
		return err
	}
	return nil
}
