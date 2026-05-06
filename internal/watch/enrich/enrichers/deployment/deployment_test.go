package deployment

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestDeploymentEnrichers(t *testing.T) {
	byID := enrichersByID()
	for _, spec := range Specs() {
		relPath := "deploy/pipeline.yml"
		if len(spec.PathTokens) > 0 {
			relPath = spec.PathTokens[0] + "pipeline.yml"
		}
		enrichertest.Run(t, enrichertest.Case{
			Name:     spec.ID,
			Enricher: byID[spec.ID],
			Input: enrich.FileInput{
				RelPath:  relPath,
				Language: "yaml",
				Source:   []byte(spec.SourceTokens[0]),
			},
			Want: enrichertest.Fact{Type: spec.FactType, Tag: "category:deployment", Name: spec.Name, Attribute: "technology", AttrValue: spec.Name},
		})
	}
}

func enrichersByID() map[string]enrich.Enricher {
	out := map[string]enrich.Enricher{}
	for _, enricher := range All() {
		out[enricher.Metadata().ID] = enricher
	}
	return out
}
