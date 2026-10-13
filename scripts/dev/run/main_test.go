package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/mertcikla/tld/v2/cmd/stop"
	"github.com/mertcikla/tld/v2/internal/localserver"
)

func TestDevWatcherProcess(t *testing.T) {
	ready := os.Getenv("TLD_TEST_DEV_WATCHER_READY")
	if ready == "" {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	if err := os.WriteFile(ready, nil, 0o600); err != nil {
		os.Exit(1)
	}
	<-signals
	// Match Vite's exit status after handling SIGTERM.
	os.Exit(128 + int(syscall.SIGTERM))
}

func TestDevWatchersStopAndCleanup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("development watchers use Unix shutdown signals")
	}
	for _, method := range []string{"tld stop", "interrupt"} {
		t.Run(method, func(t *testing.T) {
			t.Setenv("TLD_CONFIG_DIR", t.TempDir())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var watchers []*exec.Cmd
			var readyFiles []string
			for range 2 {
				ready := filepath.Join(t.TempDir(), "ready")
				watcher := exec.Command(os.Args[0], "-test.run=^TestDevWatcherProcess$")
				watcher.Env = append(os.Environ(), "TLD_TEST_DEV_WATCHER_READY="+ready)
				watchers = append(watchers, watcher)
				readyFiles = append(readyFiles, ready)
			}
			done := make(chan error, 1)
			go func() { done <- run(ctx, watchers) }()
			waitFor(t, func() bool {
				reg, err := localserver.LoadProcessRegistry()
				if err != nil || len(reg.Processes) != 2 {
					return false
				}
				for _, record := range reg.Processes {
					if record.Kind != "dev-process" || record.RepoRoot == "" {
						t.Fatalf("unexpected record: %+v", record)
					}
				}
				for _, ready := range readyFiles {
					if _, err := os.Stat(ready); err != nil {
						return false
					}
				}
				return true
			})
			if method == "tld stop" {
				cmd := stop.NewStopCmd()
				cmd.SetOut(io.Discard)
				cmd.SetErr(io.Discard)
				if err := cmd.Execute(); err != nil {
					t.Fatalf("tld stop: %v", err)
				}
			} else {
				cancel()
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("run: %v", err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("watchers did not stop")
			}
			reg, err := localserver.LoadProcessRegistry()
			if err != nil || len(reg.Processes) != 0 {
				t.Fatalf("registry after shutdown: %+v, %v", reg, err)
			}
		})
	}
}

func waitFor(t *testing.T, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("watchers did not become ready")
}

func TestStartupFailureCleansStartedWatcherAndPreservesOtherProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("development watchers use Unix shutdown signals")
	}
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	if err := localserver.RegisterProcess(localserver.ProcessRecord{Kind: "server", PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	watcher := exec.Command(os.Args[0], "-test.run=^TestDevWatcherProcess$")
	watcher.Env = append(os.Environ(), "TLD_TEST_DEV_WATCHER_READY="+filepath.Join(t.TempDir(), "ready"))
	missing := exec.Command(filepath.Join(t.TempDir(), "missing-watcher"))
	if err := run(context.Background(), []*exec.Cmd{watcher, missing}); err == nil {
		t.Fatal("expected startup failure")
	}
	if watcher.ProcessState == nil {
		t.Fatal("started watcher was not reaped")
	}
	reg, err := localserver.LoadProcessRegistry()
	if err != nil || len(reg.Processes) != 1 || reg.Processes[0].PID != os.Getpid() {
		t.Fatalf("unrelated process should be preserved: %+v, %v", reg, err)
	}
}
