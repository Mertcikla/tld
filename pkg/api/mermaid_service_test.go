package api

import (
	"context"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
)

func TestMermaidServiceParseMermaid(t *testing.T) {
	t.Parallel()

	service := &MermaidService{}
	resp, err := service.ParseMermaid(context.Background(), connect.NewRequest(&diagv1.ParseMermaidRequest{
		Source: "flowchart LR\n  A[API] -->|reads| B[DB]",
	}))
	if err != nil {
		t.Fatalf("ParseMermaid() error = %v", err)
	}
	if len(resp.Msg.GetElements()) != 2 {
		t.Fatalf("ParseMermaid() elements = %d, want 2", len(resp.Msg.GetElements()))
	}
	if len(resp.Msg.GetConnectors()) != 1 {
		t.Fatalf("ParseMermaid() connectors = %d, want 1", len(resp.Msg.GetConnectors()))
	}
	if got := resp.Msg.GetDirection(); got != diagv1.MermaidDirection_MERMAID_DIRECTION_LR {
		t.Fatalf("ParseMermaid() direction = %v", got)
	}
}

func TestMermaidServiceParseMermaidInvalidArgument(t *testing.T) {
	t.Parallel()

	service := &MermaidService{}
	_, err := service.ParseMermaid(context.Background(), connect.NewRequest(&diagv1.ParseMermaidRequest{}))
	if err == nil {
		t.Fatal("ParseMermaid(empty) error = nil")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("ParseMermaid(empty) code = %v, want %v", connect.CodeOf(err), connect.CodeInvalidArgument)
	}
}
