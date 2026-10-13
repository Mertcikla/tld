// Command run starts the core development watchers and registers them for tld stop.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/mertcikla/tld/v2/internal/localserver"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	backend := exec.Command("air")
	backend.Env = append(os.Environ(), "DEV=true", "TLD_DATA_DIR=data/dev", "TLD_CONFIG_DIR=data/dev/config")
	// Run Vite directly: signaling npm alone can leave its Vite child running.
	// Taskfile runs npm's predev (icon preparation) before this launcher.
	frontend := exec.Command("node", "node_modules/vite/bin/vite.js", "--config", "vite.config.ts")
	frontend.Dir = "frontend"
	for _, watcher := range []*exec.Cmd{backend, frontend} {
		watcher.Stdin, watcher.Stdout, watcher.Stderr = os.Stdin, os.Stdout, os.Stderr
	}
	if err := run(ctx, []*exec.Cmd{backend, frontend}); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, watchers []*exec.Cmd) error {
	type runningWatcher struct {
		cmd  *exec.Cmd
		done chan error
	}
	var started []runningWatcher
	defer func() {
		// Let Air shut down its server before resorting to killing a watcher.
		for _, watcher := range started {
			_ = watcher.cmd.Process.Signal(syscall.SIGTERM)
		}
		deadline := time.NewTimer(8 * time.Second)
		defer deadline.Stop()
		for _, watcher := range started {
			select {
			case <-watcher.done:
			case <-deadline.C:
				for _, remaining := range started {
					_ = remaining.cmd.Process.Kill()
				}
				<-watcher.done
			}
			_ = localserver.RemoveProcess(watcher.cmd.Process.Pid)
		}
	}()

	exited := make(chan error, len(watchers))
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	for _, watcher := range watchers {
		if err := watcher.Start(); err != nil {
			return fmt.Errorf("start %s: %w", watcher.Path, err)
		}
		done := make(chan error, 1)
		started = append(started, runningWatcher{cmd: watcher, done: done})
		go func() {
			err := watcher.Wait()
			done <- err
			exited <- err
		}()
		if err := localserver.RegisterProcess(localserver.ProcessRecord{
			Kind: "dev-process", PID: watcher.Process.Pid, RepoRoot: root,
		}); err != nil {
			return fmt.Errorf("register %s: %w", watcher.Path, err)
		}
	}
	select {
	case <-ctx.Done():
		return nil
	case err := <-exited:
		// Vite exits with 128 + signal when stopped by tld stop or Ctrl-C.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && (exitErr.ExitCode() == 128+int(syscall.SIGTERM) || exitErr.ExitCode() == 128+int(syscall.SIGINT)) {
			return nil
		}
		return err
	}
}
