package status

import (
	"fmt"
	"time"

	"github.com/mertcikla/tld/internal/cmdutil"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/internal/client"
	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/term"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func NewStatusCmd(wdir *string) *cobra.Command {
	var checkServer bool

	c := &cobra.Command{
		Use:   "status",
		Short: "Show sync status between local YAML and the server",
		Long: `Status compares the local workspace against the last known sync point.

With --check-server, it also performs a dry-run on the server to detect
any drift from manual changes in the frontend.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "status", err)
				}
				return err
			}
			repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), *wdir)
			if repoCtx.Name != "" && repoCtx.MatchesWorkspaceRepo(ws) {
				ws.ActiveRepo = repoCtx.Name
			}
			lockFile, err := workspace.LoadLockFile(*wdir)
			if err != nil {
				return fmt.Errorf("load lock file: %w", err)
			}

			if lockFile != nil {
				currentHash, hashErr := workspace.CalculateWorkspaceHash(*wdir)
				localModified := hashErr == nil && lockFile.WorkspaceHash != "" && currentHash != lockFile.WorkspaceHash
				conflicts := countWorkspaceConflicts(ws)

				var serverResp *connect.Response[diagv1.ApplyPlanResponse]
				if checkServer {
					if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
						return err
					}
					c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
					plan, err := planner.Build(ws, false)
					if err != nil {
						return fmt.Errorf("build plan: %w", err)
					}
					plan.Request.DryRun = new(true)
					serverResp, err = c.ApplyWorkspacePlan(cmd.Context(), connect.NewRequest(plan.Request))
					if err != nil {
						return cmdutil.WithUnauthorizedHint("server check failed", err)
					}
				}

				serverDrift := serverResp != nil && (len(serverResp.Msg.Drift) > 0 || len(serverResp.Msg.Conflicts) > 0)
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					return cmdutil.WriteJSON(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", cmdutil.BuildStatusJSON(lockFile, localModified, serverDrift, conflicts, respOrNil(serverResp)))
				}
				printStatusHeader(cmd.OutOrStdout(), localModified, serverDrift, conflicts)
				term.Label(cmd.OutOrStdout(), 15, "Last sync", lockFile.LastApply.Format(time.RFC3339))
				term.Label(cmd.OutOrStdout(), 15, "Applied by", lockFile.AppliedBy)
				term.Label(cmd.OutOrStdout(), 15, "Version", lockFile.VersionID)
				if hashErr == nil {
					if localModified {
						term.Label(cmd.OutOrStdout(), 15, "Local changes", "Modified")
					} else {
						term.Label(cmd.OutOrStdout(), 15, "Local changes", "Clean")
					}
				}
				if conflicts > 0 {
					term.Label(cmd.OutOrStdout(), 15, "Merge conflicts", fmt.Sprintf("%d found (run 'tld diff' to review)", conflicts))
				}

				if checkServer {
					term.Infof(cmd.OutOrStdout(), "Checking server drift...")
					if len(serverResp.Msg.Drift) == 0 && len(serverResp.Msg.Conflicts) == 0 {
						term.Label(cmd.OutOrStdout(), 15, "Server state", "In sync")
					} else {
						if len(serverResp.Msg.Drift) > 0 {
							term.Label(cmd.OutOrStdout(), 15, "Server state", fmt.Sprintf("%d drift items found (run 'tld pull' to sync)", len(serverResp.Msg.Drift)))
							for _, d := range serverResp.Msg.Drift {
								_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    - %s: %s (%s)\n", d.ResourceType, d.Ref, d.Reason)
							}
						}
						if len(serverResp.Msg.Conflicts) > 0 {
							term.Label(cmd.OutOrStdout(), 15, "Server state", fmt.Sprintf("%d conflicts found (run 'tld pull' or 'tld apply' to resolve)", len(serverResp.Msg.Conflicts)))
						}
					}
				}
			} else {
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					return cmdutil.WriteJSON(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", cmdutil.BuildStatusJSON(nil, false, false, 0, nil))
				}
				term.Info(cmd.OutOrStdout(), "No sync history found.")
			}

			return nil
		},
	}

	c.Flags().BoolVar(&checkServer, "check-server", false, "check against the live server state")
	return c
}

func respOrNil(resp *connect.Response[diagv1.ApplyPlanResponse]) *diagv1.ApplyPlanResponse {
	if resp == nil {
		return nil
	}
	return resp.Msg
}

func printStatusHeader(out interface{ Write([]byte) (int, error) }, localModified, serverDrift bool, conflicts int) {
	switch {
	case serverDrift:
		_, _ = fmt.Fprintln(out, term.Colorize(out, term.ColorRed, "✗ DRIFTED"),
			"  server has changes not in YAML (run tld pull)")
	case localModified || conflicts > 0:
		if conflicts > 0 {
			_, _ = fmt.Fprintf(out, "%s  %d merge conflicts (run tld diff to review)\n",
				term.Colorize(out, term.ColorYellow, "! MODIFIED"), conflicts)
		} else {
			_, _ = fmt.Fprintln(out, term.Colorize(out, term.ColorYellow, "! MODIFIED"),
				"  local changes not pushed (run tld apply)")
		}
	default:
		_, _ = fmt.Fprintln(out, term.Colorize(out, term.ColorGreen, "✓ IN SYNC"),
			"  workspace matches last applied state")
	}
}

func countWorkspaceConflicts(ws *workspace.Workspace) int {
	if ws == nil || ws.Meta == nil {
		return 0
	}
	count := 0
	for _, bucket := range []map[string]*workspace.ResourceMetadata{
		ws.Meta.Elements,
		ws.Meta.Views,
		ws.Meta.Connectors,
	} {
		for _, meta := range bucket {
			if meta != nil && meta.Conflict {
				count++
			}
		}
	}
	return count
}
