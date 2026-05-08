package iac

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestIaCEnrichers(t *testing.T) {
	byID := enrichersByID()
	for _, spec := range Specs() {
		relPath := "deploy/service.yaml"
		language := "yaml"
		if len(spec.Languages) > 0 {
			language = spec.Languages[0]
		}
		if len(spec.PathTokens) > 0 {
			relPath = spec.PathTokens[0]
		}
		var source []byte
		if len(spec.SourceTokens) > 0 {
			source = []byte(spec.SourceTokens[0])
		} else {
			source = []byte("kind: Service\n")
		}
		enrichertest.Run(t, enrichertest.Case{
			Name:     spec.ID,
			Enricher: byID[spec.ID],
			Input: enrich.FileInput{
				RelPath:  relPath,
				Language: language,
				Source:   source,
			},
			Want: enrichertest.Fact{Type: spec.FactType, Tag: "category:iac", Name: spec.Name, Attribute: "technology", AttrValue: spec.Name},
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
