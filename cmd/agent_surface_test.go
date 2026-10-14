package cmd_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
)

func agentResult(t *testing.T, out string) cmdutil.JSONOutput {
	t.Helper()
	var result cmdutil.JSONOutput
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	return result
}

func TestAgentReadSameDestinationAndChooseCache(t *testing.T) {
	dir, dataDir := t.TempDir(), t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--target", "local", "--data-dir", dataDir)
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := exec.NewRunner(ws.Config, "local", dataDir, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runner.Close() }()
	newName := "Changed on target"
	if _, err := runner.UpdateElement(context.Background(), int32(ws.Meta.Elements["api"].ID), api.ElementInput{Name: newName}); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"list", "elements", "--target", "local", "--data-dir", dataDir},
		{"--target", "local", "--data-dir", dataDir, "list", "elements"},
		{"inspect", "api", "--target", "local", "--data-dir", dataDir, "--type", "element"},
		{"render", "root", "--target", "local", "--data-dir", dataDir},
	} {
		out, _ := cmd.MustRunCmd(t, dir, append(args, "--format", "json")...)
		if !strings.Contains(out, newName) {
			t.Fatalf("read did not select written destination: %v\n%s", args, out)
		}
	}
	out, _ := cmd.MustRunCmd(t, dir, "list", "elements", "--source", "cache", "--target", "local", "--data-dir", dataDir, "--format", "json")
	if strings.Contains(out, newName) || !strings.Contains(out, "API") {
		t.Fatalf("explicit cache source ignored: %s", out)
	}
	out, _ = cmd.MustRunCmd(t, dir, "context", "--target", "local", "--data-dir", dataDir, "--format", "json")
	info := agentResult(t, out).Extra
	if info["target"] != "local" || info["read_source"] != "live" || info["destination"] != filepath.Join(dataDir, "tld.db") {
		t.Fatalf("incorrect effective context: %s", out)
	}
	// Reading live must not refresh the cache as an undocumented side effect.
	cached, err := workspace.Load(dir)
	if err != nil || cached.Elements["api"].Name != "API" {
		t.Fatalf("live read modified cache: %v,%v", cached, err)
	}
}

func TestAgentContextDoesNotExposeCredentials(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	t.Setenv("TLD_SERVER_URL", "https://username:password@example.test/api?token=private-token")
	t.Setenv("TLD_API_KEY", "private-api-key")
	t.Setenv("TLD_ORG_ID", cmd.TestWorkspaceID)
	out, _ := cmd.MustRunCmd(t, dir, "context", "--format", "json")
	for _, secret := range []string{"username", "password", "private-token", "private-api-key"} {
		if strings.Contains(out, secret) {
			t.Fatalf("context exposed credentials: %s", out)
		}
	}
	if agentResult(t, out).Extra["destination"] != "https://example.test/api" {
		t.Fatalf("incorrect public destination: %s", out)
	}
}

func TestAgentRecoveryHintsLeadToWorkingCommands(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api")
	output := filepath.Join(t.TempDir(), "missing", "diagram.mmd")
	out, _, err := cmd.RunCmd(t, dir, "render", "root", "--output", output, "--format", "json")
	result := agentResult(t, out)
	if err == nil || result.Error.Code != "path_not_found" || !strings.Contains(result.Error.Hint, "output directory") {
		t.Fatalf("incorrect output-path recovery: %v\n%s", err, out)
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		t.Fatal(err)
	}
	cmd.MustRunCmd(t, dir, "render", "root", "--output", output, "--format", "json")
	out, _, err = cmd.RunCmd(t, dir, "update", "element", "api", "descriptionn", "Handles traffic", "--format", "json")
	result = agentResult(t, out)
	if err == nil || result.Error.Code != "unknown_field" || !strings.Contains(result.Error.Hint, "description field") {
		t.Fatalf("missing spelling recovery: %v\n%s", err, out)
	}
	cmd.MustRunCmd(t, dir, "update", "element", "api", "description", "Handles traffic")
}

func TestAgentPullAndPreviewJSON(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api")
	cmd.MustRunCmd(t, dir, "add", "Database", "--ref", "db")
	for _, args := range [][]string{
		{"pull", "--dry-run"}, {"pull"},
		{"connect", "--from", "api", "--to", "db", "--view", "root", "--label", "reads", "--dry-run"},
	} {
		out, stderr := cmd.MustRunCmd(t, dir, append(args, "--format", "json")...)
		result := agentResult(t, out)
		preview := args[len(args)-1] == "--dry-run"
		if result.Status != "ok" || result.DryRun != preview || stderr != "" {
			t.Fatalf("inconsistent preview result: %s stderr=%s", out, stderr)
		}
		if args[0] == "connect" && result.Items[0].Ref != "root/api~db/reads" {
			t.Fatalf("preview returned unusable key: %s", out)
		}
	}
	path := filepath.Join(dir, ".tld", "elements.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), "name: API", "name: Local API", 1)), 0600); err != nil {
		t.Fatal(err)
	}
	out, stderr, err := cmd.RunCmd(t, dir, "pull", "--format", "json")
	result := agentResult(t, out)
	if err == nil || result.Error.Code != "local_changes" || stderr != "" {
		t.Fatalf("JSON pull prompted or lost conflict: %v\n%s\n%s", err, out, stderr)
	}
	cmd.MustRunCmd(t, dir, "pull", "--dry-run", "--format", "json")
}
