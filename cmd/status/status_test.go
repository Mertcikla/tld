package status_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mertcikla/tld/cmd"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/workspace"
)

func TestStatusCmd_Clean(t *testing.T) {
	dir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	cmd.MustInitWorkspace(t, dir)

	hash, err := workspace.CalculateWorkspaceHash(dir)
	if err != nil {
		t.Fatalf("CalculateWorkspaceHash: %v", err)
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{
		Version:       "v1",
		VersionID:     "version-1",
		LastApply:     time.Now(),
		AppliedBy:     "cli",
		Resources:     &workspace.ResourceCounts{},
		WorkspaceHash: hash,
	}); err != nil {
		t.Fatalf("WriteLockFile: %v", err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "status")
	if err != nil {
		t.Fatalf("status: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "IN SYNC") {
		t.Fatalf("missing IN SYNC header:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Local changes:") {
		t.Fatalf("missing clean status detail:\n%s", stdout)
	}
}

func TestStatusCmd_Modified(t *testing.T) {
	dir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	cmd.MustInitWorkspace(t, dir)

	if err := workspace.WriteLockFile(dir, &workspace.LockFile{
		Version:       "v1",
		VersionID:     "version-1",
		LastApply:     time.Now(),
		AppliedBy:     "cli",
		Resources:     &workspace.ResourceCounts{},
		WorkspaceHash: "sha256:stale",
	}); err != nil {
		t.Fatalf("WriteLockFile: %v", err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "status")
	if err != nil {
		t.Fatalf("status: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "MODIFIED") {
		t.Fatalf("missing MODIFIED header:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Local changes:") {
		t.Fatalf("missing modified detail:\n%s", stdout)
	}
}

func TestStatusCmd_NoLockFile(t *testing.T) {
	dir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	cmd.MustInitWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "status")
	if err != nil {
		t.Fatalf("status: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "No sync history found.") {
		t.Fatalf("missing no-lock message:\n%s", stdout)
	}
}

func TestStatusCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	cmd.MustInitWorkspace(t, dir)

	hash, err := workspace.CalculateWorkspaceHash(dir)
	if err != nil {
		t.Fatalf("CalculateWorkspaceHash: %v", err)
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{
		Version:       "v1",
		VersionID:     "version-1",
		LastApply:     time.Now(),
		AppliedBy:     "cli",
		Resources:     &workspace.ResourceCounts{},
		WorkspaceHash: hash,
	}); err != nil {
		t.Fatalf("WriteLockFile: %v", err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "status", "--format", "json")
	if err != nil {
		t.Fatalf("status --format json: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var payload planner.JSONOutput
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal json output: %v\nstdout=%s", err, stdout)
	}
	if payload.Command != "status" || payload.Status != "in_sync" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestStatusCmd_ConflictCount(t *testing.T) {
	dir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	cmd.MustInitWorkspace(t, dir)

	if err := workspace.WriteMetadataSection(dir, "elements.yaml", "_meta_elements", map[string]*workspace.ResourceMetadata{
		"api": {Conflict: true},
		"db":  {Conflict: true},
	}); err != nil {
		t.Fatalf("WriteMetadataSection: %v", err)
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{VersionID: "version-1", LastApply: time.Now()}); err != nil {
		t.Fatalf("WriteLockFile: %v", err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "status")
	if err != nil {
		t.Fatalf("status: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Merge conflicts:") || !strings.Contains(stdout, "2") {
		t.Fatalf("missing conflict count: %s", stdout)
	}
}

func TestStatusCmd_CheckServer_InSync(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)
	hash, err := workspace.CalculateWorkspaceHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{VersionID: "v1", WorkspaceHash: hash, LastApply: time.Now()}); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "status", "--check-server")
	if err != nil {
		t.Fatalf("status --check-server: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "Server state:") || !strings.Contains(stdout, "In sync") {
		t.Fatalf("missing in-sync server output: %s", stdout)
	}
}

func TestStatusCmd_CheckServer_Drifted(t *testing.T) {
	svc := &cmd.MockDiagramService{ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
		resp := cmd.SuccessResponse(req)
		resp.Drift = []*diagv1.PlanDriftItem{{ResourceType: "element", Ref: "api", Reason: "server changed"}}
		return resp, nil
	}}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)
	hash, err := workspace.CalculateWorkspaceHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{VersionID: "v1", WorkspaceHash: hash, LastApply: time.Now()}); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "status", "--check-server")
	if err != nil {
		t.Fatalf("status --check-server: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "1 drift items found") {
		t.Fatalf("missing drift output: %s", stdout)
	}
}
