package runtimeenrich

import (
	"context"
	"testing"

	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichertest"
)

func TestRuntimeManifests(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "kubernetes manifest matches workload name",
		Enricher: RuntimeManifests(),
		Input: enrich.FileInput{
			RelPath:  "k8s/frontend.yaml",
			Language: "yaml",
			Source: []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
`),
		},
		Want: enrichertest.Fact{Type: "runtime.component", Tag: "runtime:kubernetes", Name: "frontend"},
	})
}

func TestRuntimeOpenAPIJSONRequiresInfoTitle(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "openapi json uses declared title",
		Enricher: RuntimeManifests(),
		Input: enrich.FileInput{
			RelPath:  "openapi.json",
			Language: "json",
			Source:   []byte(`{"openapi":"3.1.0","info":{"title":"Checkout API","version":"1.0.0"}}`),
		},
		Want: enrichertest.Fact{Type: "runtime.component", Tag: "protocol:http", Name: "Checkout API"},
	})

	input := enrich.FileInput{
		RelPath:  "openapi.json",
		Language: "json",
		Source:   []byte(`{"openapi":"3.1.0","paths":{}}`),
	}
	emitter := &factCollector{}
	if err := RuntimeManifests().EnrichFile(context.Background(), input, emitter); err != nil {
		t.Fatalf("enrich: %v", err)
	}
	if len(emitter.facts) != 0 {
		t.Fatalf("expected title-less OpenAPI JSON not to emit facts, got %+v", emitter.facts)
	}
}

type factCollector struct {
	facts []enrich.Fact
}

func (c *factCollector) EmitFact(f enrich.Fact) error {
	c.facts = append(c.facts, f)
	return nil
}

func (c *factCollector) Warn(enrich.Warning) {}
