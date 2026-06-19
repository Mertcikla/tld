package grpc

import (
	"context"
	"testing"

	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichertest"
)

func TestGRPCEnrichers(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "go grpc client requires activation and matches generated client constructor",
		Enricher: GoGRPC(),
		Input: enrich.FileInput{
			RelPath:  "src/frontend/rpc.go",
			Language: "go",
			Source:   []byte(`func f() { _ = pb.NewCartServiceClient(conn) }`),
		},
		Signals: []enrich.ActivationSignal{{Kind: enrich.SignalImport, Value: "google.golang.org/grpc"}},
		Want:    enrichertest.Fact{Type: "grpc.client", Tag: "grpc:client", Name: "cart"},
	})
}

func TestGRPCDoesNotEmitBlankEndpointRefs(t *testing.T) {
	input := enrich.FileInput{
		RelPath:  "src/frontend/rpc.go",
		Language: "go",
		Source:   []byte(`func endpoint() string { return os.Getenv("CART_SERVICE_ADDR") }`),
		Signals:  []enrich.ActivationSignal{{Kind: enrich.SignalImport, Value: "google.golang.org/grpc"}},
	}
	facts, _, err := enrich.NewRegistry(GoGRPC()).EnrichFile(context.Background(), input)
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	if len(facts) != 0 {
		t.Fatalf("expected env read not to emit gRPC facts, got %+v", facts)
	}
}

func TestGRPCDoesNotEmitBuildDependencyFacts(t *testing.T) {
	input := enrich.FileInput{
		RelPath:  "build.gradle",
		Language: "gradle",
		Source:   []byte(`implementation "io.grpc:grpc-netty:1.60.0"`),
		Signals:  []enrich.ActivationSignal{{Kind: enrich.SignalImport, Value: "io.grpc"}},
	}
	facts, _, err := enrich.NewRegistry(JavaGRPC()).EnrichFile(context.Background(), input)
	if err != nil {
		t.Fatalf("enrich: %v", err)
	}
	if len(facts) != 0 {
		t.Fatalf("expected build dependency not to emit gRPC facts, got %+v", facts)
	}
}
