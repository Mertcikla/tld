package grounded

import (
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func strPtr(s string) *string { return &s }

func TestSummarizeAffectedContextUngrounded(t *testing.T) {
	diagram := &pb.ImpactDiagram{
		Diff: &pb.SnapshotDiff{Sources: []*pb.SourceChange{{Path: "api/orders/handler.go"}}},
	}
	explore := app.ExploreData{Views: map[string]app.ExploreViewData{
		"7": {Placements: []app.PlacedElement{
			{Name: "create-order", FilePath: strPtr("api/orders/handler.go#function:CreateOrder")},
			{Name: "charge", FilePath: strPtr("payments/stripe/client.go")},
			{Name: "db"},
		}},
	}}
	sum := Summarize(diagram, explore, "", false, nil)
	if sum.Mode != "grounded" || sum.Affected != 1 || sum.Context != 1 || sum.Ungrounded != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	if len(sum.Views) != 1 || sum.Views[0].Affected != 1 {
		t.Fatalf("views = %+v", sum.Views)
	}
}

func TestSummarizeFolderLinkMatches(t *testing.T) {
	diagram := &pb.ImpactDiagram{
		Diff: &pb.SnapshotDiff{Sources: []*pb.SourceChange{{Path: "db/migrations/0042.sql"}}},
	}
	explore := app.ExploreData{Views: map[string]app.ExploreViewData{
		"9": {Placements: []app.PlacedElement{{Name: "db", FilePath: strPtr("db/")}}},
	}}
	sum := Summarize(diagram, explore, "", false, nil)
	if sum.Affected != 1 {
		t.Fatalf("folder link should match, got %+v", sum)
	}
}

func TestRollupFanoutCollapsesHundredEdges(t *testing.T) {

	var edges []*pb.ImpactEdge
	for i := 0; i < 100; i++ {
		target := "file|db/migrations/f.sql"
		if i < 12 {
			target = "file|api/orders/validate.go"
		} else if i < 20 {
			target = "file|payments/stripe/client.go"
		}
		edges = append(edges, &pb.ImpactEdge{FromKey: "file|api/orders/handler.go", ToKey: target})
	}
	diagram := &pb.ImpactDiagram{
		Nodes: []*pb.ImpactNode{{Key: "file|api/orders/handler.go", Path: "api/orders/handler.go"}},
		Edges: edges,
	}
	fanout := RollupFanout(diagram, MaxFanoutShown)
	if len(fanout) != 1 || fanout[0].Total != 100 {
		t.Fatalf("fanout = %+v", fanout)
	}
	if len(fanout[0].Groups) > MaxFanoutShown {
		t.Fatalf("groups exceed budget: %+v", fanout[0])
	}
}

func TestSummarizeListsContextAndConnectorsInMatchedView(t *testing.T) {
	diagram := &pb.ImpactDiagram{
		Diff: &pb.SnapshotDiff{Sources: []*pb.SourceChange{{Path: "a.go"}}},
	}
	explore := app.ExploreData{Views: map[string]app.ExploreViewData{
		"7": {
			Placements: []app.PlacedElement{
				{ElementID: 1, Name: "entry", FilePath: strPtr("a.go")},
				{ElementID: 2, Name: "neighbour", FilePath: strPtr("b.go")},
				{ElementID: 3, Name: "notes"},
			},
			Connectors: []app.Connector{
				{SourceElementID: 1, TargetElementID: 2, Label: strPtr("calls")},
			},
		},
		"8": {Placements: []app.PlacedElement{
			{ElementID: 9, Name: "elsewhere", FilePath: strPtr("z.go")},
		}},
	}}
	sum := Summarize(diagram, explore, "", false, nil)
	if sum.Affected != 1 || sum.Context != 2 || sum.Ungrounded != 1 {
		t.Fatalf("counts = %+v", sum)
	}
	// Matched view lists affected + context + unlinked; the untouched view
	// stays counted only.
	if len(sum.Elements) != 3 {
		t.Fatalf("elements = %+v", sum.Elements)
	}
	found := map[string]string{}
	for _, el := range sum.Elements {
		found[el.Name] = el.Status
		if el.ElementID == 0 {
			t.Fatalf("element missing id: %+v", el)
		}
	}
	if found["entry"] != StatusAffected || found["neighbour"] != StatusContext || found["notes"] != StatusUngrounded {
		t.Fatalf("statuses = %v", found)
	}
	if len(sum.Connectors) != 1 || sum.Connectors[0].Label != "calls" {
		t.Fatalf("connectors = %+v", sum.Connectors)
	}
}

func TestSummarizeIndirectNeighbourChange(t *testing.T) {
	// validate.py changed; nothing linked changed, but build.py (linked)
	// imports it, so the owning view still matches via proximity.
	diagram := &pb.ImpactDiagram{
		Nodes: []*pb.ImpactNode{
			{Key: "file|validate.py", Path: "validate.py", Name: "validate.py"},
		},
		Diff: &pb.SnapshotDiff{Sources: []*pb.SourceChange{{Path: "validate.py"}}},
	}
	explore := app.ExploreData{Views: map[string]app.ExploreViewData{
		"7": {Placements: []app.PlacedElement{
			{ElementID: 1, Name: "build", FilePath: strPtr("build.py")},
			{ElementID: 2, Name: "unrelated", FilePath: strPtr("other.py")},
		}},
	}}
	neighbours := map[string][]string{
		"validate.py": {"build.py"},
		"build.py":    {"validate.py"},
	}
	sum := Summarize(diagram, explore, "", false, neighbours)
	if sum.Affected != 0 || sum.FallbackToRaw {
		t.Fatalf("indirect change must not count affected nor fall back: %+v", sum)
	}
	if len(sum.Views) != 1 || sum.Views[0].Indirect != 1 {
		t.Fatalf("views = %+v", sum.Views)
	}
	var build *ElementStatus
	for i := range sum.Elements {
		if sum.Elements[i].Name == "build" {
			build = &sum.Elements[i]
		}
	}
	if build == nil || build.Status != StatusContext || len(build.Near) != 1 || build.Near[0] != "validate.py" {
		t.Fatalf("build element = %+v", build)
	}
	if len(sum.Views) != 1 || sum.Views[0].Indirect != 1 || sum.FallbackToRaw {
		t.Fatalf("indirect view = %+v", sum.Views)
	}
}

func TestSummarizeUncoveredListsWorkspaceGaps(t *testing.T) {
	diagram := &pb.ImpactDiagram{
		Diff: &pb.SnapshotDiff{Sources: []*pb.SourceChange{
			{Path: "a.go"},
			{Path: "sub/b.go"},
			{Path: "orphan.go"},
		}},
	}
	explore := app.ExploreData{Views: map[string]app.ExploreViewData{
		"7": {Placements: []app.PlacedElement{
			{ElementID: 1, Name: "a", FilePath: strPtr("a.go")},
			{ElementID: 2, Name: "folder", FilePath: strPtr("sub/")},
		}},
	}}
	sum := Summarize(diagram, explore, "", false, nil)
	if len(sum.Uncovered) != 1 || sum.Uncovered[0] != "orphan.go" {
		t.Fatalf("uncovered = %+v", sum.Uncovered)
	}
}
