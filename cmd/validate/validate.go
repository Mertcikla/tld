package validate

import (
	"fmt"

	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/term"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func NewValidateCmd(wdir *string) *cobra.Command {
	var strictness int
	var verbose bool

	c := &cobra.Command{
		Use:   "validate",
		Short: "Validate the workspace YAML files",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}
			repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), *wdir)
			rules := ws.IgnoreRulesForRepository(repoCtx.Name)

			// Override strictness if flag is set
			if strictness > 0 {
				ws.Config.Validation.Level = strictness
			}

			errs := ws.Validate()
			if len(errs) > 0 {
				term.Fail(cmd.ErrOrStderr(), "Validation errors:")
				for _, e := range errs {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "    - %s\n", e)
				}
				return fmt.Errorf("%d validation error(s)", len(errs))
			}

			broken := cmdutil.CheckSymbols(cmd.Context(), ws, repoCtx, rules)
			if len(broken) > 0 {
				term.Fail(cmd.ErrOrStderr(), "Symbol verification errors:")
				for _, msg := range broken {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "    - %s\n", msg)
				}
				return fmt.Errorf("%d symbol verification error(s)", len(broken))
			}
			term.Success(cmd.OutOrStdout(), "Symbol verification passed")

			if len(ws.Elements) > 0 || len(ws.Connectors) > 0 {
				diagramCount := cmdutil.CountElementDiagrams(ws)
				term.Successf(cmd.OutOrStdout(), "Workspace valid: %d elements, %d diagrams, %d connectors",
					len(ws.Elements), diagramCount, len(ws.Connectors))
			}

			// Evaluate Architectural warnings
			warnings := planner.AnalyzePlan(ws)
			if len(warnings) > 0 {
				level := ws.Config.Validation.Level
				if level == 0 {
					level = workspace.DefaultValidationLevel
				}
				levelNames := map[int]string{1: "Minimal", 2: "Standard", 3: "Strict"}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n## Architectural Warnings (Level %d: %s)\n\n", level, levelNames[level])
				for _, wg := range warnings {
					if verbose {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n%s\n", wg.RuleCode, wg.RuleName, wg.Mediation)
						for _, v := range wg.Violations {
							_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  * %s\n", v)
						}
					} else {
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s (%d violations)\n", wg.RuleCode, wg.RuleName, len(wg.Violations))
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", wg.Mediation)
					}
					_, _ = fmt.Fprintln(cmd.OutOrStdout())
				}
			}

			return nil
		},
	}

	c.Flags().IntVar(&strictness, "strictness", 0, "override validation strictness level [1-3]")
	c.Flags().BoolVarP(&verbose, "verbose", "v", false, "show full architectural warnings output")
	return c
}
