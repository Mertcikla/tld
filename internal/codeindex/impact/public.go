package impact

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	"github.com/mertcikla/tld/v2/internal/sourcelink"
	"google.golang.org/protobuf/proto"
)

func (a *sceneAssembly) captureMatch(element *diagv1.PlacedElement, match anchorMatch, outcome, reason string) {
	kind := "file"
	if match.contained {
		kind = "folder"
	}
	if match.anchor.Kind == sourcelink.AnchorSymbol || match.partial {
		kind = "symbol"
		if match.anchor.StartLine > 0 {
			kind = "line"
		}
	}
	if a.scene.Provenance[PlacementKey(int64(element.ViewId), int64(element.ElementId))].GetOrigin() == pb.ImpactOrigin_IMPACT_ORIGIN_RAW {
		kind = "fallback"
	}
	if outcome == "accepted" {
		switch kind {
		case "folder":
			reason = "Changed path is within linked folder scope"
		case "symbol":
			reason = "Verified symbol link represents changes in the linked file"
		case "line":
			reason = "Changed symbol overlaps the linked line range"
		case "file":
			reason = "Source path matches the linked file"
		case "fallback":
			reason = "No complete authored or current mapped representative"
		}
	}
	add := func(path, side string) {
		a.scene.Matches = append(a.scene.Matches, &pb.ImpactMatch{Path: path,
			Placement:  &pb.ImpactPlacementRef{ViewId: element.ViewId, ElementId: element.ElementId},
			SourceLink: element.GetFilePath(), Scope: match.scope, Kind: kind, Partial: match.partial,
			Side: side, Outcome: outcome, Reason: reason})
	}
	for _, node := range match.nodes {
		base, head := false, false
		for _, witness := range node.Witnesses {
			base = base || witness.SnapshotId == a.helper.diagram.GetDiff().GetFromSnapshotId()
			head = head || witness.SnapshotId == a.helper.diagram.GetDiff().GetToSnapshotId()
		}
		side := "both"
		if base && !head {
			side = "base"
		}
		if head && !base {
			side = "head"
		}
		add(node.Path, side)
	}
	if len(match.nodes) == 0 && (match.grounding == pb.ImpactGrounding_IMPACT_GROUNDING_UNRESOLVED || match.grounding == pb.ImpactGrounding_IMPACT_GROUNDING_AMBIGUOUS) {
		kind, outcome, reason = "unresolved", "unresolved", "Source link does not resolve in either captured snapshot"
		if match.grounding == pb.ImpactGrounding_IMPACT_GROUNDING_AMBIGUOUS {
			kind, outcome, reason = "ambiguous", "ambiguous", "Unqualified source link matches multiple repositories"
		}
		add("", "")
	}
}

func shortRef(key string) string {
	parts := strings.Split(key, "|")
	if len(parts) < 2 {
		return key
	}
	ref := "v" + parts[1]
	if len(parts) == 3 {
		prefix := "/e"
		if parts[0] == "connector" {
			prefix = "/c"
		}
		ref += prefix + parts[2]
	}
	return ref
}

func placementRef(key string) *pb.ImpactPlacementRef {
	parts := strings.Split(key, "|")
	if len(parts) != 3 {
		return nil
	}
	view, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil {
		return nil
	}
	element, err := strconv.ParseInt(parts[2], 10, 32)
	if err != nil {
		return nil
	}
	return &pb.ImpactPlacementRef{ViewId: int32(view), ElementId: int32(element)}
}

func symbolKey(symbol *pb.ImpactSymbolChange) string {
	a := symbol
	return fmt.Sprintf("%s\x00%010d\x00%010d\x00%d\x00%s\x00%d", a.GetPath(), a.GetStartLine(), a.GetEndLine(), symbol.Kind, symbol.Name, symbol.Change)
}

// PublicScene is the only wire projection of the frozen assembly. It changes
// representation, never membership, grounding, matching or source evidence.
func PublicScene(scene *model.Assembly, debug bool) *pb.ImpactScene {
	if scene == nil {
		return nil
	}
	comparison := &pb.ImpactComparison{}
	if scene.Comparison != nil {
		comparison = proto.Clone(scene.Comparison).(*pb.ImpactComparison)
	}
	comparison.RepositoryId, comparison.Key, comparison.Version = scene.RepositoryId, scene.ComparisonKey, scene.Version
	comparison.SchemaVersion, comparison.Base, comparison.Head = scene.SchemaVersion, scene.FromGitRevision, scene.ToGitRevision
	comparison.RequestedRadius, comparison.EffectiveRadius = scene.RequestedRadius, scene.EffectiveRadius
	comparison.ManifestComplete = scene.ManifestComplete
	comparison.Warnings = append([]string(nil), scene.Warnings...)
	comparison.OmittedContext = append([]string(nil), scene.OmittedContext...)
	out := &pb.ImpactScene{Comparison: comparison, Views: map[string]*pb.SceneViewContent{},
		Symbols: map[string]*pb.ImpactSymbolChange{}, Summary: &pb.ImpactSummary{LineCountsComplete: true}}
	for _, tree := range scene.Tree {
		out.Tree = append(out.Tree, proto.Clone(tree).(*diagv1.View))
	}
	for _, nav := range scene.Navigations {
		out.Navigations = append(out.Navigations, proto.Clone(nav).(*diagv1.ElementNavigationInfo))
	}
	allSymbols := map[string]*pb.ImpactSymbolChange{}
	for _, symbol := range scene.IndexedSymbols {
		s := proto.Clone(symbol).(*pb.ImpactSymbolChange)
		allSymbols[symbolKey(s)] = s
	}
	displayed := map[string]bool{}
	for _, content := range scene.Views {
		for _, placement := range content.Placements {
			for _, symbol := range placement.GetOverlay().GetSymbols() {
				s := proto.Clone(symbol).(*pb.ImpactSymbolChange)
				key := symbolKey(s)
				allSymbols[key], displayed[key] = s, true
			}
		}
	}
	refs := map[string]string{}
	for index, key := range sortedKeys(allSymbols) {
		ref := fmt.Sprintf("s%d", index+1)
		refs[key], out.Symbols[ref] = ref, allSymbols[key]
	}
	out.Summary.IndexedSymbolChanges = uint32(len(scene.IndexedSymbols))
	out.Summary.DisplayedSymbolChanges = uint32(len(displayed))
	if len(scene.IndexedSymbols) >= len(displayed) {
		out.Summary.ExcludedSymbolChanges = uint32(len(scene.IndexedSymbols) - len(displayed))
	}
	files := map[string]*pb.ImpactFile{}
	for _, file := range scene.Files {
		files[file.Path] = proto.Clone(file).(*pb.ImpactFile)
	}
	for _, path := range sortedKeys(files) {
		file := files[path]
		for _, key := range sortedKeys(allSymbols) {
			if allSymbols[key].GetPath() == path {
				file.Symbols = append(file.Symbols, refs[key])
			}
		}
		out.Files = append(out.Files, file)
		out.Summary.Files++
		if basis := file.LineCountsBasis; out.Summary.LineCountsBasis == "" {
			out.Summary.LineCountsBasis = basis
		} else if out.Summary.LineCountsBasis != basis {
			out.Summary.LineCountsBasis = "mixed"
		}
		if file.Binary {
			out.Summary.BinaryFiles++
			continue
		}
		stat := file
		out.Summary.KnownLinesAdded += stat.GetLinesAdded()
		out.Summary.KnownLinesRemoved += stat.GetLinesRemoved()
		if stat == nil || stat.LinesAdded == nil || stat.LinesRemoved == nil {
			out.Summary.LineCountsComplete = false
		}
	}
	for _, key := range sortedKeys(scene.Views) {
		content := scene.Views[key]
		target := &pb.SceneViewContent{ConnectorTargets: map[string]*pb.ImpactPlacementRef{}}
		for _, connector := range content.Connectors {
			target.Connectors = append(target.Connectors, proto.Clone(connector).(*diagv1.Connector))
			p := scene.Provenance[ConnectorKey(int64(connector.ViewId), int64(connector.Id))]
			if len(p.GetEndpointKeys()) == 2 {
				if ref := placementRef(p.EndpointKeys[1]); ref != nil && ref.ViewId != connector.ViewId {
					target.ConnectorTargets[strconv.Itoa(int(connector.Id))] = ref
				}
			}
		}
		for _, placement := range content.Placements {
			e := placement.Element
			p := scene.Provenance[PlacementKey(int64(e.ViewId), int64(e.ElementId))]
			item := &pb.ScenePlacement{Element: proto.Clone(e).(*diagv1.PlacedElement), Origin: p.GetOrigin(), Role: p.GetRole(), Grounding: p.GetGrounding(), Distance: p.GetDistance()}
			if overlay := placement.Overlay; overlay != nil {
				item.Overlay = &pb.ImpactSceneOverlay{Change: overlay.Change, Path: overlay.Path, LinesAdded: overlay.LinesAdded,
					LinesRemoved: overlay.LinesRemoved, Contained: overlay.Contained}
				for _, symbol := range overlay.Symbols {
					item.Overlay.SymbolRefs = append(item.Overlay.SymbolRefs, refs[symbolKey(proto.Clone(symbol).(*pb.ImpactSymbolChange))])
				}
			}
			switch item.Role {
			case pb.ImpactRole_IMPACT_ROLE_DIRECT, pb.ImpactRole_IMPACT_ROLE_CONTAINED:
				out.Summary.AffectedPlacements++
			case pb.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT:
				out.Summary.DependencyNeighbours++
			case pb.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT:
				out.Summary.AuthoredContext++
			case pb.ImpactRole_IMPACT_ROLE_CONTAINER:
				out.Summary.Containers++
			}
			if item.Grounding == pb.ImpactGrounding_IMPACT_GROUNDING_UNLINKED || item.Grounding == pb.ImpactGrounding_IMPACT_GROUNDING_UNRESOLVED || item.Grounding == pb.ImpactGrounding_IMPACT_GROUNDING_AMBIGUOUS {
				out.Summary.Ungrounded++
			}
			target.Placements = append(target.Placements, item)
		}
		out.Views[key] = target
	}
	if debug {
		out.Provenance = audit(scene, allSymbols, refs, displayed)
	}
	return out
}

func audit(scene *model.Assembly, symbols map[string]*pb.ImpactSymbolChange, refs map[string]string, displayed map[string]bool) *pb.ImpactAudit {
	out := &pb.ImpactAudit{References: map[string]string{}, ExcludedSymbols: map[string]string{}}
	for _, match := range scene.Matches {
		out.Matches = append(out.Matches, proto.Clone(match).(*pb.ImpactMatch))
	}
	for _, decision := range scene.Decisions {
		if decision.Action != "suppress" && (decision.Stage != "dependencies" || !strings.Contains(decision.Reason, "Required owner")) {
			continue
		}
		item := &pb.ImpactAuditDecision{Stage: decision.Stage, Action: decision.Action, Target: shortRef(decision.Key), Reason: decision.Reason}
		seenRefs := map[string]bool{}
		for _, key := range decision.RepresentativeKeys {
			if !seenRefs[key] {
				seenRefs[key] = true
				if ref := placementRef(key); ref != nil {
					item.Representatives = append(item.Representatives, ref)
				}
			}
		}
		if scene.Provenance[decision.Key] == nil && scene.AuditReferences[decision.Key] != nil {
			ref := scene.AuditReferences[decision.Key]
			out.References[item.Target] = strings.Trim(strings.Join([]string{ref.ViewName, ref.Name}, " / "), " /")
		}
		out.Decisions = append(out.Decisions, item)
	}
	seen := map[string]bool{}
	for _, key := range sortedKeys(scene.Provenance) {
		p := scene.Provenance[key]
		if p.ObjectKind == "connector" && p.Origin != pb.ImpactOrigin_IMPACT_ORIGIN_RAW {
			out.DeclaredConnectors = append(out.DeclaredConnectors, shortRef(key))
		}
		for _, witness := range p.Witnesses {
			if len(witness.Paths) < 2 {
				continue
			}
			side := "base"
			if witness.SnapshotId == scene.HeadSnapshotId {
				side = "head"
			}
			id := side + "\x00" + strings.Join(witness.Paths, "\x00")
			if seen[id] {
				continue
			}
			seen[id] = true
			out.Witnesses = append(out.Witnesses, &pb.ImpactDependencyWitness{Side: side, Paths: append([]string(nil), witness.Paths...)})
		}
	}
	for _, dependency := range scene.Dependencies {
		out.Dependencies = append(out.Dependencies, proto.Clone(dependency).(*pb.ImpactDependency))
	}
	changedPaths := map[string]bool{}
	for _, file := range scene.Files {
		changedPaths[file.Path] = true
	}
	for _, key := range sortedKeys(symbols) {
		if displayed[key] {
			continue
		}
		reason := "Indexed change has no matching retained overlay (partial grounding or display selection)"
		if symbols[key].Path == "" {
			reason = "Indexed fact has no source anchor"
		} else if !changedPaths[symbols[key].Path] {
			reason = "Indexed snapshot delta is outside the Git changed-file ledger"
		}
		out.ExcludedSymbols[refs[key]] = reason
	}
	sort.SliceStable(out.Matches, func(i, j int) bool {
		a, b := out.Matches[i], out.Matches[j]
		return fmt.Sprintf("%s/%d/%d/%s", a.Path, a.Placement.GetViewId(), a.Placement.GetElementId(), a.Outcome) < fmt.Sprintf("%s/%d/%d/%s", b.Path, b.Placement.GetViewId(), b.Placement.GetElementId(), b.Outcome)
	})
	return out
}
