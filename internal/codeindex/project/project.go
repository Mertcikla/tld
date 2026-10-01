// Package project deterministically converts a codeindex graph into candidate
// diagram elements and connectors. It performs no naming, role inference, or
// layout: those are downstream concerns. Element and connector refs are the
// codeindex logical keys, which are the canonical cross-snapshot identity.
package project

import (
	"context"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// Element is a candidate diagram element derived from a CodeFact.
type Element struct {
	Ref        string           `json:"ref"`
	Name       string           `json:"name"`
	Kind       pb.FactKind      `json:"kind"`
	Language   string           `json:"language,omitempty"`
	Repository string           `json:"repository,omitempty"`
	FilePath   string           `json:"file_path,omitempty"`
	SymbolKey  string           `json:"symbol_key,omitempty"`
	Signature  string           `json:"signature,omitempty"`
	Anchor     *pb.SourceAnchor `json:"anchor,omitempty"`
}

// Connector is a candidate diagram connector derived from aggregated EdgeFacts.
type Connector struct {
	Ref     string      `json:"ref"`
	Kind    pb.EdgeKind `json:"kind"`
	FromRef string      `json:"from_ref"`
	ToRef   string      `json:"to_ref"`
	Weight  float64     `json:"weight"`
}

// Result is the deterministic projection of one snapshot.
type Result struct {
	SnapshotID string      `json:"snapshot_id"`
	Elements   []Element   `json:"elements"`
	Connectors []Connector `json:"connectors"`
}

// Project converts a snapshot graph into candidates. Elements are emitted for
// every CodeFact; connectors aggregate EdgeFacts by logical key and are only
// emitted when both endpoints resolve to a projected element.
func Project(snap *pb.Snapshot, g *graph.Graph) Result {
	res := Result{}
	if snap != nil {
		res.SnapshotID = snap.Id
	}
	if g == nil {
		return res
	}

	byRef := map[string]*pb.CodeFact{}
	symbolToRef := map[string]string{}
	for _, f := range g.Facts {
		ref := logicalFactRef(f)
		byRef[ref] = f
		if f.SymbolKey != "" {
			if _, exists := symbolToRef[f.SymbolKey]; !exists {
				symbolToRef[f.SymbolKey] = ref
			}
		}
	}

	refs := make([]string, 0, len(byRef))
	for ref := range byRef {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	res.Elements = make([]Element, 0, len(refs))
	for _, ref := range refs {
		f := byRef[ref]
		el := Element{
			Ref:        ref,
			Name:       f.Name,
			Kind:       f.Kind,
			Language:   f.Language,
			Repository: f.RepositoryId,
			SymbolKey:  f.SymbolKey,
			Signature:  f.Signature,
			Anchor:     f.Anchor,
		}
		if f.Anchor != nil {
			el.FilePath = f.Anchor.Path
		}
		res.Elements = append(res.Elements, el)
	}

	type aggregate struct {
		ref     string
		kind    pb.EdgeKind
		fromRef string
		toRef   string
		weight  float64
		count   int
	}
	aggregates := map[string]*aggregate{}
	for _, e := range g.EdgeFacts {
		from := byRef[logicalFactRefByID(g, e.FromFactId)]
		if from == nil {
			continue
		}
		toRef := ""
		if e.ToFactId != "" {
			if to := g.Facts[e.ToFactId]; to != nil {
				toRef = logicalFactRef(to)
			}
		}
		if toRef == "" && e.TargetSymbolKey != "" {
			toRef = symbolToRef[e.TargetSymbolKey]
		}
		if toRef == "" || toRef == logicalFactRef(from) {
			continue
		}
		key := logicalEdgeRef(e)
		agg := aggregates[key]
		if agg == nil {
			aggregates[key] = &aggregate{ref: key, kind: e.Kind, fromRef: logicalFactRef(from), toRef: toRef, weight: e.Weight, count: 1}
			continue
		}
		agg.weight += e.Weight
		agg.count++
	}
	edgeRefs := make([]string, 0, len(aggregates))
	for ref := range aggregates {
		edgeRefs = append(edgeRefs, ref)
	}
	sort.Strings(edgeRefs)
	res.Connectors = make([]Connector, 0, len(edgeRefs))
	for _, ref := range edgeRefs {
		agg := aggregates[ref]
		weight := agg.weight
		if weight == 0 {
			weight = float64(agg.count)
		}
		res.Connectors = append(res.Connectors, Connector{
			Ref:     agg.ref,
			Kind:    agg.kind,
			FromRef: agg.fromRef,
			ToRef:   agg.toRef,
			Weight:  weight,
		})
	}
	return res
}

// ProjectStore loads a published snapshot and projects it.
func ProjectStore(ctx context.Context, st interface {
	Snapshot(ctx context.Context, id string) (*pb.Snapshot, error)
	LoadGraph(ctx context.Context, snapshotID string) (*graph.Graph, error)
}, snapshotID string) (Result, error) {
	snap, err := st.Snapshot(ctx, snapshotID)
	if err != nil {
		return Result{}, err
	}
	g, err := st.LoadGraph(ctx, snapshotID)
	if err != nil {
		return Result{}, err
	}
	return Project(snap, g), nil
}

func logicalFactRef(f *pb.CodeFact) string {
	if f == nil {
		return ""
	}
	if f.LogicalKey != "" {
		return f.LogicalKey
	}
	return f.Id
}

func logicalFactRefByID(g *graph.Graph, factID string) string {
	f := g.Facts[factID]
	if f == nil {
		return ""
	}
	return logicalFactRef(f)
}

func logicalEdgeRef(e *pb.EdgeFact) string {
	if e.LogicalKey != "" {
		return e.LogicalKey
	}
	return e.Id
}

// KindLabel returns a stable, human-readable fact kind label without
// introducing naming policy into the projection itself.
func KindLabel(kind pb.FactKind) string {
	return strings.TrimPrefix(kind.String(), "FACT_KIND_")
}

// KindLabelEdge returns a stable, human-readable edge kind label.
func KindLabelEdge(kind pb.EdgeKind) string {
	return strings.TrimPrefix(kind.String(), "EDGE_KIND_")
}
