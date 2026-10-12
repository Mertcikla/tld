// Package model holds private comparison analysis and assembly state.
// These types are cache implementation details, never RPC contracts.
package model

import (
	"encoding/json"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

type Diagram struct {
	RepositoryId         string                 `json:"repository_id,omitempty"`
	ComparisonKey        string                 `json:"comparison_key,omitempty"`
	Diff                 *pb.SnapshotDiff       `json:"diff,omitempty"`
	Nodes                []*Node                `json:"nodes,omitempty"`
	Edges                []*Edge                `json:"edges,omitempty"`
	Version              string                 `json:"version,omitempty"`
	Scene                *Assembly              `json:"scene,omitempty"`
	RequestedRadius      *uint32                `json:"requested_radius,omitempty"`
	WorkspaceFingerprint string                 `json:"workspace_fingerprint,omitempty"`
	ViewFilter           string                 `json:"view_filter,omitempty"`
	Warnings             []string               `json:"warnings,omitempty"`
	ManifestComplete     bool                   `json:"manifest_complete,omitempty"`
	Comparison           *pb.ImpactComparison   `json:"comparison,omitempty"`
	Files                []*pb.ImpactFile       `json:"files,omitempty"`
	Dependencies         []*pb.ImpactDependency `json:"dependencies,omitempty"`
}

type Node struct {
	Key       string            `json:"key,omitempty"`
	Path      string            `json:"path,omitempty"`
	Name      string            `json:"name,omitempty"`
	Change    pb.ChangeKind     `json:"change,omitempty"`
	Distance  uint32            `json:"distance,omitempty"`
	Symbols   *pb.CodeFactDelta `json:"symbols,omitempty"`
	Witnesses []*Witness        `json:"witnesses,omitempty"`
}

type Edge struct {
	FromKey string        `json:"from_key,omitempty"`
	ToKey   string        `json:"to_key,omitempty"`
	Change  pb.ChangeKind `json:"change,omitempty"`
	Weight  float64       `json:"weight,omitempty"`
}

type Witness struct {
	SnapshotId string   `json:"snapshot_id,omitempty"`
	Paths      []string `json:"paths,omitempty"`
}

type Metadata struct {
	Key          string             `json:"key,omitempty"`
	ObjectKind   string             `json:"object_kind,omitempty"`
	ViewId       int64              `json:"view_id,omitempty"`
	ResourceId   int64              `json:"resource_id,omitempty"`
	Name         string             `json:"name,omitempty"`
	Origin       pb.ImpactOrigin    `json:"origin,omitempty"`
	Grounding    pb.ImpactGrounding `json:"grounding,omitempty"`
	Role         pb.ImpactRole      `json:"role,omitempty"`
	Paths        []string           `json:"paths,omitempty"`
	Distance     uint32             `json:"distance,omitempty"`
	Witnesses    []*Witness         `json:"witnesses,omitempty"`
	Partial      bool               `json:"partial,omitempty"`
	EndpointKeys []string           `json:"endpoint_keys,omitempty"`
}

type Decision struct {
	Stage              string   `json:"stage,omitempty"`
	Action             string   `json:"action,omitempty"`
	Key                string   `json:"key,omitempty"`
	Reason             string   `json:"reason,omitempty"`
	RepresentativeKeys []string `json:"representative_keys,omitempty"`
}

type Live struct {
	Diagram     *Diagram `json:"diagram,omitempty"`
	Watching    bool     `json:"watching,omitempty"`
	Error       string   `json:"error,omitempty"`
	GitBranch   string   `json:"git_branch,omitempty"`
	GitRevision string   `json:"git_revision,omitempty"`
}

type Overlay struct {
	Change       pb.ChangeKind            `json:"change,omitempty"`
	Path         string                   `json:"path,omitempty"`
	LinesAdded   *uint32                  `json:"lines_added,omitempty"`
	LinesRemoved *uint32                  `json:"lines_removed,omitempty"`
	Symbols      []*pb.ImpactSymbolChange `json:"symbols,omitempty"`
	Contained    bool                     `json:"contained,omitempty"`
}

type Placement struct {
	Element *diagv1.PlacedElement `json:"element,omitempty"`
	Overlay *Overlay              `json:"overlay,omitempty"`
}

type ViewContent struct {
	Placements []*Placement        `json:"placements,omitempty"`
	Connectors []*diagv1.Connector `json:"connectors,omitempty"`
}

type Assembly struct {
	Tree                 []*diagv1.View                  `json:"tree,omitempty"`
	Views                map[string]*ViewContent         `json:"views,omitempty"`
	Navigations          []*diagv1.ElementNavigationInfo `json:"navigations,omitempty"`
	FallbackViewId       int64                           `json:"fallback_view_id,omitempty"`
	SchemaVersion        string                          `json:"schema_version,omitempty"`
	RepositoryId         string                          `json:"repository_id,omitempty"`
	ComparisonKey        string                          `json:"comparison_key,omitempty"`
	Version              string                          `json:"version,omitempty"`
	FromGitRevision      string                          `json:"from_git_revision,omitempty"`
	ToGitRevision        string                          `json:"to_git_revision,omitempty"`
	AuthoredViewIds      []int64                         `json:"authored_view_ids,omitempty"`
	Provenance           map[string]*Metadata            `json:"provenance,omitempty"`
	RequestedRadius      uint32                          `json:"requested_radius,omitempty"`
	EffectiveRadius      uint32                          `json:"effective_radius,omitempty"`
	Warnings             []string                        `json:"warnings,omitempty"`
	ManifestComplete     bool                            `json:"manifest_complete,omitempty"`
	OmittedContext       []string                        `json:"omitted_context,omitempty"`
	WorkspaceFingerprint string                          `json:"workspace_fingerprint,omitempty"`
	Decisions            []*Decision                     `json:"decisions,omitempty"`
	Comparison           *pb.ImpactComparison            `json:"comparison,omitempty"`
	Files                []*pb.ImpactFile                `json:"files,omitempty"`
	Matches              []*pb.ImpactMatch               `json:"matches,omitempty"`
	Dependencies         []*pb.ImpactDependency          `json:"dependencies,omitempty"`
	IndexedSymbols       []*pb.ImpactSymbolChange        `json:"indexed_symbols,omitempty"`
	BaseSnapshotId       string                          `json:"base_snapshot_id,omitempty"`
	HeadSnapshotId       string                          `json:"head_snapshot_id,omitempty"`
	AuditReferences      map[string]*AuditReference      `json:"audit_references,omitempty"`
}

type AuditReference struct {
	Name     string `json:"name,omitempty"`
	ViewName string `json:"view_name,omitempty"`
}

func (x *Diagram) GetRepositoryId() string {
	if x != nil {
		return x.RepositoryId
	}
	return ""
}

func (x *Diagram) GetComparisonKey() string {
	if x != nil {
		return x.ComparisonKey
	}
	return ""
}

func (x *Diagram) GetDiff() *pb.SnapshotDiff {
	if x != nil {
		return x.Diff
	}
	return nil
}

func (x *Diagram) GetNodes() []*Node {
	if x != nil {
		return x.Nodes
	}
	return nil
}

func (x *Diagram) GetEdges() []*Edge {
	if x != nil {
		return x.Edges
	}
	return nil
}

func (x *Diagram) GetScene() *Assembly {
	if x != nil {
		return x.Scene
	}
	return nil
}

func (x *Diagram) GetRequestedRadius() uint32 {
	if x != nil && x.RequestedRadius != nil {
		return *x.RequestedRadius
	}
	return 0
}

func (x *Diagram) GetWarnings() []string {
	if x != nil {
		return x.Warnings
	}
	return nil
}

func (x *Diagram) GetManifestComplete() bool {
	if x != nil {
		return x.ManifestComplete
	}
	return false
}

func (x *Node) GetKey() string {
	if x != nil {
		return x.Key
	}
	return ""
}

func (x *Node) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

func (x *Node) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Node) GetChange() pb.ChangeKind {
	if x != nil {
		return x.Change
	}
	return 0
}

func (x *Node) GetDistance() uint32 {
	if x != nil {
		return x.Distance
	}
	return 0
}

func (x *Node) GetSymbols() *pb.CodeFactDelta {
	if x != nil {
		return x.Symbols
	}
	return nil
}

func (x *Edge) GetFromKey() string {
	if x != nil {
		return x.FromKey
	}
	return ""
}

func (x *Edge) GetToKey() string {
	if x != nil {
		return x.ToKey
	}
	return ""
}

func (x *Metadata) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Metadata) GetOrigin() pb.ImpactOrigin {
	if x != nil {
		return x.Origin
	}
	return 0
}

func (x *Metadata) GetGrounding() pb.ImpactGrounding {
	if x != nil {
		return x.Grounding
	}
	return 0
}

func (x *Metadata) GetRole() pb.ImpactRole {
	if x != nil {
		return x.Role
	}
	return 0
}

func (x *Metadata) GetDistance() uint32 {
	if x != nil {
		return x.Distance
	}
	return 0
}

func (x *Metadata) GetEndpointKeys() []string {
	if x != nil {
		return x.EndpointKeys
	}
	return nil
}

func (x *Overlay) GetChange() pb.ChangeKind {
	if x != nil {
		return x.Change
	}
	return 0
}

func (x *Overlay) GetPath() string {
	if x != nil {
		return x.Path
	}
	return ""
}

func (x *Overlay) GetLinesAdded() uint32 {
	if x != nil && x.LinesAdded != nil {
		return *x.LinesAdded
	}
	return 0
}

func (x *Overlay) GetLinesRemoved() uint32 {
	if x != nil && x.LinesRemoved != nil {
		return *x.LinesRemoved
	}
	return 0
}

func (x *Overlay) GetSymbols() []*pb.ImpactSymbolChange {
	if x != nil {
		return x.Symbols
	}
	return nil
}

func (x *Overlay) GetContained() bool {
	if x != nil {
		return x.Contained
	}
	return false
}

func (x *Placement) GetElement() *diagv1.PlacedElement {
	if x != nil {
		return x.Element
	}
	return nil
}

func (x *Placement) GetOverlay() *Overlay {
	if x != nil {
		return x.Overlay
	}
	return nil
}

func (x *ViewContent) GetPlacements() []*Placement {
	if x != nil {
		return x.Placements
	}
	return nil
}

func (x *ViewContent) GetConnectors() []*diagv1.Connector {
	if x != nil {
		return x.Connectors
	}
	return nil
}

func (x *Assembly) GetTree() []*diagv1.View {
	if x != nil {
		return x.Tree
	}
	return nil
}

func (x *Assembly) GetViews() map[string]*ViewContent {
	if x != nil {
		return x.Views
	}
	return nil
}

func (x *Assembly) GetNavigations() []*diagv1.ElementNavigationInfo {
	if x != nil {
		return x.Navigations
	}
	return nil
}

func (x *Assembly) GetFallbackViewId() int64 {
	if x != nil {
		return x.FallbackViewId
	}
	return 0
}

func (x *Assembly) GetSchemaVersion() string {
	if x != nil {
		return x.SchemaVersion
	}
	return ""
}

func (x *Assembly) GetAuthoredViewIds() []int64 {
	if x != nil {
		return x.AuthoredViewIds
	}
	return nil
}

func (x *Assembly) GetComparison() *pb.ImpactComparison {
	if x != nil {
		return x.Comparison
	}
	return nil
}

func (x *Assembly) GetFiles() []*pb.ImpactFile {
	if x != nil {
		return x.Files
	}
	return nil
}

// Clone isolates selection from the immutable cached result. All fields are
// JSON values; protobuf members use their public fields for this private copy.
func Clone[T any](value *T) *T {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		panic(err)
	}
	return &out
}
