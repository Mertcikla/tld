package plan

import (
	"fmt"
	"os"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/cmd/apply"
	"github.com/mertcikla/tld/v2/internal/cmdutil"

	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/client"
	"github.com/mertcikla/tld/v2/internal/planner"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewPlanCmd(wdir *string) *cobra.Command {
	var planOutput string
	var recreateIDs bool
	var verbose bool
	var strictness int
	var target string
	var dataDir string

	c := &cobra.Command{
		Use:   "plan",
		Short: "Show what would be applied (legacy, hidden)",
		Long: `Legacy: CRUD commands now apply instantly with immediate feedback.
'tld plan' is kept for backward compatibility only.`,
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			wantsJSONEarly := false
			if f := cmd.Root().PersistentFlags().Lookup("format"); f != nil {
				wantsJSONEarly = cmdutil.WantsJSON(f.Value.String())
			}
			if !wantsJSONEarly {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Note: `tld plan` is legacy; CRUD commands now apply instantly.\n")
			}
			ws, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				return err
			}
			runner, err := apply.NewRunner(ws.Config, target, dataDir, false, nil)
			if err != nil {
				return err
			}
			if runner.Name() == apply.TargetRemote {
				if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
					return err
				}
			}

			// Override strictness if flag is set
			if strictness > 0 {
				ws.Config.Validation.Level = strictness
			}

			if errs := ws.ValidateWithOpts(workspace.ValidationOptions{SkipSymbols: true}); len(errs) > 0 {
				for _, e := range errs {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "validation error: %s\n", e)
				}
				return fmt.Errorf("workspace has %d validation error(s)", len(errs))
			}
			repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), *wdir)
			if repoCtx.Name != "" && repoCtx.MatchesWorkspaceRepo(ws) {
				ws.ActiveRepo = repoCtx.Name
			}
			plan, err := planner.Build(ws, recreateIDs)
			if err != nil {
				return fmt.Errorf("build plan: %w", err)
			}

			out := cmd.OutOrStdout()
			if planOutput != "" {
				f, err := os.Create(planOutput)
				if err != nil {
					return fmt.Errorf("create output file: %w", err)
				}
				defer func() { _ = f.Close() }()
				out = f
			}

			wantsJSON := cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String())
			compactJSON := cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true"
			if !wantsJSON {
				apply.RenderTargetInfo(out, runner)
			}

			resp := &diagv1.ApplyPlanResponse{}
			if runner.SupportsDryRun() {
				// Perform dry run on the server to detect conflicts and drift.
				c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
				req := plan.Request
				*req.DryRun = true

				remoteResp, err := c.ApplyWorkspacePlan(cmd.Context(), connect.NewRequest(req))
				if err != nil {
					if wantsJSON {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), compactJSON, "plan", cmdutil.WithUnauthorizedHint("server plan failed", err))
					}
					return cmdutil.WithUnauthorizedHint("server plan failed", err)
				}
				resp = remoteResp.Msg
			}

			warnings := planner.AnalyzePlan(ws)
			if wantsJSON {
				return cmdutil.WriteJSON(out, compactJSON, cmdutil.BuildPlanJSON(ws, resp, warnings))
			}

			planner.RenderPlanMarkdown(out, plan, ws, verbose)

			// Show conflicts and drift if any
			if len(resp.Conflicts) > 0 {
				_, _ = fmt.Fprintf(out, "\n  %d conflicts detected:\n", len(resp.Conflicts))
				for _, c := range resp.Conflicts {
					_, _ = fmt.Fprintf(out, "  * %s \"%s\" (remote is newer: %s)\n",
						c.ResourceType, c.Ref, c.RemoteUpdatedAt.AsTime().Format(time.RFC3339))
				}
			}

			if len(resp.Drift) > 0 {
				_, _ = fmt.Fprintf(out, "\n %d drift items detected:\n", len(resp.Drift))
				for _, d := range resp.Drift {
					_, _ = fmt.Fprintf(out, "  * %s \"%s\": %s\n", d.ResourceType, d.Ref, d.Reason)
				}
			}

			// Evaluate Diagram warnings
			if len(warnings) > 0 {
				level := ws.Config.Validation.Level
				if level == 0 {
					level = workspace.DefaultValidationLevel
				}
				levelNames := map[int]string{1: "Minimal", 2: "Standard", 3: "Strict"}
				_, _ = fmt.Fprintf(out, "\n## Architectural Warnings (Level %d: %s)\n\n", level, levelNames[level])
				for _, wg := range warnings {
					if verbose {
						_, _ = fmt.Fprintf(out, "[%s] %s\n%s\n", wg.RuleCode, wg.RuleName, wg.Mediation)
						for _, v := range wg.Violations {
							_, _ = fmt.Fprintf(out, "  * %s\n", v)
						}
					} else {
						_, _ = fmt.Fprintf(out, "[%s] %s (%d violations)\n", wg.RuleCode, wg.RuleName, len(wg.Violations))
					}
					_, _ = fmt.Fprintln(out)
				}
			}

			return nil
		},
	}

	c.Flags().StringVarP(&planOutput, "output", "o", "", "write plan to file instead of stdout")
	c.Flags().BoolVar(&recreateIDs, "recreate-ids", false, "ignore existing resource IDs and let the server generate new ones")
	c.Flags().BoolVarP(&verbose, "verbose", "v", false, "show detailed resource reporting (elements, diagrams, connectors)")
	c.Flags().IntVar(&strictness, "strictness", 0, "override validation strictness level [1-3]")
	c.Flags().StringVar(&target, "target", "", "plan target: auto, local, cloud, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}
