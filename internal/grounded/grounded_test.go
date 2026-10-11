package grounded

import (
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

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
