package impact

import (
	"fmt"
	"strconv"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	"google.golang.org/protobuf/proto"
)

// Capture symbol relationships while assembling the comparison, so historical
// and exported scenes keep the same endpoints even after the index changes.
func (a *sceneAssembly) projectSymbolRelationships() error {
	seen := map[string]bool{}
	baseEdges := map[string]*pb.EdgeFact{}
	if a.scene.BaseSnapshotId != "" {
		after := ""
		for {
			page, err := a.helper.store.EdgeFacts(a.helper.ctx, a.scene.BaseSnapshotId, pb.EdgeKind_EDGE_KIND_UNSPECIFIED, "", after, 1000)
			if err != nil {
				return fmt.Errorf("load base symbol relationships: %w", err)
			}
			for _, edge := range page {
				baseEdges[edge.LogicalKey] = edge
			}
			if len(page) < 1000 {
				break
			}
			after = page[len(page)-1].Id
		}
	}
	for _, snapshot := range []string{a.scene.HeadSnapshotId, a.scene.BaseSnapshotId} {
		if snapshot == "" {
			continue
		}
		facts, err := a.helper.store.AllFacts(a.helper.ctx, snapshot, pb.FactKind_FACT_KIND_UNSPECIFIED)
		if err != nil {
			return fmt.Errorf("load symbol endpoints: %w", err)
		}
		byID := map[string]*pb.CodeFact{}
		for _, fact := range facts {
			byID[fact.Id] = fact
		}
		after := ""
		for {
			edges, err := a.helper.store.EdgeFacts(a.helper.ctx, snapshot, pb.EdgeKind_EDGE_KIND_UNSPECIFIED, "", after, 1000)
			if err != nil {
				return fmt.Errorf("load symbol relationships: %w", err)
			}
			for _, edge := range edges {
				if edge.Kind != pb.EdgeKind_EDGE_KIND_CALLS && edge.Kind != pb.EdgeKind_EDGE_KIND_REFERENCES {
					continue
				}
				from, to := byID[edge.FromFactId], byID[edge.ToFactId]
				if from == nil || to == nil || from.Anchor == nil || to.Anchor == nil || from.Anchor.Path == to.Anchor.Path {
					continue
				}
				identity := edge.LogicalKey
				if identity == "" {
					identity = fmt.Sprintf("%d:%s:%s", edge.Kind, from.Anchor.Path+":"+from.Name, to.Anchor.Path+":"+to.Name)
				}
				if seen[identity] {
					continue
				}
				seen[identity] = true
				change := pb.ChangeKind_CHANGE_KIND_UNSPECIFIED
				if snapshot == a.scene.BaseSnapshotId {
					change = pb.ChangeKind_CHANGE_KIND_REMOVED
				}
				if snapshot == a.scene.HeadSnapshotId && edge.LogicalKey != "" {
					if before := baseEdges[edge.LogicalKey]; before == nil {
						change = pb.ChangeKind_CHANGE_KIND_ADDED
					} else if before.Weight != edge.Weight {
						change = pb.ChangeKind_CHANGE_KIND_MODIFIED
					}
				}
				for _, fromKey := range a.full[from.Anchor.Path] {
					for _, toKey := range a.full[to.Anchor.Path] {
						if fromKey == toKey && from.Id == to.Id {
							continue
						}
						source, target := a.scene.Provenance[fromKey], a.scene.Provenance[toKey]
						content := a.scene.Views[strconv.FormatInt(source.ViewId, 10)]
						authored := false
						for _, existing := range content.Connectors {
							if existing.Id > 0 && connectorRepresentsDependency(existing, int32(source.ResourceId), int32(target.ResourceId)) {
								authored = true
								break
							}
						}
						if authored {
							continue
						}
						id := a.transientID("symbol:" + identity + ":" + fromKey + ":" + toKey)
						for _, endpoint := range []struct {
							key  string
							fact *pb.CodeFact
						}{{fromKey, from}, {toKey, to}} {
							p := a.scene.Provenance[endpoint.key]
							for _, placement := range a.scene.Views[strconv.FormatInt(p.ViewId, 10)].Placements {
								if int64(placement.Element.ElementId) != p.ResourceId || placement.Overlay == nil {
									continue
								}
								found := false
								for _, symbol := range placement.Overlay.Symbols {
									if symbol.Name == endpoint.fact.Name && symbol.StartLine == endpoint.fact.Anchor.StartLine {
										found = true
										break
									}
								}
								if !found {
									placement.Overlay.Symbols = append(placement.Overlay.Symbols, symbolChange(pb.ChangeKind_CHANGE_KIND_UNSPECIFIED, endpoint.fact))
								}
							}
						}
						label := "references"
						if edge.Kind == pb.EdgeKind_EDGE_KIND_CALLS {
							label = "calls"
						}
						connector := &diagv1.Connector{Id: id, ViewId: int32(source.ViewId), SourceElementId: int32(source.ResourceId), TargetElementId: int32(target.ResourceId),
							Label: proto.String(label), Direction: "forward", Style: "bezier", SourceHandle: proto.String(fmt.Sprintf("right-line-%d", from.Anchor.StartLine+1)), TargetHandle: proto.String(fmt.Sprintf("left-line-%d", to.Anchor.StartLine+1)), Tags: appendTagUnique(nil, connectorChangeTag(change))}
						// Replace the synthesized file edge once precise endpoints are available.
						kept := content.Connectors[:0]
						for _, existing := range content.Connectors {
							if existing.Id < 0 && !isSymbolHandle(existing.GetSourceHandle()) && connectorRepresentsDependency(existing, int32(source.ResourceId), int32(target.ResourceId)) {
								delete(a.scene.Provenance, ConnectorKey(source.ViewId, int64(existing.Id)))
								continue
							}
							kept = append(kept, existing)
						}
						content.Connectors = append(kept, connector)
						key := ConnectorKey(source.ViewId, int64(id))
						a.scene.Provenance[key] = &model.Metadata{Key: key, ObjectKind: "connector", ViewId: source.ViewId, ResourceId: int64(id), Name: label, Origin: pb.ImpactOrigin_IMPACT_ORIGIN_RAW, Grounding: pb.ImpactGrounding_IMPACT_GROUNDING_RESOLVED, EndpointKeys: []string{fromKey, toKey}, Paths: []string{from.Anchor.Path, to.Anchor.Path}}
					}
				}
			}
			if len(edges) < 1000 {
				break
			}
			after = edges[len(edges)-1].Id
		}
	}
	return nil
}
