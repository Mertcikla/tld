package validate

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	mappingcheck "github.com/mertcikla/tld/v2/internal/codeindex/mappingcheck"
	"github.com/mertcikla/tld/v2/internal/term"
	archwarnings "github.com/mertcikla/tld/v2/internal/warnings"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

var allWarningCodes = func() map[string]bool {
	codes := make(map[string]bool)
	for _, r := range archwarnings.Rules() {
		codes[r.Code] = true
	}
	return codes
}()

var levelNames = map[int]string{1: "Minimal", 2: "Standard", 3: "Strict"}

func normalizeRuleCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func knownRuleCodes() []string {
	codes := make([]string, 0, len(allWarningCodes))
	for code := range allWarningCodes {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func withoutRuleCode(codes []string, code string) []string {
	if len(codes) == 0 {
		return codes
	}
	filtered := make([]string, 0, len(codes))
	for _, c := range codes {
		if normalizeRuleCode(c) != code {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func NewValidateCmd(wdir *string) *cobra.Command {
	var strictness int
	var verbose bool
	var strict bool
	var dataDir string

	c := &cobra.Command{
		Use:   "validate [rule-code]",
		Short: "Validate the workspace and check diagram freshness",
		Long: `Validate the workspace YAML files for structural errors, verify that
referenced symbols still exist in source files, and flag diagrams whose
metadata is older than the file's last git commit.

When called without arguments, validates the entire workspace and shows a summary
of architectural warnings grouped by rule code.

When called with a rule code (e.g. ARC002), shows only that rule's violations
in full detail with individual element and connector information. The requested
rule runs regardless of the configured strictness level or exclude list.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}
			repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), *wdir)
			rules := ws.IgnoreRulesForRepository(repoCtx.Name)

			// Identify codeindex-materialized elements live against the local
			// database so ARC205 scores only user-authored diagrams.
			var scoreOpts []archwarnings.Option
			if resolvedDir, dirErr := workspace.ResolveDataDir(&ws.Config, dataDir); dirErr == nil {
				if classify := mappingcheck.Classifier(cmd.Context(), resolvedDir); classify != nil {
					scoreOpts = append(scoreOpts, archwarnings.WithCodeindexElementClassifier(classify))
				}
			}

			if strictness > 0 {
				ws.Config.Validation.Level = strictness
			}

			if len(args) == 1 {
				code := normalizeRuleCode(args[0])
				if !allWarningCodes[code] {
					return fmt.Errorf("unknown rule code %q; known codes: %s", code, strings.Join(knownRuleCodes(), ", "))
				}
				// A directly requested rule runs regardless of the configured
				// strictness level or exclude list.
				ws.Config.Validation.IncludeRules = append(ws.Config.Validation.IncludeRules, code)
				ws.Config.Validation.ExcludeRules = withoutRuleCode(ws.Config.Validation.ExcludeRules, code)
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

			warnings := archwarnings.Analyze(ws, scoreOpts...)

			if len(args) == 1 {
				if normalizeRuleCode(args[0]) == "ARC205" {
					return printGrounding(cmd, ws, scoreOpts...)
				}
				return printRuleViolations(cmd, args[0], warnings)
			}

			if len(ws.Elements) > 0 || len(ws.Connectors) > 0 {
				viewCount := cmdutil.CountViews(ws)
				term.Successf(cmd.OutOrStdout(), "Workspace valid: %d elements, %d views, %d connectors",
					len(ws.Elements), viewCount, len(ws.Connectors))
			} else {
				term.Warnf(cmd.OutOrStdout(), "nothing to validate")
			}

			if len(warnings) > 0 {
				printWarningSummary(cmd, ws, warnings, verbose)
			}

			return nil
		},
	}

	c.Flags().IntVar(&strictness, "strictness", 0, "override validation strictness level [1-3]")
	c.Flags().BoolVarP(&verbose, "verbose", "v", false, "show full architectural warnings output")
	c.Flags().BoolVar(&strict, "strict", false, "exit non-zero when outdated diagrams are detected")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for the local database used to identify codeindex elements")
	c.AddCommand(newRulesCmd())
	return c
}

func newRulesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rules",
		Short: "List architectural validation rules grouped by strictness level",
		Long: `List every architectural validation rule, grouped by the strictness level at
which it becomes active, along with its description.

Rules are enabled based on validation.level in .tld.yaml (1 = Minimal,
2 = Standard, 3 = Strict). Use 'tld validate <code>' to inspect a rule's
violations in the current workspace.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			printRules(cmd)
			return nil
		},
	}
}

func printRules(cmd *cobra.Command) {
	out := cmd.OutOrStdout()
	rules := archwarnings.Rules()

	levels := make([]int, 0, len(levelNames))
	seen := make(map[int]bool)
	for _, r := range rules {
		if !seen[r.Level] {
			seen[r.Level] = true
			levels = append(levels, r.Level)
		}
	}
	sort.Ints(levels)

	_, _ = fmt.Fprintln(out, "Architectural Validation Rules")
	_, _ = fmt.Fprintln(out)

	for _, level := range levels {
		name := levelNames[level]
		if name == "" {
			name = "Custom"
		}
		_, _ = fmt.Fprintf(out, "Level %d (%s)\n\n", level, name)
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "CODE\tRULE\tDESCRIPTION")
		for _, r := range rules {
			if r.Level == level {
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Code, r.Name, r.Description)
			}
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintln(out)
	}

	_, _ = fmt.Fprintln(out, "Use 'tld validate <code>' to see detailed violations for a rule.")
}

func printWarningSummary(cmd *cobra.Command, ws *workspace.Workspace, warnings []archwarnings.WarningGroup, verbose bool) {
	level := ws.Config.Validation.Level
	if level == 0 {
		level = workspace.DefaultValidationLevel
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "\nArchitectural Warnings (Level %d: %s)\n\n", level, levelNames[level])
	_, _ = fmt.Fprintln(out, "Issues found in workspace that may affect the visibility and usability of your diagrams. Consider applying the suggested mediations to improve your diagrams.")
	_, _ = fmt.Fprintln(out)

	if verbose {
		for _, wg := range warnings {
			_, _ = fmt.Fprintf(out, "[%s] %s\n", wg.RuleCode, wg.RuleName)
			_, _ = fmt.Fprintf(out, "  %s\n", wg.Mediation)
			if wg.Score != nil {
				for _, reason := range wg.Score.Reasoning {
					_, _ = fmt.Fprintf(out, "  %s\n", reason)
				}
			}
			for _, v := range wg.Violations {
				_, _ = fmt.Fprintf(out, "    - %s\n", v)
			}
			_, _ = fmt.Fprintln(out)
		}
	} else {
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "CODE\tRULE\tVIOLATIONS")
		for _, wg := range warnings {
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\n", wg.RuleCode, wg.RuleName, len(wg.Violations))
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintln(out)

		tw = tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "CODE\tMEDIATION")
		for _, wg := range warnings {
			_, _ = fmt.Fprintf(tw, "%s\t%s\n", wg.RuleCode, wg.Mediation)
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintln(out)
	}

	_, _ = fmt.Fprintln(out, "To suppress specific rule codes, use .tld.yaml: validation.exclude_rules: [ARC002]")
}

func printGrounding(cmd *cobra.Command, ws *workspace.Workspace, opts ...archwarnings.Option) error {
	out := cmd.OutOrStdout()
	report := archwarnings.Grounding(ws, opts...)

	_, _ = fmt.Fprintln(out, "[ARC205] Low Grounding")
	_, _ = fmt.Fprintln(out, "Description: View has too few source-linked elements")
	_, _ = fmt.Fprintf(out, "Workspace source grounding: %d/10\n\n", report.Value)
	_, _ = fmt.Fprintf(out, "Linkable elements: %d\n", report.Eligible)
	_, _ = fmt.Fprintf(out, "Source-linked:     %d\n", report.Grounded)
	if report.External > 0 {
		_, _ = fmt.Fprintf(out, "Exempt (external links): %d\n", report.External)
	}
	_, _ = fmt.Fprintln(out)

	_, _ = fmt.Fprintln(out, "Reasoning:")
	for _, reason := range report.Reasoning {
		_, _ = fmt.Fprintf(out, "  - %s\n", reason)
	}

	hasViews := false
	for _, view := range report.Views {
		if view.Eligible > 0 {
			hasViews = true
			break
		}
	}
	if hasViews {
		_, _ = fmt.Fprintln(out, "\nPer-view scores:")
		tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  VIEW\tSCORE\tLINKED\tUNGROUNDED")
		for _, view := range report.Views {
			if view.Eligible == 0 {
				continue
			}
			_, _ = fmt.Fprintf(tw, "  %s\t%d/10\t%d/%d\t%s\n", view.ViewRef, view.Value, view.Grounded, view.Eligible, strings.Join(view.Ungrounded, ", "))
		}
		_ = tw.Flush()
	}

	_, _ = fmt.Fprintln(out, "\nHow to improve:")
	if rule, ok := archwarnings.RuleByCode("ARC205"); ok {
		_, _ = fmt.Fprintf(out, "  %s\n", rule.Mediation)
	}
	return nil
}

func printRuleViolations(cmd *cobra.Command, code string, warnings []archwarnings.WarningGroup) error {
	code = normalizeRuleCode(code)

	if !allWarningCodes[code] {
		return fmt.Errorf("unknown rule code %q; known codes: %s", code, strings.Join(knownRuleCodes(), ", "))
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
