package stop

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/mertcikla/tld/internal/localserver"
	"github.com/spf13/cobra"
)

func NewStopCmd() *cobra.Command {
	var forceKill bool

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the local tlDiagram web server",
		Long: `Stop the tlDiagram web server started with 'tld serve'.

Sends SIGTERM and waits up to 10 seconds for a graceful shutdown.
Use --kill to send SIGKILL immediately.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStop(cmd, forceKill)
		},
	}

	cmd.Flags().BoolVar(&forceKill, "kill", false, "force-stop with SIGKILL instead of SIGTERM")
	return cmd
}

func runStop(cmd *cobra.Command, forceKill bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	pidPath := localserver.PIDPath(cwd)
	pid, err := localserver.ReadPID(pidPath)
	if err != nil {
		return fmt.Errorf("no server found (could not read %s)", pidPath)
	}

	if !localserver.IsRunning(pid) {
		_ = os.Remove(pidPath)
		return fmt.Errorf("no server running (stale pid file removed)")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	if forceKill {
		if err := proc.Kill(); err != nil {
			return fmt.Errorf("kill: %w", err)
		}
		_ = os.Remove(pidPath)
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Server killed.")
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal: %w", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !localserver.IsRunning(pid) {
			_ = os.Remove(pidPath)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Server stopped.")
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("server did not stop within 10s; use --kill to force")
}
