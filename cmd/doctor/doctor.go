package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/tools"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// NewDoctorCmd reports external indexer availability and embedding endpoint
// reachability for the codeindex pipeline.
func NewDoctorCmd() *cobra.Command {
	var asJSON bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check codeindex prerequisites (SCIP indexers and embedding endpoint)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			global, err := workspace.LoadGlobalConfig()
			if err != nil {
				return err
			}
			cfg := configbridge.FromGlobal(global)
			ctx := cmd.Context()
			report := buildReport(ctx, cfg)
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
	Tools     []tools.Status `json:"tools"`
	Embedding endpointReport `json:"embedding"`
	OK        bool           `json:"ok"`
}

type endpointReport struct {
	Endpoint  string `json:"endpoint"`
	Reachable bool   `json:"reachable"`
	Error     string `json:"error,omitempty"`
}

func buildReport(ctx context.Context, cfg config.Config) report {
	toolStatuses := tools.Check(ctx, cfg)
	emb := checkEndpoint(ctx, cfg.Embedding.Endpoint)
	ok := emb.Reachable || cfg.Embedding.Endpoint == ""
	for _, s := range toolStatuses {
		if !s.Found {
			ok = false
		}
	}
	return report{Tools: toolStatuses, Embedding: emb, OK: ok}
}

func checkEndpoint(ctx context.Context, endpoint string) endpointReport {
	if endpoint == "" {
		return endpointReport{Endpoint: "", Reachable: false, Error: "not configured"}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return endpointReport{Endpoint: endpoint, Error: err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return endpointReport{Endpoint: endpoint, Error: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()
	// Any HTTP response means the host is reachable; the exact status is
	// irrelevant because the embeddings route is a POST.
	return endpointReport{Endpoint: endpoint, Reachable: true}
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

	_, _ = fmt.Fprintln(out, "Embedding endpoint:")
	switch {
	case r.Embedding.Endpoint == "":
		_, _ = fmt.Fprintln(out, "  not configured")
	case r.Embedding.Reachable:
		_, _ = fmt.Fprintf(out, "  OK          %s\n", r.Embedding.Endpoint)
	default:
		_, _ = fmt.Fprintf(out, "  UNREACHABLE %s: %s\n", r.Embedding.Endpoint, r.Embedding.Error)
	}

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
