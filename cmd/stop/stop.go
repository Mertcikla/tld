package stop

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/mertcikla/tld/internal/localserver"
	"github.com/mertcikla/tld/internal/term"
	"github.com/mertcikla/tld/internal/workspace"
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
			dataDirFlag, _ := cmd.Flags().GetString("data-dir")
			cfg, err := workspace.LoadGlobalConfig()
			if err != nil {
				return err
			}
			dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
			if err != nil {
				return err
			}
			return runStop(cmd, forceKill, dataDir)
		},
	}

	cmd.Flags().BoolVar(&forceKill, "kill", false, "force-stop with SIGKILL instead of SIGTERM")
	cmd.Flags().String("data-dir", "", "directory for database and logs (overrides config and env)")
	return cmd
}

func runStop(cmd *cobra.Command, forceKill bool, dataDir string) error {
	pidPath := localserver.PIDPath(dataDir)
	pid, err := localserver.ReadPID(pidPath)
	if err != nil {
		return fmt.Errorf("no server running")
	}

	if !localserver.IsRunning(pid) {
		_ = os.Remove(pidPath)
		return fmt.Errorf("no server running")
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
		term.Success(cmd.OutOrStdout(), "Server killed.")
		return nil
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal: %w", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !localserver.IsRunning(pid) {
			_ = os.Remove(pidPath)
			term.Success(cmd.OutOrStdout(), "Server stopped.")
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("server did not stop within 10s; use --kill to force")
}
