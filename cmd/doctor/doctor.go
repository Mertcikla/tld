package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/tools"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// NewDoctorCmd reports external SCIP indexer availability for the codeindex
// pipeline.
func NewDoctorCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check codeindex prerequisites (external SCIP indexers)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			global, err := workspace.LoadGlobalConfig()
			if err != nil {
				return err
			}
			cfg := configbridge.FromGlobal(global)
			report := buildReport(cmd.Context(), cfg)
			if asJSON || cmdutil.WantsJSONFromCmd(cmd) {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(report)
			}
			printReport(cmd, report)
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "output the report as JSON")
	return c
}

type report struct {
	Tools []tools.Status `json:"tools"`
	OK    bool           `json:"ok"`
}

func buildReport(ctx context.Context, cfg config.Config) report {
	toolStatuses := tools.Check(ctx, cfg)
	ok := true
	for _, s := range toolStatuses {
		if !s.Found {
			ok = false
		}
	}
	return report{Tools: toolStatuses, OK: ok}
}

func printReport(cmd *cobra.Command, r report) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "SCIP indexers:")
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TOOL\tSTATUS\tDETAIL")
	for _, s := range r.Tools {
		status := "OK"
		detail := s.Path
		if !s.Found {
			status = "MISSING"
			detail = s.Error
		} else if s.Version != "" {
			detail = fmt.Sprintf("%s (%s)", s.Path, s.Version)
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Name, status, detail)
	}
	_ = tw.Flush()

	if r.OK {
		_, _ = fmt.Fprintln(out, "\nAll prerequisites satisfied.")
	} else {
		_, _ = fmt.Fprintln(out, "\nSome prerequisites are missing; indexing projects that need them will fail.")
	}
}

// Summary is a one-line description used by callers that only need a count.
func Summary(statuses []tools.Status) string {
	missing := 0
	for _, s := range statuses {
		if !s.Found {
			missing++
		}
	}
	if missing == 0 {
		return fmt.Sprintf("%d indexers available", len(statuses))
	}
	names := make([]string, 0, missing)
	for _, s := range statuses {
		if !s.Found {
			names = append(names, s.Name)
		}
	}
	return fmt.Sprintf("%d/%d indexers available; missing: %s", len(statuses)-missing, len(statuses), strings.Join(names, ", "))
}
