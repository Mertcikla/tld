// Package parity runs the ported codeindex pipeline over a repository and
// captures a stable summary of what it extracted. It is used to detect
// extraction regressions and, when a codeindex baseline is supplied, to compare
// against the upstream engine.
package parity

import (
	"context"
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
)

// Report is a deterministic summary of one snapshot build. It deliberately
// excludes random snapshot/repository ids so it can be compared across runs and
// engines.
type Report struct {
	Sources      int               `json:"sources"`
	Projects     int               `json:"projects"`
	Warnings     int               `json:"warnings"`
	Facts        int               `json:"facts"`
	Edges        int               `json:"edges"`
	FactsByKind  map[string]int    `json:"facts_by_kind"`
	EdgesByKind  map[string]int    `json:"edges_by_kind"`
	FactsByPath  map[string]int    `json:"facts_by_path"`
	ToolVersions map[string]string `json:"tool_versions,omitempty"`
}

// Run builds a snapshot for root and summarises it.
func Run(ctx context.Context, root string, cfg config.Config) (Report, error) {
	pipeline := indexer.Pipeline{Config: cfg}
	req := &pb.IndexRequest{Directory: root}
	snap, g, err := pipeline.Build(ctx, req, nil)
	if err != nil {
		return Report{}, err
	}
	return Summarize(snap, g), nil
}

// Summarize reduces a built snapshot and graph to a comparable Report.
func Summarize(snap *pb.Snapshot, g *graph.Graph) Report {
	r := Report{
		FactsByKind:  map[string]int{},
		EdgesByKind:  map[string]int{},
		FactsByPath:  map[string]int{},
		ToolVersions: map[string]string{},
	}
	if snap != nil {
		r.Sources = len(snap.Sources)
		r.Projects = len(snap.Projects)
		r.Warnings = len(snap.Warnings)
		for name, version := range snap.ToolVersions {
			r.ToolVersions[name] = version
		}
	}
	if g == nil {
		return r
	}
	for _, f := range g.Facts {
		r.Facts++
		r.FactsByKind[pb.FactKind_name[int32(f.Kind)]]++
		if f.Anchor != nil {
			r.FactsByPath[f.Anchor.Path]++
		}
	}
	for _, e := range g.EdgeFacts {
		r.Edges++
		r.EdgesByKind[pb.EdgeKind_name[int32(e.Kind)]]++
	}
	return r
}

// Diff is one field-level mismatch between two reports.
type Diff struct {
	Field string
	Want  any
	Got   any
}

// Compare returns the count differences between want and got. Tool versions are
// intentionally excluded from the comparison.
func Compare(want, got Report) []Diff {
	var diffs []Diff
	add := func(field string, w, g any) { diffs = append(diffs, Diff{field, w, g}) }
	if want.Sources != got.Sources {
		add("sources", want.Sources, got.Sources)
	}
	if want.Projects != got.Projects {
		add("projects", want.Projects, got.Projects)
	}
	if want.Facts != got.Facts {
		add("facts", want.Facts, got.Facts)
	}
	if want.Edges != got.Edges {
		add("edges", want.Edges, got.Edges)
	}
	for _, kind := range sortedKeys(want.FactsByKind, got.FactsByKind) {
		if want.FactsByKind[kind] != got.FactsByKind[kind] {
			add("facts_by_kind."+kind, want.FactsByKind[kind], got.FactsByKind[kind])
		}
	}
	for _, kind := range sortedKeys(want.EdgesByKind, got.EdgesByKind) {
		if want.EdgesByKind[kind] != got.EdgesByKind[kind] {
			add("edges_by_kind."+kind, want.EdgesByKind[kind], got.EdgesByKind[kind])
		}
	}
	for _, path := range sortedKeys(want.FactsByPath, got.FactsByPath) {
		if want.FactsByPath[path] != got.FactsByPath[path] {
			add("facts_by_path."+path, want.FactsByPath[path], got.FactsByPath[path])
		}
	}
	return diffs
}

func sortedKeys(a, b map[string]int) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
