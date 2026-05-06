package runtimeenrich

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
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
