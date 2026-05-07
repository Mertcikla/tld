package grpc

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
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
