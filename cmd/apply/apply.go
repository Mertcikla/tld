package apply

import (
	"bufio"
	"context"

	"errors"
	"fmt"
	"strings"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/internal/client"
	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/reporter"
	"github.com/mertcikla/tld/internal/term"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

func NewApplyCmd(wdir *string) *cobra.Command {
	var force bool
	var debug bool
	var verbose bool
	var recreateIDs bool
	var forceApply bool

	c := &cobra.Command{
		Use:   "apply",
		Short: "Apply plan to the tldiagram.com",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "apply", err)
				}
				return err
			}
			if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
				return err
			}
			if errs := ws.ValidateWithOpts(workspace.ValidationOptions{SkipSymbols: true}); len(errs) > 0 {
				for _, e := range errs {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "validation error: %s\n", e)
				}
				return fmt.Errorf("workspace has %d validation error(s)", len(errs))
			}

			// Load lock file and metadata for conflict detection
			lockFile, err := workspace.LoadLockFile(*wdir)
			if err != nil {
				return fmt.Errorf("load lock file: %w", err)
			}

			meta, err := workspace.LoadMetadata(*wdir)
			if err != nil {
				return fmt.Errorf("load metadata: %w", err)
			}
			repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), *wdir)
			if repoCtx.Name != "" && repoCtx.MatchesWorkspaceRepo(ws) {
				ws.ActiveRepo = repoCtx.Name
			}

			plan, err := planner.Build(ws, recreateIDs)
			if err != nil {
				return fmt.Errorf("build plan: %w", err)
			}
			retries := 0

			req := plan.Request
			diagramCount := 0
			for _, element := range req.Elements {
				if element.GetHasView() {
					diagramCount++
				}
			}
			total := len(req.Elements) + diagramCount + len(req.Connectors)
			if !cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Plan: %d elements, %d diagrams, %d connectors (%d total resources)\n",
					len(req.Elements), diagramCount, len(req.Connectors), total)
			}

			// Check for version conflicts if lock file exists
			scanner := bufio.NewScanner(cmd.InOrStdin())
			if lockFile != nil && !force && !forceApply {
				currentHash, hashErr := workspace.CalculateWorkspaceHash(*wdir)
				if hashErr == nil && lockFile.WorkspaceHash != "" && currentHash == lockFile.WorkspaceHash {
					hasDrift, err := serverHasDrift(cmd.Context(), ws, plan)
					if err != nil {
						return fmt.Errorf("server drift check failed: %w", err)
					}
					if hasDrift {
						term.Warn(cmd.OutOrStdout(), "The server has changes that are not in your local YAML.")
						term.Hint(cmd.OutOrStdout(), "Run `tld pull` to merge them first, or use --force-apply to overwrite.")
						_, _ = fmt.Fprint(cmd.OutOrStdout(), "  Continue anyway? [yes/no]: ")
						if !scanner.Scan() {
							return errors.New("aborted")
						}
						answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
						if answer != "yes" && answer != "y" {
							term.Info(cmd.OutOrStdout(), "Apply cancelled.")
							return nil
						}
					}
				}
			}
			if lockFile != nil && !force {
				newPlan, err := detectAndHandleConflicts(cmd, ws, lockFile, meta, plan, scanner, *wdir, recreateIDs)
				if err != nil {
					return err
				}
				if newPlan != nil {
					plan = newPlan
					req = plan.Request
				}
			}
			if lockFile != nil && force {
				newPlan, retryCount, err := autoPullAndRebuild(cmd, ws, lockFile, plan, *wdir, recreateIDs)
				if err != nil {
					if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "apply", err)
					}
					return err
				}
				if newPlan != nil {
					plan = newPlan
					req = plan.Request
					retries = retryCount
				}
			}

			if !force {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  Apply %d resources? [yes/no]: ", total)
				if !scanner.Scan() {
					return errors.New("aborted")
				}
				answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
				if answer != "yes" && answer != "y" {
					term.Info(cmd.OutOrStdout(), "Apply cancelled.")
					return nil
				}
			}

			c := client.New(ws.Config.ServerURL, ws.Config.APIKey, debug)
			resp, err := c.ApplyWorkspacePlan(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "apply", err)
				}
				term.Fail(cmd.ErrOrStderr(), fmt.Sprintf("Apply failed: %v", err))
				term.Label(cmd.ErrOrStderr(), 12, "Target URL", client.NormalizeURL(ws.Config.ServerURL))

				if connectErr := new(connect.Error); errors.As(err, &connectErr) {
					term.Label(cmd.ErrOrStderr(), 12, "Code", connectErr.Code().String())
					if len(connectErr.Details()) > 0 {
						_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "  Details:")
						for _, detail := range connectErr.Details() {
							_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "    - %v\n", detail)
						}
					}
				}

				term.Info(cmd.ErrOrStderr(), "Transaction rolled back.")
				reporter.RenderExecutionMarkdown(cmd.ErrOrStderr(), plan, nil, false, false)
				return cmdutil.WithUnauthorizedHint("apply failed", err)
			}

			currentWS := ws
			renames, err := applyCanonicalRefs(*wdir, resp.Msg)
			if err != nil {
				return fmt.Errorf("apply canonical refs: %w", err)
			}
			if len(renames) > 0 {
				currentWS, err = workspace.Load(*wdir)
				if err != nil {
					return fmt.Errorf("reload workspace after canonical ref rename: %w", err)
				}
				if !cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					for _, rename := range renames {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  ref renamed: %s -> %s\n", rename.from, rename.to)
					}
				}
			}

			if err := updatePlanMetadataFromResponse(*wdir, meta, currentWS, plan, resp.Msg); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update metadata: %v\n", err)
			}

			if err := updateLockFileFromResponse(*wdir, lockFile, currentWS, meta, resp.Msg); err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning: failed to update lock file: %v\n", err)
			}
			currentWS.Meta = meta
			if err := workspace.Save(currentWS); err != nil {
				term.Warnf(cmd.ErrOrStderr(), "Failed to rewrite workspace metadata: %v", err)
			}

			if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
				return cmdutil.WriteJSON(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", cmdutil.BuildApplyJSON(currentWS, resp.Msg, retries))
			}

			reporter.RenderExecutionMarkdown(cmd.OutOrStdout(), plan, resp.Msg, true, verbose)

			if len(resp.Msg.Drift) > 0 {
				term.Warnf(cmd.ErrOrStderr(), "%d drift item(s) detected", len(resp.Msg.Drift))
				return fmt.Errorf("%d drift item(s) detected", len(resp.Msg.Drift))
			}
			return nil
		},
	}

	c.Flags().BoolVarP(&force, "force", "f", false, "skip interactive approval prompt")
	c.Flags().BoolVar(&debug, "debug", false, "enable detailed network request logging")
	c.Flags().BoolVarP(&verbose, "verbose", "v", false, "print each created resource")
	c.Flags().BoolVar(&recreateIDs, "recreate-ids", false, "ignore existing resource IDs and let the server generate new ones")
	c.Flags().BoolVar(&forceApply, "force-apply", false, "bypass the pre-apply server drift warning")
	return c
}

func serverHasDrift(ctx context.Context, ws *workspace.Workspace, plan *planner.Plan) (bool, error) {
	c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
	req := proto.Clone(plan.Request).(*diagv1.ApplyPlanRequest)
	req.DryRun = new(true)
	resp, err := c.ApplyWorkspacePlan(ctx, connect.NewRequest(req))
	if err != nil {
		return false, err
	}
	return len(resp.Msg.Drift) > 0, nil
}

func autoPullAndRebuild(cmd *cobra.Command, ws *workspace.Workspace, lockFile *workspace.LockFile, plan *planner.Plan, wdir string, recreateIDs bool) (*planner.Plan, int, error) {
	c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
	req := proto.Clone(plan.Request).(*diagv1.ApplyPlanRequest)
	req.DryRun = new(true)
	resp, err := c.ApplyWorkspacePlan(cmd.Context(), connect.NewRequest(req))
	if err != nil {
		return nil, 0, fmt.Errorf("server plan failed: %w", err)
	}
	if len(resp.Msg.Conflicts) == 0 {
		return nil, 0, nil
	}
	if !cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
		term.Warnf(cmd.ErrOrStderr(), "Version conflict detected during force apply. Pulling and retrying once...")
	}
	newPlan, err := pullAndRebuildPlan(cmd, ws, lockFile, wdir, recreateIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("auto-retry pull failed: %w", err)
	}
	return newPlan, 1, nil
}

// detectAndHandleConflicts checks for version conflicts by performing a dry run on the server.
// Returns a new plan if Pull & Merge was performed, nil if the original plan should be used.
func detectAndHandleConflicts(
	cmd *cobra.Command,
	ws *workspace.Workspace,
	lockFile *workspace.LockFile,
	_ *workspace.Meta,
	plan *planner.Plan,
	scanner *bufio.Scanner,
	wdir string,
	recreateIDs bool,
) (*planner.Plan, error) {
	// Perform dry run on the server
	c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
	plan.Request.DryRun = new(true)
	resp, err := c.ApplyWorkspacePlan(cmd.Context(), connect.NewRequest(plan.Request))
	plan.Request.DryRun = nil // reset so the real apply is not also a dry run
	if err != nil {
		return nil, fmt.Errorf("server plan failed: %w", err)
	}

	if len(resp.Msg.Conflicts) == 0 {
		return nil, nil
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Version conflict detected:\n")
	if resp.Msg.Version != nil {
		term.Warnf(cmd.ErrOrStderr(), "Remote has newer version %s (%s) via %s",
			resp.Msg.Version.VersionId, resp.Msg.Version.CreatedAt.AsTime().Format(time.RFC3339), resp.Msg.Version.CreatedBy)
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  %d conflicts detected:\n", len(resp.Msg.Conflicts))
	for _, conflict := range resp.Msg.Conflicts {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "    * %s %q (local %s, remote %s)\n",
			conflict.ResourceType, conflict.Ref,
			conflict.LocalUpdatedAt.AsTime().Format(time.RFC3339),
			conflict.RemoteUpdatedAt.AsTime().Format(time.RFC3339))
	}

	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nOptions:\n")
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  [1] Abort and review changes\n")
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  [2] Pull & Merge (fetch server state and merge locally)\n")
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "  [3] Force Apply (overwrite remote changes)\n")
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "\nChoose option [1-3]: ")

	if !scanner.Scan() {
		return nil, errors.New("no response received")
	}

	choice := strings.TrimSpace(scanner.Text())
	switch choice {
	case "1":
		term.Info(cmd.OutOrStdout(), "Apply aborted.")
		return nil, errors.New("apply aborted by user")

	case "2":
		term.Info(cmd.OutOrStdout(), "Pulling server state and merging locally...")
		newPlan, err := pullAndRebuildPlan(cmd, ws, lockFile, wdir, recreateIDs)
		if err != nil {
			return nil, fmt.Errorf("pull & merge: %w", err)
		}
		term.Success(cmd.OutOrStdout(), "Merge complete. Proceeding with apply...")
		return newPlan, nil

	case "3":
		term.Info(cmd.OutOrStdout(), "Proceeding with force apply...")
		return nil, nil

	default:
		return nil, errors.New("invalid choice or aborted")
	}
}

// pullAndRebuildPlan fetches the server state, merges it with the local workspace,
// and rebuilds the plan from the merged state.
func pullAndRebuildPlan(cmd *cobra.Command, ws *workspace.Workspace, lockFile *workspace.LockFile, wdir string, recreateIDs bool) (*planner.Plan, error) {
	diagv1client := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)

	targetOrg := ws.Config.WorkspaceID
	if targetOrg == "" {
		return nil, cmdutil.WorkspaceIDRequired("org-id required in tld.yaml for Pull & Merge")
	}

	exportResp, err := diagv1client.ExportWorkspace(cmd.Context(), connect.NewRequest(&diagv1.ExportOrganizationRequest{
		OrgId: targetOrg,
	}))
	if err != nil {
		return nil, fmt.Errorf("export workspace: %w", err)
	}

	newWS := cmdutil.ConvertExportResponse(ws, exportResp.Msg)

	lastSyncMeta := &workspace.Meta{
		Elements:   make(map[string]*workspace.ResourceMetadata),
		Views:      make(map[string]*workspace.ResourceMetadata),
		Connectors: make(map[string]*workspace.ResourceMetadata),
	}
	if lockFile != nil && lockFile.Metadata != nil {
		lastSyncMeta = lockFile.Metadata
	}

	if err := workspace.MergeWorkspace(wdir, newWS, lastSyncMeta, ws.Meta); err != nil {
		return nil, fmt.Errorf("merge: %w", err)
	}

	// Reload the merged workspace and rebuild the plan
	mergedWS, err := workspace.Load(wdir)
	if err != nil {
		return nil, fmt.Errorf("reload after merge: %w", err)
	}
	repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), wdir)
	if repoCtx.Name != "" && repoCtx.MatchesWorkspaceRepo(mergedWS) {
		mergedWS.ActiveRepo = repoCtx.Name
	}
	if errs := mergedWS.ValidateWithOpts(workspace.ValidationOptions{SkipSymbols: true}); len(errs) > 0 {
		for _, e := range errs {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Warning (post-merge validation): %s\n", e)
		}
	}

	newPlan, err := planner.Build(mergedWS, recreateIDs)
	if err != nil {
		return nil, fmt.Errorf("rebuild plan: %w", err)
	}
	return newPlan, nil
}

type refRename struct {
	from string
	to   string
}

func applyCanonicalRefs(wdir string, resp *diagv1.ApplyPlanResponse) ([]refRename, error) {
	var renames []refRename
	for _, result := range resp.GetElementResults() {
		if result.GetCanonicalRef() == "" || result.GetCanonicalRef() == result.GetRef() {
			continue
		}
		if err := workspace.RenameElement(wdir, result.GetRef(), result.GetCanonicalRef()); err != nil {
			return nil, fmt.Errorf("rename element %s -> %s: %w", result.GetRef(), result.GetCanonicalRef(), err)
		}
		renames = append(renames, refRename{from: result.GetRef(), to: result.GetCanonicalRef()})
	}
	return renames, nil
}

func updatePlanMetadataFromResponse(_ string, meta *workspace.Meta, ws *workspace.Workspace, plan *planner.Plan, respMsg *diagv1.ApplyPlanResponse) error {
	_ = plan
	if meta == nil {
		meta = &workspace.Meta{}
	}
	if meta.Elements == nil {
		meta.Elements = make(map[string]*workspace.ResourceMetadata)
	}
	if meta.Views == nil {
		meta.Views = make(map[string]*workspace.ResourceMetadata)
	}
	if meta.Connectors == nil {
		meta.Connectors = make(map[string]*workspace.ResourceMetadata)
	}

	elementMetadata := canonicalizeMetadata(respMsg.GetElementMetadata(), elementRefRenames(respMsg))
	viewMetadata := canonicalizeMetadata(respMsg.GetViewMetadata(), elementRefRenames(respMsg))
	connectorMetadata := canonicalizeConnectorMetadata(respMsg.GetConnectorMetadata(), elementRefRenames(respMsg))

	for ref := range ws.Elements {
		if metadata, ok := resourceMetadataFromMap(elementMetadata, ref); ok {
			meta.Elements[ref] = metadata
		}
	}
	for ref, element := range ws.Elements {
		if !element.HasView {
			continue
		}
		if metadata, ok := resourceMetadataFromMap(viewMetadata, ref); ok {
			meta.Views[ref] = metadata
		}
	}
	for ref := range ws.Connectors {
		if metadata, ok := resourceMetadataFromMap(connectorMetadata, ref); ok {
			meta.Connectors[ref] = metadata
		}
	}

	return nil
}

// updateLockFileFromResponse updates lock file with response data
func updateLockFileFromResponse(wdir string, existingLock *workspace.LockFile, ws *workspace.Workspace, meta *workspace.Meta, respMsg *diagv1.ApplyPlanResponse) error {
	summary := respMsg.GetSummary()
	diagramCount := len(respMsg.GetCreatedViews())
	elementCount := len(respMsg.GetCreatedElements())
	connectorCount := len(respMsg.GetCreatedConnectors())
	legacyDiagramCount := diagramCount
	legacyElementCount := elementCount
	legacyConnectorCount := connectorCount
	if summary != nil {
		legacyDiagramCount = int(summary.GetViewsCreated())
		legacyElementCount = int(summary.GetElementsCreated())
		legacyConnectorCount = int(summary.GetConnectorsCreated())
	}

	var lockFile *workspace.LockFile
	if existingLock != nil {
		lockFile = existingLock
	} else {
		lockFile = &workspace.LockFile{}
	}

	// Generate new version ID
	versionID := fmt.Sprintf("v%d", legacyDiagramCount+legacyElementCount+legacyConnectorCount)
	if respMsg.Version != nil {
		versionID = respMsg.Version.VersionId
	}

	// Calculate workspace hash
	workspaceHash, err := workspace.CalculateWorkspaceHash(wdir)
	if err != nil {
		return fmt.Errorf("calculate workspace hash: %w", err)
	}

	// Update lock file
	workspace.UpdateLockFile(lockFile, versionID, "cli", &workspace.ResourceCounts{
		Elements:   len(ws.Elements),
		Views:      diagramCount,
		Connectors: len(ws.Connectors),
	}, workspaceHash, nil, meta)

	if err := workspace.WriteLockFile(wdir, lockFile); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	return nil
}

func resourceMetadataFromMap(source map[string]*diagv1.ResourceMetadata, ref string) (*workspace.ResourceMetadata, bool) {
	resourceMeta, ok := source[ref]
	if !ok || resourceMeta == nil {
		return nil, false
	}
	metadata := &workspace.ResourceMetadata{ID: workspace.ResourceID(resourceMeta.Id)}
	if resourceMeta.UpdatedAt != nil {
		metadata.UpdatedAt = resourceMeta.UpdatedAt.AsTime()
	}
	return metadata, true
}

func elementRefRenames(resp *diagv1.ApplyPlanResponse) map[string]string {
	renames := make(map[string]string)
	for _, result := range resp.GetElementResults() {
		if result.GetCanonicalRef() == "" || result.GetCanonicalRef() == result.GetRef() {
			continue
		}
		renames[result.GetRef()] = result.GetCanonicalRef()
	}
	return renames
}

func canonicalizeMetadata(source map[string]*diagv1.ResourceMetadata, renames map[string]string) map[string]*diagv1.ResourceMetadata {
	if len(source) == 0 {
		return source
	}
	out := make(map[string]*diagv1.ResourceMetadata, len(source))
	for ref, metadata := range source {
		canonicalRef := ref
		if renamed, ok := renames[ref]; ok {
			canonicalRef = renamed
		}
		out[canonicalRef] = metadata
	}
	return out
}

func canonicalizeConnectorMetadata(source map[string]*diagv1.ResourceMetadata, renames map[string]string) map[string]*diagv1.ResourceMetadata {
	if len(source) == 0 {
		return source
	}
	out := make(map[string]*diagv1.ResourceMetadata, len(source))
	for ref, metadata := range source {
		out[canonicalConnectorRef(ref, renames)] = metadata
	}
	return out
}

func canonicalConnectorRef(ref string, renames map[string]string) string {
	parts := strings.Split(ref, ":")
	if len(parts) < 4 {
		if renamed, ok := renames[ref]; ok {
			return renamed
		}
		return ref
	}
	for _, index := range []int{0, 1, 2} {
		if renamed, ok := renames[parts[index]]; ok {
			parts[index] = renamed
		}
	}
	return strings.Join(parts, ":")
}
