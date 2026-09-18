package validate

import (
	"fmt"
	"strings"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/planner"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

var allWarningCodes = map[string]bool{
	"ARC001": true, "ARC002": true, "ARC003": true, "ARC004": true,
	"ARC005": true, "ARC006": true, "ARC007": true,
	"ARC101": true, "ARC102": true, "ARC103": true,
	"ARC201": true, "ARC202": true, "ARC203": true,
}

func NewValidateCmd(wdir *string) *cobra.Command {
	var strictness int
	var verbose bool

	c := &cobra.Command{
		Use:   "validate [rule-code]",
		Short: "Validate the workspace YAML files",
		Long: `Validate the workspace YAML files for structural errors and architectural warnings.

When called without arguments, validates the entire workspace and shows a summary
of architectural warnings grouped by rule code.

When called with a rule code (e.g. ARC002), shows only that rule's violations
in full detail with individual element and connector information.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}
			repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), *wdir)
			rules := ws.IgnoreRulesForRepository(repoCtx.Name)

			if strictness > 0 {
				ws.Config.Validation.Level = strictness
			}
			validationWarnings := ws.ValidateWarnings()

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

			if len(ws.Elements) > 0 || len(ws.Connectors) > 0 {
				viewCount := cmdutil.CountViews(ws)
				term.Successf(cmd.OutOrStdout(), "Workspace valid: %d elements, %d views, %d connectors",
					len(ws.Elements), viewCount, len(ws.Connectors))
				term.Hint(cmd.OutOrStdout(), "CRUD commands apply instantly; use --yaml-only to stage without applying.")
			} else {
				term.Warnf(cmd.OutOrStdout(), "nothing to validate")
			}

			if len(validationWarnings) > 0 {
				term.Warn(cmd.OutOrStdout(), "Validation warnings:")
				for _, warning := range validationWarnings {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    - %s\n", warning)
				}
			}

			warnings := planner.AnalyzePlan(ws)

			if len(args) == 1 {
				return printRuleViolations(cmd, args[0], warnings)
			}

			if len(warnings) > 0 {
				printWarningSummary(cmd, ws, warnings, verbose)
			}

			return nil
		},
	}

	c.Flags().IntVar(&strictness, "strictness", 0, "override validation strictness level [1-3]")
	c.Flags().BoolVarP(&verbose, "verbose", "v", false, "show full architectural warnings output")
	return c
}

func printWarningSummary(cmd *cobra.Command, ws *workspace.Workspace, warnings []planner.WarningGroup, verbose bool) {
	level := ws.Config.Validation.Level
	if level == 0 {
		level = workspace.DefaultValidationLevel
	}
	levelNames := map[int]string{1: "Minimal", 2: "Standard", 3: "Strict"}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n## Architectural Warnings (Level %d: %s)\n\n", level, levelNames[level])
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Issues found in workspace that may affect the visibility and usability of your diagrams. Consider applying the suggested mediations to improve your diagrams.\n\n")
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
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "To suppress specific rule codes, use .tld.yaml: validation.exclude_rules: [ARC002]")
}

func printRuleViolations(cmd *cobra.Command, code string, warnings []planner.WarningGroup) error {
	code = strings.ToUpper(strings.TrimSpace(code))

	if !allWarningCodes[code] {
		var known []string
		for k := range allWarningCodes {
			known = append(known, k)
		}
		return fmt.Errorf("unknown rule code %q; known codes: %s", code, strings.Join(known, ", "))
	}

	for _, wg := range warnings {
		if wg.RuleCode == code {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s\n", wg.RuleCode, wg.RuleName)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", wg.Description)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Violations: %d\n\n", len(wg.Violations))
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "How to fix:\n")
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", wg.Mediation)
			if len(wg.Violations) > 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nViolating elements:\n")
				for _, v := range wg.Violations {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  * %s\n", v)
				}
			}
			return nil
		}
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No violations found for %s.\n", code)
	return nil
}
