package impact

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"github.com/mertcikla/tld/v2/internal/layout"
	"github.com/mertcikla/tld/v2/internal/sourcelink"
	"google.golang.org/protobuf/proto"
)

// SceneSchemaVersion identifies the portable scene contract. Bump it when the
// scene shape changes so obsolete comparisons require regeneration.
const SceneSchemaVersion = PipelineVersion

func transientName(p string) string {
	dir, base := path.Split(p)
	parent := strings.TrimSuffix(dir, "/")
	if i := strings.LastIndex(parent, "/"); i >= 0 {
		parent = parent[i+1:]
	}
	if parent == "" || parent == "." {
		return base
	}
	return parent + "/" + base
}

// Scene returns the frozen comparison scene. Artifacts without a canonical
// scene are rejected; historical reads never consult the current workspace.
func (s Service) Scene(ctx context.Context, diagram *model.Diagram) (*model.Assembly, error) {
	if diagram == nil {
		return &model.Assembly{SchemaVersion: SceneSchemaVersion}, nil
	}
	if diagram.Scene != nil && diagram.Scene.SchemaVersion == PipelineVersion {
		return model.Clone(diagram.Scene), nil
	}
	return nil, fmt.Errorf("comparison schema is obsolete; run compare again")
}

// sceneEvidence holds captured analysis and shared source-link conversion helpers.
type sceneEvidence struct {
	diagram   *model.Diagram
	workspace core.ExploreData
	root      string
	mappings  []cstore.ResourceMapping
	store     *cstore.Store
	ctx       context.Context
	nodes     map[string]*model.Node
	sources   map[string]*pb.SourceChange
}

// connectorChangeTag names the workspace-external change state of a scene
// connector. The tag rides the existing connector payload, so the canvas can
// style new/changed edges with no proto change. Empty for unchanged pairs.
func connectorChangeTag(change pb.ChangeKind) string {
	switch change {
	case pb.ChangeKind_CHANGE_KIND_ADDED:
		return "change:added"
	case pb.ChangeKind_CHANGE_KIND_REMOVED:
		return "change:removed"
	case pb.ChangeKind_CHANGE_KIND_MODIFIED:
		return "change:modified"
	default:
		return ""
	}
}

func appendTagUnique(tags []string, tag string) []string {
	if tag == "" {
		return tags
	}
	for _, existing := range tags {
		if existing == tag {
			return tags
		}
	}
	return append(tags, tag)
}

// linkBase parses a source link into its repo-relative base path and whether
// it addresses a folder (trailing slash) rather than a file. Absolute
// checkout paths are relativized against the repository root.
func (b *sceneEvidence) linkBase(link string) (base string, isFolder bool, ok bool) {
	parsed := sourcelink.Parse(strings.TrimSpace(link))
	raw := strings.ReplaceAll(strings.TrimSpace(parsed.BasePath), "\\", "/")
	if raw == "" {
		return "", false, false
	}
	isFolder = strings.HasSuffix(raw, "/")
	rel, relOK := b.repoRelativePath(raw)
	if !relOK || rel == "" {
		return "", false, false
	}
	return strings.TrimSuffix(rel, "/"), isFolder, true
}

// sortedKeys returns the sorted keys of a string-keyed set or index, keeping
// scene assembly deterministic across runs.
func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// repoRelativePath normalizes an authored link's base path to the
// repository-relative form diagram nodes carry. Absolute paths that live under
// the compared repository's checkout are stripped to their relative form;
// relative paths are kept. Anything that resolves outside the repository
// returns false and never matches.
func (b *sceneEvidence) repoRelativePath(base string) (string, bool) {
	if strings.HasPrefix(base, "/") {
		if strings.TrimSpace(b.root) == "" {
			return "", false
		}
		root := strings.TrimSuffix(strings.ReplaceAll(b.root, "\\", "/"), "/")
		rel, ok := strings.CutPrefix(base, root+"/")
		if !ok {
			return "", false
		}
		base = rel
	}
	cleaned := path.Clean(base)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}

// symbolAnchorMatches reports whether a changed node carries a symbol change
// on the authored anchor: a symbol anchor matches by declaration name, a line
// anchor by overlap between the author's line range and a changed symbol's
// anchor lines. Authored line anchors are 1-based (editor convention) while
// stored fact anchors are 0-based, so the author range is shifted down first.
func symbolAnchorMatches(node *model.Node, anchor sourcelink.Anchor) bool {
	if node == nil {
		return false
	}
	for _, kind := range []pb.ChangeKind{
		pb.ChangeKind_CHANGE_KIND_ADDED,
		pb.ChangeKind_CHANGE_KIND_REMOVED,
		pb.ChangeKind_CHANGE_KIND_MODIFIED,
	} {
		for _, fact := range factsByKind(node.GetSymbols(), kind) {
			if fact == nil {
				continue
			}
			switch anchor.Kind {
			case sourcelink.AnchorSymbol:
				if anchor.Symbol != "" && fact.GetName() == anchor.Symbol {
					return true
				}
			case sourcelink.AnchorLine:
				if anchor.StartLine <= 0 {
					continue
				}
				end := anchor.EndLine
				if end <= 0 {
					end = anchor.StartLine
				}
				if factAnchor := fact.GetAnchor(); factAnchor != nil &&
					int(factAnchor.GetStartLine()) <= end-1 && anchor.StartLine-1 <= int(factAnchor.GetEndLine()) {
					return true
				}
			}
		}
	}
	return false
}

// overlay shapes a diagram node into the transient annotation the canvas
// renders. Context nodes keep their hop distance so clients can scope them.
// Symbol changes are carried structured: a reader labels them itself, so no
// pre-shaped text has to stay in sync with the underlying fact. Every overlay
// this helper shapes is a direct hit on its own path; roll-ups set CONTAINED
// themselves.
func (b *sceneEvidence) overlay(node *model.Node) *model.Overlay {
	if node == nil {
		return nil
	}
	overlay := &model.Overlay{Change: node.GetChange(), Path: node.GetPath()}
	if source := b.sources[node.GetPath()]; source != nil {
		overlay.LinesAdded = source.LinesAdded
		overlay.LinesRemoved = source.LinesRemoved
	}
	delta := node.GetSymbols()
	for _, kind := range []pb.ChangeKind{
		pb.ChangeKind_CHANGE_KIND_ADDED,
		pb.ChangeKind_CHANGE_KIND_REMOVED,
		pb.ChangeKind_CHANGE_KIND_MODIFIED,
	} {
		for _, fact := range factsByKind(delta, kind) {
			if fact == nil {
				continue
			}
			overlay.Symbols = append(overlay.Symbols, symbolChange(kind, fact))
		}
	}
	return overlay
}

// symbolChange keeps only the portable identity of a changed fact: name, kind,
// source path and lines. Snapshot provenance, qualified SCIP
// names, evidence, and imports are left behind so a scene carries no
// index-only weight.
func symbolChange(kind pb.ChangeKind, fact *pb.CodeFact) *pb.ImpactSymbolChange {
	change := &pb.ImpactSymbolChange{
		Change: kind,
		Name:   fact.GetName(),
		Kind:   fact.GetKind(),
	}
	if anchor := fact.GetAnchor(); anchor != nil {
		change.Path, change.StartLine, change.EndLine = anchor.GetPath(), anchor.GetStartLine(), anchor.GetEndLine()
	}
	return change
}

func sceneView(view core.ViewTreeNode) *diagv1.View {
	out := &diagv1.View{
		Id: int32(view.ID), OwnerElementId: int32Pointer(view.OwnerElementID), Name: view.Name,
		Description: view.Description, LevelLabel: view.LevelLabel,
		Tags: append([]string(nil), view.Tags...), Level: int32(view.Level), Depth: int32(view.Depth),
		ParentViewId: int32Pointer(view.ParentViewID),
		Children:     make([]*diagv1.View, 0, len(view.Children)),
	}
	for _, child := range view.Children {
		out.Children = append(out.Children, sceneView(child))
	}
	return out
}

func scenePlacement(placement core.PlacedElement) *model.Placement {
	element := &diagv1.PlacedElement{
		Id: int32(placement.ID), ViewId: int32(placement.ViewID), ElementId: int32(placement.ElementID),
		PositionX: placement.PositionX, PositionY: placement.PositionY,
		Name: placement.Name, Description: placement.Description, Kind: placement.Kind,
		Technology: placement.Technology, Url: placement.URL, LogoUrl: placement.LogoURL,
		Tags: append([]string(nil), placement.Tags...),
		Repo: placement.Repo, RepositoryId: placement.RepositoryID, Branch: placement.Branch,
		FilePath: placement.FilePath, Language: placement.Language,
		HasView: placement.HasView, ViewLabel: placement.ViewLabel,
		BypassNoiseGate: placement.BypassNoiseGate,
	}
	for _, connector := range placement.TechnologyConnectors {
		slug := connector.Slug
		element.TechnologyLinks = append(element.TechnologyLinks, &diagv1.TechnologyLink{
			Type: connector.Type, Slug: &slug, Label: connector.Label, IsPrimaryIcon: connector.IsPrimaryIcon,
		})
	}
	return &model.Placement{Element: element}
}

func sceneConnector(connector core.Connector) *diagv1.Connector {
	return &diagv1.Connector{
		Id: int32(connector.ID), ViewId: int32(connector.ViewID),
		SourceElementId: int32(connector.SourceElementID), TargetElementId: int32(connector.TargetElementID),
		Label: connector.Label, Description: connector.Description, Relationship: connector.Relationship,
		Direction: connector.Direction, Style: connector.Style, Url: connector.URL,
		SourceHandle: connector.SourceHandle, TargetHandle: connector.TargetHandle,
		Tags: append([]string(nil), connector.Tags...),
	}
}

func sceneNavigation(link core.ViewConnector) *diagv1.ElementNavigationInfo {
	return &diagv1.ElementNavigationInfo{
		Id: int32(link.ID), ElementId: int32Pointer(link.ElementID),
		FromViewId: int32(link.FromViewID), ToViewId: int32(link.ToViewID),
		ToViewName: link.ToViewName, RelationType: link.RelationType,
	}
}

// int32Pointer narrows an optional int64 id into the workspace proto's int32
// id space. Scene ids fit int32: workspace rows and transient negatives.
func int32Pointer(value *int64) *int32 {
	if value == nil {
		return nil
	}
	narrowed := int32(*value)
	return &narrowed
}

// adjustSceneConnectorHandles re-attaches connectors to the handles that yield
// the shortest anchor distance for the scene's final placements, mirroring the
// map pipeline's "Adjust Connectors" pass.
func adjustSceneConnectorHandles(placements []*model.Placement, connectors []*diagv1.Connector) {
	if len(connectors) == 0 {
		return
	}
	positions := make(map[int64]layout.Placement, len(placements))
	for _, placement := range placements {
		element := placement.GetElement()
		positions[int64(element.GetElementId())] = layout.Placement{ElementID: int64(element.GetElementId()), X: element.GetPositionX(), Y: element.GetPositionY()}
	}
	for _, connector := range connectors {
		source, sourceOK := positions[int64(connector.GetSourceElementId())]
		target, targetOK := positions[int64(connector.GetTargetElementId())]
		if !sourceOK || !targetOK {
			continue
		}
		sourceHandle, targetHandle := layout.ChooseConnectorHandles(source, target)
		connector.SourceHandle = proto.String(sourceHandle)
		connector.TargetHandle = proto.String(targetHandle)
	}
}

func factsByKind(delta *pb.CodeFactDelta, kind pb.ChangeKind) []*pb.CodeFact {
	if delta == nil {
		return nil
	}
	switch kind {
	case pb.ChangeKind_CHANGE_KIND_ADDED:
		return delta.GetAdded()
	case pb.ChangeKind_CHANGE_KIND_REMOVED:
		return delta.GetRemoved()
	case pb.ChangeKind_CHANGE_KIND_MODIFIED:
		return delta.GetModified()
	default:
		return nil
	}
}

func filePathValue(path *string) string {
	if path == nil {
		return ""
	}
	return *path
}

func normalizeScenePath(path string) string {
	return strings.TrimSuffix(strings.ReplaceAll(path, "\\", "/"), "/")
}

// pruneScopedSceneTree rebuilds the scene tree from the views that kept
// placements, preserving structural ancestors so the hierarchy traverses.
// Views in kept carry placements; ancestors without placements stay as
// transparent nodes and their missing content renders as children only.
func pruneScopedSceneTree(nodes []*diagv1.View, kept map[string]map[int32]bool) []*diagv1.View {
	out := make([]*diagv1.View, 0, len(nodes))
	for _, view := range nodes {
		if view == nil {
			continue
		}
		children := pruneScopedSceneTree(view.GetChildren(), kept)
		if len(kept[strconv.FormatInt(int64(view.GetId()), 10)]) == 0 && len(children) == 0 {
			continue
		}
		view.Children = children
		out = append(out, view)
	}
	return out
}
