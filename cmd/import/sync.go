package importcmd

import (
	"fmt"
	"sort"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// NewSyncCmd pushes the current workspace YAML to the sync target. It reuses the
// import batch machinery, so it is idempotent and applies in a single
// transaction. It is the inverse of `tld pull`.
func NewSyncCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		target  string
		dataDir string
		dryRun  bool
	)

	c := &cobra.Command{
		Use:   "sync",
		Short: "Push the current workspace to the target (local, remote, or cloud)",
		Long: `Push the current workspace YAML to the sync target.

This is the inverse of 'tld pull': local YAML changes (for example source links
written by 'tld link') are applied to the local database or remote server so the
running app and UI reflect them. Elements, views, placements and connectors are
matched to their existing target resources. Unchanged values are skipped and
reported separately from new and updated resources.

Use --target to choose the destination:
  auto    API key + workspace id present -> remote, otherwise local (default)
  local   the local SQLite/Postgres database
  remote  the configured tlDiagram server (requires an API key)
  cloud   alias for remote`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSync(cmd, *wdir, *format, *compact, target, dataDir, dryRun)
		},
	}

	c.Flags().BoolVar(&dryRun, "dry-run", false, "report changes without applying them")
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, remote, or cloud")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}

func runSync(cmd *cobra.Command, wdir, format string, compact bool, target, dataDir string, dryRun bool) error {
	target, dataDir = cmdutil.TargetOverrides(cmd, target, dataDir)
	fail := func(err error) error {
		if cmdutil.WantsJSON(format) {
			return cmdutil.WriteCommandError(cmd.OutOrStdout(), compact, "sync", err)
		}
		return err
	}

	ws, err := cmdutil.LoadWorkspace(wdir)
	if err != nil {
		return fail(err)
	}

	sess, err := cmdutil.OpenSession(cmd, wdir, target, dataDir)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = sess.Close() }()
	runner, err := sess.Runner()
	if err != nil {
		return fail(err)
	}
	if runner.Name() == exec.TargetRemote {
		if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
			return fail(err)
		}
	}

	ctx := cmd.Context()
	batch, err := exec.BuildImportBatch(ctx, runner, ws, ws.Elements, sortedConnectors(ws.Connectors))
	if err != nil {
		return fail(fmt.Errorf("sync aborted before any changes were made: %w", err))
	}

	setImportDiagnostics(batch, ws, ws.Elements)
	if dryRun {
		return reportImport(cmd, format, compact, "sync", "dry-run", batch)
	}

	resp, err := runner.ApplyBatch(ctx, batch.Request)
	if err != nil {
		return fail(explainApplyError(err))
	}

	actual, err := runner.ExportWorkspace(ctx)
	if err != nil {
		return fail(sess.CacheError(err))
	}
	if err := persistImportCache(wdir, ws, batch, resp, actual); err != nil {
		return fail(sess.CacheError(err))
	}

	batch.Views = ws.ViewDiagnostics()
	return reportImport(cmd, format, compact, "sync", "ok", batch, resp)
}

// sortedConnectors returns the workspace connectors in a deterministic order so
// the generated batch is stable across runs.
func sortedConnectors(connectors map[string]*workspace.Connector) []*workspace.Connector {
	out := make([]*workspace.Connector, 0, len(connectors))
	for _, connector := range connectors {
		if connector != nil {
			out = append(out, connector)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return workspace.ConnectorKey(out[i]) < workspace.ConnectorKey(out[j])
	})
	return out
}
