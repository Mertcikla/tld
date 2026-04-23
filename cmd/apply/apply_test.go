package apply_test

import (
	"github.com/mertcikla/diag/tld/cmd"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/diag/tld/internal/planner"
	"github.com/mertcikla/diag/tld/internal/workspace"
)


func TestApplyCmd_SuccessAutoApprove(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	stdout, _, err := cmd.RunCmd(t, dir, "apply", "--force")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(stdout, "SUCCESS") || !strings.Contains(stdout, "## Planned vs Created") {
		t.Fatalf("unexpected output: %q", stdout)
	}
}

func TestApplyCmd_BearerTokenSentToServer(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)
	cmd.WriteConfig(t, dir, serverURL, "my-secret-key")

	if _, _, err := cmd.RunCmd(t, dir, "apply", "--force"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	svc.Mu.Lock()
	defer svc.Mu.Unlock()
	if svc.LastHeader.Get("Authorization") != "Bearer my-secret-key" {
		t.Fatalf("Authorization = %q", svc.LastHeader.Get("Authorization"))
	}
}

func TestApplyCmd_WorkspaceIDInRequest(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	if _, _, err := cmd.RunCmd(t, dir, "apply", "--force"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	svc.Mu.Lock()
	defer svc.Mu.Unlock()
	if svc.LastRequest.GetOrgId() != cmd.TestWorkspaceID {
		t.Fatalf("OrgId = %q", svc.LastRequest.GetOrgId())
	}
}

func TestApplyCmd_ServerError_CodeInternal(t *testing.T) {
	svc := &cmd.MockDiagramService{ApplyFunc: func(_ *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("server exploded"))
	}}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	_, stderr, err := cmd.RunCmd(t, dir, "apply", "--force")
	if err == nil || !strings.Contains(stderr, "Apply failed") {
		t.Fatalf("expected apply failure, stderr=%q err=%v", stderr, err)
	}
}

func TestApplyCmd_DriftDetected(t *testing.T) {
	svc := &cmd.MockDiagramService{ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
		resp := cmd.SuccessResponse(req)
		resp.Drift = []*diagv1.PlanDriftItem{{ResourceType: "element", Ref: "api", Reason: "name changed"}}
		return resp, nil
	}}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	_, stderr, err := cmd.RunCmd(t, dir, "apply", "--force")
	if err == nil || !strings.Contains(stderr, "drift item(s) detected") {
		t.Fatalf("expected drift failure, stderr=%q err=%v", stderr, err)
	}
}

func TestApplyCmd_PrefightDriftWarningAbort(t *testing.T) {
	svc := &cmd.MockDiagramService{ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
		if req.DryRun != nil && *req.DryRun {
			return &diagv1.ApplyPlanResponse{Drift: []*diagv1.PlanDriftItem{{ResourceType: "element", Ref: "api", Reason: "remote changed"}}}, nil
		}
		return cmd.SuccessResponse(req), nil
	}}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)
	hash, err := workspace.CalculateWorkspaceHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{VersionID: "v1", WorkspaceHash: hash}); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("no\n"), "apply")
	if err != nil {
		t.Fatalf("expected graceful cancel, got %v", err)
	}
	if !strings.Contains(stdout, "server has changes that are not in your local YAML") || !strings.Contains(stdout, "Apply cancelled.") {
		t.Fatalf("unexpected output: %q", stdout)
	}
	if svc.LastRequest == nil || svc.LastRequest.DryRun == nil || !*svc.LastRequest.DryRun {
		t.Fatalf("expected only dry-run preflight request, got %#v", svc.LastRequest)
	}
}

func TestApplyCmd_ForceApplySkipsPreflightDriftPrompt(t *testing.T) {
	svc := &cmd.MockDiagramService{ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
		if req.DryRun != nil && *req.DryRun {
			return &diagv1.ApplyPlanResponse{Drift: []*diagv1.PlanDriftItem{{ResourceType: "element", Ref: "api", Reason: "remote changed"}}}, nil
		}
		return cmd.SuccessResponse(req), nil
	}}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)
	hash, err := workspace.CalculateWorkspaceHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{VersionID: "v1", WorkspaceHash: hash}); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "apply", "--force", "--force-apply")
	if err != nil {
		t.Fatalf("expected apply success, stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}
	if strings.Contains(stdout, "server has changes that are not in your local YAML") {
		t.Fatalf("unexpected preflight prompt output: %q", stdout)
	}
}

func TestApplyCmd_InteractiveApprove(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	stdout, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("yes\n"), "apply")
	if err != nil || !strings.Contains(stdout, "SUCCESS") {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
}

func TestApplyCmd_InteractiveDecline(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	stdout, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("no\n"), "apply")
	if err != nil {
		t.Fatalf("apply with stdin no: %v", err)
	}
	if !strings.Contains(stdout, "Apply cancelled") {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestApplyCmd_MissingConfig(t *testing.T) {
	dir := t.TempDir()
	_, _, err := cmd.RunCmd(t, dir, "apply", "--force")
	if err == nil || !strings.Contains(err.Error(), "load workspace") {
		t.Fatalf("expected missing config error, got %v", err)
	}
}

func TestApplyCmd_CreatedResourcesInOutput(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	stdout, _, err := cmd.RunCmd(t, dir, "apply", "--force", "--verbose")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(stdout, "### Views") || !strings.Contains(stdout, "### Elements") || !strings.Contains(stdout, "### Connectors") {
		t.Fatalf("unexpected verbose output: %q", stdout)
	}
}

func TestApplyCmd_ConflictAbort(t *testing.T) {
	svc := &cmd.MockDiagramService{ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
		if req.DryRun != nil && *req.DryRun {
			return &diagv1.ApplyPlanResponse{Conflicts: []*diagv1.PlanConflictItem{{ResourceType: "element", Ref: "api"}}}, nil
		}
		return cmd.SuccessResponse(req), nil
	}}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{VersionID: "v1"}); err != nil {
		t.Fatal(err)
	}

	_, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("1\n"), "apply")
	if err == nil || !strings.Contains(err.Error(), "apply aborted by user") {
		t.Fatalf("expected abort error, got %v", err)
	}
}

func TestApplyCmd_ConflictForce(t *testing.T) {
	svc := &cmd.MockDiagramService{ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
		if req.DryRun != nil && *req.DryRun {
			return &diagv1.ApplyPlanResponse{Conflicts: []*diagv1.PlanConflictItem{{ResourceType: "element", Ref: "api"}}}, nil
		}
		return cmd.SuccessResponse(req), nil
	}}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{VersionID: "v1"}); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("2\nyes\n"), "apply")
	if err != nil || !strings.Contains(stdout, "SUCCESS") {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
}

func TestApplyCmd_JSONOutput(t *testing.T) {
	svc := &cmd.MockDiagramService{}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "apply", "--force", "--format", "json")
	if err != nil {
		t.Fatalf("apply --format json: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var payload planner.JSONOutput
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal json output: %v\nstdout=%s", err, stdout)
	}
	if payload.Command != "apply" || payload.Status != "ok" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Retries != 0 {
		t.Fatalf("unexpected retries: %+v", payload)
	}
}

func TestApplyCmd_JSONOutputIncludesRetryCount(t *testing.T) {
	var calls int
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			calls++
			if req.DryRun != nil && *req.DryRun {
				return &diagv1.ApplyPlanResponse{Conflicts: []*diagv1.PlanConflictItem{{ResourceType: "element", Ref: "api"}}}, nil
			}
			return cmd.SuccessResponse(req), nil
		},
		ExportFunc: func(_ *diagv1.ExportOrganizationRequest) (*diagv1.ExportOrganizationResponse, error) {
			return &diagv1.ExportOrganizationResponse{}, nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)
	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	cmd.SeedElementWorkspace(t, dir)
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{VersionID: "v1"}); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "apply", "--force", "--format", "json")
	if err != nil {
		t.Fatalf("apply --format json: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var payload planner.JSONOutput
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal json output: %v\nstdout=%s", err, stdout)
	}
	if payload.Retries != 1 {
		t.Fatalf("expected retries=1, got %+v", payload)
	}
	if calls < 2 {
		t.Fatalf("expected dry-run conflict check and real apply, got %d calls", calls)
	}
}
