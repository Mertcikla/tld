package crudsync

import (
	"context"
	"fmt"
	"io"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/cmd/apply"
	"github.com/mertcikla/tld/v2/internal/client"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/planner"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

// Options controls the synchronous apply behavior.
type Options struct {
	Target  string
	DataDir string
	Command string
	Format  string
	Compact bool
}

// Result carries the outcome of a synchronous apply for immediate feedback.
type Result struct {
	Runner   apply.Runner
	Response *diagv1.ApplyPlanResponse
	Plan     *planner.Plan
}

// ApplyAfterMutation preserves the legacy helper used by MCP tools.
// It delegates to ApplySynchronous with fail-fast semantics.
func ApplyAfterMutation(cmd *cobra.Command, wdir string, dataDir string) error {
	ctx := context.Background()
	if cmd != nil && cmd.Context() != nil {
		ctx = cmd.Context()
	}
	_, err := ApplySynchronous(ctx, wdir, Options{DataDir: dataDir})
	if err != nil {
		return err
	}
	return nil
}

// ApplySynchronous builds the workspace plan and applies it immediately,
// without interactive prompts. On remote drift/conflicts it fails fast with
// a `tld pull` hint instead of prompting.
func ApplySynchronous(ctx context.Context, wdir string, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ws, err := cmdutil.LoadWorkspace(wdir)
	if err != nil {
		return nil, err
	}
	if errs := ws.ValidateWithOpts(workspace.ValidationOptions{SkipSymbols: true}); len(errs) > 0 {
		return nil, fmt.Errorf("workspace has %d validation error(s): %v", len(errs), errs[0])
	}

	lockFile, err := workspace.LoadLockFile(wdir)
	if err != nil {
		return nil, fmt.Errorf("load lock file: %w", err)
	}
	meta, err := workspace.LoadMetadata(wdir)
	if err != nil {
		return nil, fmt.Errorf("load metadata: %w", err)
	}
	var previousMeta *workspace.Meta
	if lockFile != nil {
		previousMeta = lockFile.Metadata
	}
	runner, err := apply.NewRunner(ws.Config, opts.Target, opts.DataDir, false, previousMeta)
	if err != nil {
		return nil, err
	}
	if runner.Name() == apply.TargetRemote {
		if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
			return nil, err
		}
	}
	repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), wdir)
	if repoCtx.Name != "" && repoCtx.MatchesWorkspaceRepo(ws) {
		ws.ActiveRepo = repoCtx.Name
	}
	plan, err := planner.Build(ws, false)
	if err != nil {
		return nil, fmt.Errorf("build plan: %w", err)
	}

	// Fail fast on remote drift/conflicts: single dry-run, no prompts.
	if runner.SupportsDryRun() {
		c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
		dryReq := proto.Clone(plan.Request).(*diagv1.ApplyPlanRequest)
		dryReq.DryRun = new(true)
		dryResp, err := c.ApplyWorkspacePlan(ctx, connect.NewRequest(dryReq))
		if err != nil {
			return nil, cmdutil.WithUnauthorizedHint("server plan failed", err)
		}
		if len(dryResp.Msg.GetConflicts()) > 0 {
			return nil, fmt.Errorf("version conflict detected (%d conflict(s)): server has newer changes. Run `tld pull` to merge first (legacy: `tld apply --force`)", len(dryResp.Msg.GetConflicts()))
		}
		if len(dryResp.Msg.GetDrift()) > 0 {
			return nil, fmt.Errorf("server has changes that are not in your local YAML (%d drift item(s)). Run `tld pull` to merge first (legacy: `tld apply --force-apply` to overwrite)", len(dryResp.Msg.GetDrift()))
		}
	}

	resp, err := runner.ApplyWorkspacePlan(ctx, plan.Request)
	if err != nil {
		return nil, cmdutil.WithUnauthorizedHint("auto-apply failed", err)
	}
	if len(resp.GetDrift()) > 0 {
		return nil, fmt.Errorf("%d drift item(s) detected after apply", len(resp.GetDrift()))
	}

	currentWS := ws
	renames, err := apply.ApplyCanonicalRefs(wdir, resp)
	if err != nil {
		return nil, fmt.Errorf("apply canonical refs: %w", err)
	}
	_ = renames
	if len(renames) > 0 {
		currentWS, err = workspace.Load(wdir)
		if err != nil {
			return nil, fmt.Errorf("reload workspace after canonical ref rename: %w", err)
		}
	}
	if err := apply.ApplyViewNames(ctx, runner, currentWS, plan, resp); err != nil {
		return nil, fmt.Errorf("apply view names: %w", err)
	}
	if err := apply.UpdatePlanMetadataFromResponse(wdir, meta, currentWS, plan, resp); err != nil {
		return nil, fmt.Errorf("update metadata: %w", err)
	}
	if err := apply.UpdateLockFileFromResponse(wdir, lockFile, currentWS, meta, resp); err != nil {
		return nil, fmt.Errorf("update lock file: %w", err)
	}
	currentWS.Meta = meta
	if err := workspace.Save(currentWS); err != nil {
		return nil, fmt.Errorf("save workspace metadata: %w", err)
	}
	return &Result{Runner: runner, Response: resp, Plan: plan}, nil
}

// RenderResult prints immediate synchronous feedback in text mode.
func RenderResult(out io.Writer, runner apply.Runner, resp *diagv1.ApplyPlanResponse) {
	summary := resp.GetSummary()
	var elements, views, connectors int64
	if summary != nil {
		elements = int64(summary.GetElementsCreated())
		views = int64(summary.GetViewsCreated())
		connectors = int64(summary.GetConnectorsCreated())
	} else {
		elements = int64(len(resp.GetCreatedElements()))
		views = int64(len(resp.GetCreatedViews()))
		connectors = int64(len(resp.GetCreatedConnectors()))
	}
	term.Successf(out, "applied: %d elements, %d diagrams, %d connectors", elements, views, connectors)
	apply.RenderTargetInfo(out, runner)
	apply.RenderPostApplyLocation(out, runner)
}

// SyncAndReport runs ApplySynchronous and writes text or JSON feedback.
// On failure the YAML is already written; the error is wrapped to make that clear.
func SyncAndReport(cmd *cobra.Command, wdir string, opts Options, out io.Writer) (*Result, error) {
	ctx := context.Background()
	if cmd != nil && cmd.Context() != nil {
		ctx = cmd.Context()
	}
	result, err := ApplySynchronous(ctx, wdir, opts)
	if err != nil {
		detail := strings.TrimSpace(err.Error())
		_ = detail
		return nil, fmt.Errorf("workspace YAML was updated, but auto-apply failed: %w", err)
	}
	if cmdutil.WantsJSON(opts.Format) {
		command := opts.Command
		if command == "" {
			command = "apply"
		}
		ws, loadErr := cmdutil.LoadWorkspace(wdir)
		if loadErr != nil {
			return result, fmt.Errorf("workspace YAML was updated and applied, but feedback failed: %w", loadErr)
		}
		return result, cmdutil.WriteJSON(out, opts.Compact, cmdutil.BuildSyncJSON(command, ws, result.Response))
	}
	RenderResult(out, result.Runner, result.Response)
	return result, nil
}
