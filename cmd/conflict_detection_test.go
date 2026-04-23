package cmd_test

import (
	"github.com/mertcikla/diag/tld/cmd"
	"fmt"
	"strings"
	"testing"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/diag/tld/internal/workspace"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// conflictResponse builds an ApplyPlanResponse with one conflict item.
func conflictResponse(local, remote time.Time) *diagv1.ApplyPlanResponse {
	var localPb, remotePb *timestamppb.Timestamp
	if !local.IsZero() {
		localPb = timestamppb.New(local)
	}
	if !remote.IsZero() {
		remotePb = timestamppb.New(remote)
	}
	return &diagv1.ApplyPlanResponse{
		Conflicts: []*diagv1.PlanConflictItem{
			{
				ResourceType:    "diagram",
				Ref:             "sys",
				LocalUpdatedAt:  localPb,
				RemoteUpdatedAt: remotePb,
			},
		},
	}
}

// writeLockFile is a test helper that writes a minimal lock file in dir.
func writeLockFile(t *testing.T, dir string) {
	t.Helper()
	if err := workspace.WriteLockFile(dir, &workspace.LockFile{VersionID: "v1"}); err != nil {
		t.Fatalf("write lock file: %v", err)
	}
}

// --- Dry-run sequencing ---

// TestApplyCmd_DryRunFiredBeforeRealApply verifies that when a lock file exists and
// --force is NOT set, tld apply always performs a dry-run conflict check
// before the real apply, regardless of whether tld plan was run first.
func TestApplyCmd_DryRunFiredBeforeRealApply(t *testing.T) {
	var callLog []bool // true = dry run, false = real apply
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			isDry := req.DryRun != nil && *req.DryRun
			callLog = append(callLog, isDry)
			return cmd.SuccessResponse(req), nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	writeLockFile(t, dir)

	_, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("yes\n"), "apply")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	if len(callLog) != 2 {
		t.Fatalf("expected 2 ApplyPlan calls (dry-run + real), got %d", len(callLog))
	}
	if !callLog[0] {
		t.Error("first call should be dry-run (dry_run=true)")
	}
	if callLog[1] {
		t.Error("second call should be the real apply (dry_run=false)")
	}
}

// TestApplyCmd_DryRunNotFiredWhenNoLockFile verifies that conflict detection is
// skipped entirely when no lock file is present (e.g. first apply after init).
func TestApplyCmd_DryRunNotFiredWhenNoLockFile(t *testing.T) {
	var callCount int
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			callCount++
			if req.DryRun != nil && *req.DryRun {
				t.Error("dry-run should not be fired when there is no lock file")
			}
			return cmd.SuccessResponse(req), nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	// Deliberately no lock file written.

	_, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("yes\n"), "apply")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected exactly 1 ApplyPlan call (no dry-run), got %d", callCount)
	}
}

// TestApplyCmd_DryRunResetAfterConflictCheck verifies that after the dry-run conflict
// check, the real apply request is sent with dry_run unset (nil), so the server
// actually commits the changes and does not silently discard them.
func TestApplyCmd_DryRunResetAfterConflictCheck(t *testing.T) {
	var realReq *diagv1.ApplyPlanRequest
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			if req.DryRun == nil || !*req.DryRun {
				realReq = proto.Clone(req).(*diagv1.ApplyPlanRequest)
			}
			return cmd.SuccessResponse(req), nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	writeLockFile(t, dir)

	_, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("yes\n"), "apply")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	if realReq == nil {
		t.Fatal("real apply request was never sent")
	}
	if realReq.DryRun != nil && *realReq.DryRun {
		t.Error("real apply request must not have dry_run=true")
	}
}

// --- --force conflict detection ---

// TestApplyCmd_AutoApprovePerformsConflictDetection verifies that --force
// now performs a dry-run conflict check when a lock file is present.
func TestApplyCmd_AutoApprovePerformsConflictDetection(t *testing.T) {
	var dryRunCalled bool
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			if req.DryRun != nil && *req.DryRun {
				dryRunCalled = true
			}
			return cmd.SuccessResponse(req), nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	writeLockFile(t, dir) // lock file present, but --force should skip conflict check

	_, _, err := cmd.RunCmd(t, dir, "apply", "--force")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !dryRunCalled {
		t.Error("--force should perform dry-run conflict detection when a lock file exists")
	}
}

// --- Conflict output details ---

// TestApplyCmd_ConflictShowsTimestamps verifies that when the server returns a conflict
// with timestamps, the CLI prints both local and remote timestamps so the user can
// judge which is newer.
func TestApplyCmd_ConflictShowsTimestamps(t *testing.T) {
	local := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	remote := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC) // remote is 2h newer

	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			if req.DryRun != nil && *req.DryRun {
				return conflictResponse(local, remote), nil
			}
			return cmd.SuccessResponse(req), nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	writeLockFile(t, dir)

	_, stderr, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("1\n"), "apply")
	if err == nil {
		t.Fatal("expected error (user chose abort)")
	}

	if !strings.Contains(stderr, "2024-01-01T10:00:00Z") {
		t.Errorf("stderr should contain local timestamp, got: %q", stderr)
	}
	if !strings.Contains(stderr, "2024-01-01T12:00:00Z") {
		t.Errorf("stderr should contain remote timestamp, got: %q", stderr)
	}
}

// TestApplyCmd_ConflictShowsVersionInfo verifies that when the server returns a
// workspace version alongside conflict items, the version ID and creator are printed.
func TestApplyCmd_ConflictShowsVersionInfo(t *testing.T) {
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			if req.DryRun != nil && *req.DryRun {
				resp := conflictResponse(time.Time{}, time.Time{})
				resp.Version = &diagv1.WorkspaceVersion{
					VersionId: "frontend-v42",
					CreatedBy: "frontend",
					CreatedAt: timestamppb.New(time.Date(2024, 3, 28, 9, 0, 0, 0, time.UTC)),
				}
				return resp, nil
			}
			return cmd.SuccessResponse(req), nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	writeLockFile(t, dir)

	_, stderr, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("1\n"), "apply")
	if err == nil {
		t.Fatal("expected error (user chose abort)")
	}

	if !strings.Contains(stderr, "frontend-v42") {
		t.Errorf("stderr should contain remote version ID, got: %q", stderr)
	}
	if !strings.Contains(stderr, "frontend") {
		t.Errorf("stderr should contain remote version creator, got: %q", stderr)
	}
}

// TestApplyCmd_ConflictCountInOutput verifies that when multiple resources conflict,
// the count is reported so the user knows the scope of the conflict.
func TestApplyCmd_ConflictCountInOutput(t *testing.T) {
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			if req.DryRun != nil && *req.DryRun {
				return &diagv1.ApplyPlanResponse{
					Conflicts: []*diagv1.PlanConflictItem{
						{ResourceType: "diagram", Ref: "d1"},
						{ResourceType: "diagram", Ref: "d2"},
						{ResourceType: "object", Ref: "o1"},
					},
				}, nil
			}
			return cmd.SuccessResponse(req), nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	writeLockFile(t, dir)

	_, stderr, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("1\n"), "apply")
	if err == nil {
		t.Fatal("expected error (user chose abort)")
	}

	if !strings.Contains(stderr, "3 conflicts") {
		t.Errorf("stderr should report 3 conflicts, got: %q", stderr)
	}
}

// --- Conflict prompt choices ---

// TestApplyCmd_ConflictInvalidChoiceAborts verifies that an unrecognized input at
// the conflict prompt (including the unimplemented option 3) returns an error rather
// than silently proceeding with the apply.
func TestApplyCmd_ConflictInvalidChoiceAborts(t *testing.T) {
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			if req.DryRun != nil && *req.DryRun {
				return conflictResponse(time.Time{}, time.Time{}), nil
			}
			return cmd.SuccessResponse(req), nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	for _, choice := range []string{"3\n", "99\n", "x\n", "\n"} {
		t.Run(fmt.Sprintf("choice=%q", strings.TrimRight(choice, "\n")), func(t *testing.T) {
			dir := t.TempDir()
			cmd.SetupApplyWorkspace(t, dir, serverURL)
			writeLockFile(t, dir)

			_, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader(choice), "apply")
			if err == nil {
				t.Errorf("choice %q should return an error, but apply succeeded", choice)
			}

			svc.Mu.Lock()
			req := svc.LastRequest
			svc.Mu.Unlock()
			if req != nil && (req.DryRun == nil || !*req.DryRun) {
				t.Error("real apply should not have been called after invalid conflict choice")
			}
		})
	}
}

// TestApplyCmd_ConflictEOFAborts verifies that if stdin is closed before the user
// responds to the conflict prompt, the apply is safely aborted.
func TestApplyCmd_ConflictEOFAborts(t *testing.T) {
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			if req.DryRun != nil && *req.DryRun {
				return conflictResponse(time.Time{}, time.Time{}), nil
			}
			return cmd.SuccessResponse(req), nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	writeLockFile(t, dir)

	_, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader(""), "apply") // empty = EOF immediately
	if err == nil {
		t.Error("EOF on conflict prompt should return an error")
	}
}

// TestApplyCmd_NoConflictProceedsToConfirmation verifies that when the dry-run
// returns no conflicts, the apply proceeds directly to the normal "Apply N
// resources? [yes/no]" prompt without printing any conflict warning.
func TestApplyCmd_NoConflictProceedsToConfirmation(t *testing.T) {
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			return cmd.SuccessResponse(req), nil // no conflicts
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	writeLockFile(t, dir)

	stdout, stderr, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("yes\n"), "apply")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if strings.Contains(stderr, "conflict") {
		t.Errorf("no conflict warning expected, but stderr contains 'conflict': %q", stderr)
	}
	if !strings.Contains(stdout, "SUCCESS") {
		t.Errorf("expected SUCCESS in stdout: %q", stdout)
	}
}

// TestApplyCmd_ConflictServerErrorAborts verifies that if the dry-run conflict check
// itself fails (e.g. network error), the apply is aborted rather than proceeding
// blindly.
func TestApplyCmd_ConflictServerErrorAborts(t *testing.T) {
	svc := &cmd.MockDiagramService{
		ApplyFunc: func(req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
			if req.DryRun != nil && *req.DryRun {
				return nil, fmt.Errorf("network timeout")
			}
			return cmd.SuccessResponse(req), nil
		},
	}
	serverURL := cmd.NewMockServer(t, svc)

	dir := t.TempDir()
	cmd.SetupApplyWorkspace(t, dir, serverURL)
	writeLockFile(t, dir)

	_, _, err := cmd.RunCmdWithStdin(t, dir, strings.NewReader("yes\n"), "apply")
	if err == nil {
		t.Fatal("expected error when dry-run check fails")
	}
	if !strings.Contains(err.Error(), "server plan failed") {
		t.Errorf("error %q should contain 'server plan failed'", err.Error())
	}
}
