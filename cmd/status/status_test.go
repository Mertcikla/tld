package status_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/localserver"
)

func TestStatusCmdReportsNoRunningProcesses(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())

	stdout, stderr, err := cmd.RunCmd(t, t.TempDir(), "status")
	if err != nil {
		t.Fatalf("status: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "No tld processes running.") {
		t.Fatalf("missing no-process output: %s", stdout)
	}
}

func TestStatusCmdReportsRegisteredProcesses(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "tld.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := localserver.SaveProcessRegistry(localserver.ProcessRegistry{Processes: []localserver.ProcessRecord{
		{Kind: localserver.ProcessKindServer, PID: os.Getpid(), DataDir: dataDir, Addr: "127.0.0.1:1", StartedAt: "2026-05-18T00:00:00Z"},
		{Kind: localserver.ProcessKindWatch, PID: os.Getpid(), DataDir: dataDir, RepoRoot: "/repo", RepositoryID: 42},
	}}); err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	stdout, stderr, err := cmd.RunCmd(t, t.TempDir(), "status")
	if err != nil {
		t.Fatalf("status: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	for _, want := range []string{"Server:", "Watch:", "PID:", "URL:", "Ready:", "no", "Repo:", "/repo", "Repository ID:", "42", "DB size:"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("status output missing %q:\n%s", want, stdout)
		}
	}
}

func TestStatusCmdJSONOutput(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	if err := localserver.SaveProcessRegistry(localserver.ProcessRegistry{Processes: []localserver.ProcessRecord{
		{Kind: localserver.ProcessKindServer, PID: os.Getpid(), Addr: "127.0.0.1:1"},
	}}); err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	stdout, stderr, err := cmd.RunCmd(t, t.TempDir(), "status", "--format", "json")
	if err != nil {
		t.Fatalf("status --format json: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var payload struct {
		Command   string `json:"command"`
		Status    string `json:"status"`
		Processes []struct {
			Kind  string `json:"kind"`
			Ready *bool  `json:"ready"`
		} `json:"processes"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal json output: %v\nstdout=%s", err, stdout)
	}
	if payload.Command != "status" || payload.Status != "running" || len(payload.Processes) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Processes[0].Kind != localserver.ProcessKindServer || payload.Processes[0].Ready == nil || *payload.Processes[0].Ready {
		t.Fatalf("unexpected process payload: %+v", payload.Processes[0])
	}
}
