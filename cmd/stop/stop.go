package stop

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	watchpkg "github.com/mertcikla/tld/v2/internal/watch"
	"github.com/spf13/cobra"
)

var (
	processIsRunning = localserver.IsRunning
	signalPID        = signalProcess
	killPID          = killProcess
	stopSleep        = time.Sleep
)

func NewStopCmd() *cobra.Command {
	var forceKill bool

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop local tlDiagram processes",
		Long: `Stop local tlDiagram processes started by 'tld serve' or 'tld watch'.

Process state is read from the global tld process registry.
Sends graceful stop requests and waits up to 10 seconds for shutdown.
Use --kill to send SIGKILL immediately to all registered processes.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStop(cmd, forceKill)
		},
	}

	cmd.Flags().BoolVar(&forceKill, "kill", false, "force-stop with SIGKILL instead of graceful shutdown")
	cmd.Flags().String("data-dir", "", "deprecated; process registry is global")
	return cmd
}

func runStop(cmd *cobra.Command, forceKill bool) error {
	reg, err := localserver.LoadProcessRegistry()
	if err != nil {
		return err
	}

	live := make([]localserver.ProcessRecord, 0, len(reg.Processes))
	for _, proc := range reg.Processes {
		if processIsRunning(proc.PID) {
			live = append(live, proc)
			continue
		}
		_ = localserver.RemoveProcess(proc.PID)
	}
	if len(live) == 0 {
		return fmt.Errorf("no tld processes running")
	}

	if forceKill {
		var errs []string
		for _, proc := range live {
			if err := killPID(proc.PID); err != nil {
				errs = append(errs, fmt.Sprintf("%s pid %d: %v", proc.Kind, proc.PID, err))
				continue
			}
			_ = localserver.RemoveProcess(proc.PID)
			term.Success(cmd.OutOrStdout(), fmt.Sprintf("%s killed (pid %d).", printableKind(proc.Kind), proc.PID))
		}
		if len(errs) > 0 {
			return fmt.Errorf("kill failed: %s", strings.Join(errs, "; "))
		}
		return nil
	}

	var signalErrs []string
	for _, proc := range live {
		if proc.Kind == localserver.ProcessKindWatch {
			_ = requestWatchStop(proc)
		}
		if err := signalPID(proc.PID, syscall.SIGTERM); err != nil {
			if !processIsRunning(proc.PID) {
				_ = localserver.RemoveProcess(proc.PID)
				continue
			}
			signalErrs = append(signalErrs, fmt.Sprintf("%s pid %d: %v", proc.Kind, proc.PID, err))
		}
	}
	if len(signalErrs) > 0 {
		return fmt.Errorf("signal failed: %s", strings.Join(signalErrs, "; "))
	}

	deadline := time.Now().Add(10 * time.Second)
	remaining := live
	for time.Now().Before(deadline) {
		next := remaining[:0]
		for _, proc := range remaining {
			if processIsRunning(proc.PID) {
				next = append(next, proc)
				continue
			}
			_ = localserver.RemoveProcess(proc.PID)
			term.Success(cmd.OutOrStdout(), fmt.Sprintf("%s stopped (pid %d).", printableKind(proc.Kind), proc.PID))
		}
		if len(next) == 0 {
			return nil
		}
		remaining = next
		stopSleep(200 * time.Millisecond)
	}

	return fmt.Errorf("%d tld process(es) did not stop within 10s; use --kill to force", len(remaining))
}

func requestWatchStop(proc localserver.ProcessRecord) error {
	if proc.DataDir == "" {
		return nil
	}
	sqliteStore, err := store.Open(localserver.DatabasePath(proc.DataDir), assets.FS)
	if err != nil {
		return err
	}
	defer func() { _ = sqliteStore.DB().Close() }()
	return watchpkg.NewStore(sqliteStore.DB()).RequestStopActive(context.Background())
}

func printableKind(kind string) string {
	switch kind {
	case localserver.ProcessKindServer:
		return "Server"
	case localserver.ProcessKindWatch:
		return "Watch"
	default:
		return "Process"
	}
}

func killProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	return proc.Kill()
}
