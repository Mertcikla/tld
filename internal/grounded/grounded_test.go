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
	sum := Summarize(diagram, explore, "", false)
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
	sum := Summarize(diagram, explore, "", false)
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
