package stop

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mertcikla/tld/v2/internal/localserver"
)

func TestStopCmdReportsNoTLDProcessesForEmptyRegistry(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	cmd := NewStopCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "no tld processes running") {
		t.Fatalf("err = %v, want no tld processes running", err)
	}
}

func TestStopCmdGracefullyStopsAllRegisteredProcesses(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	if err := localserver.SaveProcessRegistry(localserver.ProcessRegistry{Processes: []localserver.ProcessRecord{
		{Kind: localserver.ProcessKindServer, PID: 101, Addr: "127.0.0.1:8060"},
		{Kind: localserver.ProcessKindServer, PID: 202, RepoRoot: "/repo"},
	}}); err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	restore := stubProcessControl(map[int]bool{101: true, 202: true}, nil)
	defer restore()

	cmd := NewStopCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("stop: %v", err)
	}

	wantSignals := []int{101, 202}
	if !reflect.DeepEqual(recordedSignals, wantSignals) {
		t.Fatalf("signals = %v, want %v", recordedSignals, wantSignals)
	}
	reg, err := localserver.LoadProcessRegistry()
	if err != nil {
		t.Fatalf("LoadProcessRegistry: %v", err)
	}
	if len(reg.Processes) != 0 {
		t.Fatalf("registry should be empty after stop, got %+v", reg.Processes)
	}
	if strings.Count(out.String(), "Server stopped") != 2 {
		t.Fatalf("missing stop output: %q", out.String())
	}
}

func TestStopCmdKillStopsAllRegisteredProcesses(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	if err := localserver.SaveProcessRegistry(localserver.ProcessRegistry{Processes: []localserver.ProcessRecord{
		{Kind: localserver.ProcessKindServer, PID: 303},
		{Kind: localserver.ProcessKindServer, PID: 404},
	}}); err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	restore := stubProcessControl(map[int]bool{303: true, 404: true}, nil)
	defer restore()

	cmd := NewStopCmd()
	cmd.SetArgs([]string{"--kill"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("stop --kill: %v", err)
	}
	wantKills := []int{303, 404}
	if !reflect.DeepEqual(recordedKills, wantKills) {
		t.Fatalf("kills = %v, want %v", recordedKills, wantKills)
	}
}

var (
	recordedSignals []int
	recordedKills   []int
)

func stubProcessControl(running map[int]bool, signalErr error) func() {
	oldRunning := processIsRunning
	oldSignal := signalPID
	oldKill := killPID
	oldSleep := stopSleep
	recordedSignals = nil
	recordedKills = nil

	processIsRunning = func(pid int) bool {
		return running[pid]
	}
	signalPID = func(pid int, sig os.Signal) error {
		if sig != syscall.SIGTERM {
			return signalErr
		}
		recordedSignals = append(recordedSignals, pid)
		running[pid] = false
		return signalErr
	}
	killPID = func(pid int) error {
		recordedKills = append(recordedKills, pid)
		running[pid] = false
		return nil
	}
	stopSleep = func(time.Duration) {}

	return func() {
		processIsRunning = oldRunning
		signalPID = oldSignal
		killPID = oldKill
		stopSleep = oldSleep
		recordedSignals = nil
		recordedKills = nil
	}
}
