package check

import (
	"fmt"

	"github.com/mertcikla/diag/tld/internal/cmdutil"
	"github.com/mertcikla/diag/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func NewCheckCmd(wdir *string) *cobra.Command {
	var strict bool

	c := &cobra.Command{
		Use:   "check",
		Short: "Check workspace health and diagram freshness",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}
			repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), *wdir)
			rules := ws.IgnoreRulesForRepository(repoCtx.Name)

			allPassed := true

			// 1. Validate Workspace
			errs := ws.Validate()
			if len(errs) > 0 {
				allPassed = false
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "FAIL  Validation")
				for _, e := range errs {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "      - %s\n", e)
				}
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "PASS  Validation")
			}

			// 2. Check Symbols
			broken := cmdutil.CheckSymbols(cmd.Context(), ws, repoCtx, rules)
			if len(broken) > 0 {
				allPassed = false
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "FAIL  Symbol Verification")
				for _, msg := range broken {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "      - %s\n", msg)
				}
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "PASS  Symbol Verification")
			}

			// 2. Check Freshness
			outdated := cmdutil.CheckOutdated(ws, repoCtx, rules)
			if len(outdated) > 0 {
				if strict {
					allPassed = false
				}
				label := "WARN "
				if strict {
					label = "FAIL "
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s Outdated Diagrams\n", label)
				for _, msg := range outdated {
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "      - %s\n", msg)
				}
				if strict {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "      (use `tld apply` to sync diagram metadata)")
				}
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "PASS  Outdated Diagrams")
			}

			if !allPassed {
				return fmt.Errorf("check failed")
			}
			return nil
		},
	}

	c.Flags().BoolVar(&strict, "strict", false, "exit non-zero when outdated diagrams are detected")
	return c
}
