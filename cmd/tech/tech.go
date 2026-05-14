package tech

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"text/tabwriter"

	"github.com/mertcikla/tld/v2/internal/tech"
	"github.com/spf13/cobra"
)

const defaultLimit = 50
const maxLimit = 500

type catalogPage struct {
	Items  []tech.CatalogEntry `json:"items"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
	Count  int                 `json:"count"`
	Total  int                 `json:"total"`
	Next   string              `json:"next,omitempty"`
	Prev   string              `json:"prev,omitempty"`
}

// NewTechCmd creates commands for inspecting the embedded technology catalog.
func NewTechCmd() *cobra.Command {
	var limit int
	var offset int

	cmd := &cobra.Command{
		Use:   "tech",
		Short: "Inspect the embedded technology catalog",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if limit <= 0 {
				return fmt.Errorf("--limit must be greater than 0")
			}
			if limit > maxLimit {
				return fmt.Errorf("--limit must be %d or less", maxLimit)
			}
			if offset < 0 {
				return fmt.Errorf("--offset must be 0 or greater")
			}
			page := buildCatalogPage(tech.Catalog(), limit, offset)
			if wantsJSON(cmd) {
				return writeJSON(cmd, page)
			}
			renderCatalogPage(cmd, page)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", defaultLimit, "number of catalog entries to return")
	cmd.Flags().IntVar(&offset, "offset", 0, "zero-based catalog entry offset")
	cmd.AddCommand(newSuggestCmd())
	return cmd
}

func newSuggestCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "suggest <name>",
		Short: "Suggest similar catalog technologies",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit <= 0 {
				return fmt.Errorf("--limit must be greater than 0")
			}
			if limit > maxLimit {
				return fmt.Errorf("--limit must be %d or less", maxLimit)
			}
			suggestions := tech.SuggestSimilar(args[0], limit)
			if wantsJSON(cmd) {
				payload := struct {
					Query       string   `json:"query"`
					Suggestions []string `json:"suggestions"`
				}{Query: args[0], Suggestions: suggestions}
				return writeJSON(cmd, payload)
			}
			for _, suggestion := range suggestions {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), suggestion)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 5, "maximum number of suggestions")
	return cmd
}

func buildCatalogPage(items []tech.CatalogEntry, limit, offset int) catalogPage {
	total := len(items)
	start := min(offset, total)
	end := min(start+limit, total)
	page := catalogPage{
		Items:  items[start:end],
		Limit:  limit,
		Offset: start,
		Count:  end - start,
		Total:  total,
	}
	if end < total {
		page.Next = fmt.Sprintf("tld tech --limit %d --offset %d", limit, end)
	}
	if start > 0 {
		prevOffset := int(math.Max(0, float64(start-limit)))
		page.Prev = fmt.Sprintf("tld tech --limit %d --offset %d", limit, prevOffset)
	}
	return page
}

func renderCatalogPage(cmd *cobra.Command, page catalogPage) {
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SLUG\tNAME\tSHORT")
	for _, item := range page.Items {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", item.Slug, item.Name, item.NameShort)
	}
	_ = w.Flush()

	if page.Total == 0 {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "showing 0 of 0")
		return
	}
	last := page.Offset + page.Count
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "showing %d-%d of %d (limit %d offset %d)\n", page.Offset+1, last, page.Total, page.Limit, page.Offset)
	if page.Prev != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "prev: %s\n", page.Prev)
	}
	if page.Next != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "next: %s\n", page.Next)
	}
}

func wantsJSON(cmd *cobra.Command) bool {
	flag := cmd.Root().PersistentFlags().Lookup("format")
	return flag != nil && strings.EqualFold(flag.Value.String(), "json")
}

func compactJSON(cmd *cobra.Command) bool {
	flag := cmd.Root().PersistentFlags().Lookup("compact")
	return flag != nil && flag.Value.String() == "true"
}

func writeJSON(cmd *cobra.Command, payload any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	if !compactJSON(cmd) {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(payload)
}
