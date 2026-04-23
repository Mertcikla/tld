package plan

import (
	"fmt"
	"os"
	"time"

	"github.com/mertcikla/tld/internal/cmdutil"

	"connectrpc.com/connect"
	"github.com/mertcikla/tld/internal/client"
	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func NewPlanCmd(wdir *string) *cobra.Command {
	var planOutput string
	var recreateIDs bool
	var verbose bool
	var strictness int

	c := &cobra.Command{
		Use:   "plan",
		Short: "Show what would be applied",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				return err
			}
			if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
				return err
			}

			// Override strictness if flag is set
			if strictness > 0 {
				if ws.Config.Validation == nil {
					ws.Config.Validation = &workspace.ValidationConfig{}
				}
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

			// Perform dry run on the server to detect conflicts and drift
			c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
			req := plan.Request
			*req.DryRun = true

			resp, err := c.ApplyWorkspacePlan(cmd.Context(), connect.NewRequest(req))
			if err != nil {
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "plan", cmdutil.WithUnauthorizedHint("server plan failed", err))
				}
				return cmdutil.WithUnauthorizedHint("server plan failed", err)
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

			warnings := planner.AnalyzePlan(ws)
			if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
				return cmdutil.WriteJSON(out, cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", cmdutil.BuildPlanJSON(ws, resp.Msg, warnings))
			}

			planner.RenderPlanMarkdown(out, plan, ws, verbose)

			// Show conflicts and drift if any
			if len(resp.Msg.Conflicts) > 0 {
				_, _ = fmt.Fprintf(out, "\n  %d conflicts detected:\n", len(resp.Msg.Conflicts))
				for _, c := range resp.Msg.Conflicts {
					_, _ = fmt.Fprintf(out, "  * %s \"%s\" (remote is newer: %s)\n",
						c.ResourceType, c.Ref, c.RemoteUpdatedAt.AsTime().Format(time.RFC3339))
				}
			}

			if len(resp.Msg.Drift) > 0 {
				_, _ = fmt.Fprintf(out, "\n %d drift items detected:\n", len(resp.Msg.Drift))
				for _, d := range resp.Msg.Drift {
					_, _ = fmt.Fprintf(out, "  * %s \"%s\": %s\n", d.ResourceType, d.Ref, d.Reason)
				}
			}

			// Evaluate Diagram warnings
			if len(warnings) > 0 {
				level := workspace.DefaultValidationLevel
				if ws.Config.Validation != nil && ws.Config.Validation.Level > 0 {
					level = ws.Config.Validation.Level
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
	return c
}
