// Package visibility scores projected codeindex elements and connectors to
// decide which candidates are shown. It reproduces the intent of the legacy
// watch filter (changed files, user overrides, high-signal infrastructure,
// graph proximity, and noise penalties) but sources its inputs from the
// codeindex projection and embeddings instead of the watch_* graph.
package visibility

import (
	"context"
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/project"
)

// projectionStore is the storage surface needed to project a published snapshot.
type projectionStore interface {
	Snapshot(ctx context.Context, id string) (*pb.Snapshot, error)
	LoadGraph(ctx context.Context, snapshotID string) (*graph.Graph, error)
}

// ComputeForStore projects a published snapshot from storage and scores it.
func ComputeForStore(ctx context.Context, st projectionStore, snapshotID string, in Input, cfg Config) ([]Decision, error) {
	res, err := project.ProjectStore(ctx, st, snapshotID)
	if err != nil {
		return nil, err
	}
	in.Projection = res
	return Compute(in, cfg), nil
}

// Weights mirror the legacy watch visibility weights.
type Weights struct {
	Changed               float64
	Selected              float64
	UserShow              float64
	UserHide              float64
	HighSignalFact        float64
	RelationshipProximity float64
	DependencyFact        float64
	UtilityNoise          float64
	HighDegreeNoise       float64
}

// Config controls scoring.
type Config struct {
	CoreThresholdEnabled bool
	CoreThreshold        float64
	HighDegreeThreshold  int
	Weights              Weights
}

// DefaultConfig returns weights matching tld's global watch defaults.
func DefaultConfig() Config {
	return Config{
		CoreThresholdEnabled: true,
		CoreThreshold:        1,
		HighDegreeThreshold:  10,
		Weights: Weights{
			Changed:               100,
			Selected:              100,
			UserShow:              100,
			UserHide:              -100,
			HighSignalFact:        1.5,
			RelationshipProximity: 1,
			DependencyFact:        0.2,
			UtilityNoise:          -0.8,
			HighDegreeNoise:       -1.5,
		},
	}
}

// Input is everything scoring needs beyond the projection itself.
type Input struct {
	Projection   project.Result
	ChangedFiles map[string]bool
	ForceShow    map[string]bool
	ForceHide    map[string]bool
}

// Decision is the visibility outcome for one element or connector ref.
type Decision struct {
	Ref     string
	Kind    string
	Visible bool
	Score   float64
	Reason  string
}

var highSignalKinds = map[pb.FactKind]bool{
	pb.FactKind_FACT_KIND_DEPLOYABLE:        true,
	pb.FactKind_FACT_KIND_DATASTORE:         true,
	pb.FactKind_FACT_KIND_QUEUE:             true,
	pb.FactKind_FACT_KIND_TOPIC:             true,
	pb.FactKind_FACT_KIND_ROUTE:             true,
	pb.FactKind_FACT_KIND_ENTRYPOINT:        true,
	pb.FactKind_FACT_KIND_RPC:               true,
	pb.FactKind_FACT_KIND_CATALOG_COMPONENT: true,
	pb.FactKind_FACT_KIND_BRIDGE_HTTP:       true,
	pb.FactKind_FACT_KIND_SCHEMA:            true,
}

var utilityKinds = map[pb.FactKind]bool{
	pb.FactKind_FACT_KIND_ENV:              true,
	pb.FactKind_FACT_KIND_IMPORT:           true,
	pb.FactKind_FACT_KIND_WORKSPACE_MEMBER: true,
	pb.FactKind_FACT_KIND_CONFIG_REF:       true,
}

// Compute scores every projected element and connector. Elements are returned
// first (sorted by ref), then connectors. Output is deterministic.
func Compute(in Input, cfg Config) []Decision {
	elements := make([]project.Element, len(in.Projection.Elements))
	copy(elements, in.Projection.Elements)
	sort.Slice(elements, func(i, j int) bool { return elements[i].Ref < elements[j].Ref })

	degree := map[string]int{}
	adjacency := map[string]map[string]bool{}
	for _, c := range in.Projection.Connectors {
		degree[c.FromRef]++
		degree[c.ToRef]++
		if adjacency[c.FromRef] == nil {
			adjacency[c.FromRef] = map[string]bool{}
		}
		if adjacency[c.ToRef] == nil {
			adjacency[c.ToRef] = map[string]bool{}
		}
		adjacency[c.FromRef][c.ToRef] = true
		adjacency[c.ToRef][c.FromRef] = true
	}

	// Seeds: changed or user-shown elements propagate proximity to neighbors.
	seeds := map[string]bool{}
	for _, el := range elements {
		if in.ChangedFiles[el.FilePath] || in.ForceShow[el.Ref] {
			seeds[el.Ref] = true
		}
	}

	var out []Decision
	for _, el := range elements {
		score := 0.0
		reasons := ""

		if in.ChangedFiles[el.FilePath] {
			score += cfg.Weights.Changed
			reasons += "changed "
		}
		if in.ForceShow[el.Ref] {
			score += cfg.Weights.UserShow
			reasons += "user_show "
		}
		if in.ForceHide[el.Ref] {
			score += cfg.Weights.UserHide
			reasons += "user_hide "
		}
		if highSignalKinds[el.Kind] {
			score += cfg.Weights.HighSignalFact
			reasons += "high_signal "
		}
		if utilityKinds[el.Kind] {
			score += cfg.Weights.UtilityNoise
			reasons += "utility "
		}
		if el.Kind == pb.FactKind_FACT_KIND_DEPENDENCY {
			score += cfg.Weights.DependencyFact
			reasons += "dependency "
		}
		if !seeds[el.Ref] && neighborOfSeed(el.Ref, adjacency, seeds) {
			score += cfg.Weights.RelationshipProximity
			reasons += "proximity "
		}
		if cfg.HighDegreeThreshold > 0 && degree[el.Ref] >= cfg.HighDegreeThreshold {
			score += cfg.Weights.HighDegreeNoise
			reasons += "high_degree "
		}

		visible := true
		if cfg.CoreThresholdEnabled {
			visible = score >= cfg.CoreThreshold
		}
		if in.ForceShow[el.Ref] {
			visible = true
		}
		if in.ForceHide[el.Ref] {
			visible = false
		}
		out = append(out, Decision{Ref: el.Ref, Kind: "element", Visible: visible, Score: score, Reason: trim(reasons)})
	}

	connectors := make([]project.Connector, len(in.Projection.Connectors))
	copy(connectors, in.Projection.Connectors)
	sort.Slice(connectors, func(i, j int) bool { return connectors[i].Ref < connectors[j].Ref })
	for _, c := range connectors {
		visible := true
		if cfg.CoreThresholdEnabled {
			visible = c.Weight >= 1
		}
		out = append(out, Decision{Ref: c.Ref, Kind: "connector", Visible: visible, Score: c.Weight})
	}
	return out
}

func neighborOfSeed(ref string, adjacency map[string]map[string]bool, seeds map[string]bool) bool {
	for neighbor := range adjacency[ref] {
		if seeds[neighbor] {
			return true
		}
	}
	return false
}

func trim(s string) string {
	if len(s) > 0 && s[len(s)-1] == ' ' {
		return s[:len(s)-1]
	}
	return s
}
