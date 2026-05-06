package observability

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestObservabilityEnrichers(t *testing.T) {
	for _, spec := range Specs() {
		source := spec.SourceTokens[0]
		enrichertest.Run(t, enrichertest.Case{
			Name:     spec.ID,
			Enricher: enrichersByID()[spec.ID],
			Input: enrich.FileInput{
				RelPath:  "src/service",
				Language: spec.Languages[0],
				Source:   []byte(source),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: spec.Triggers[0].Value}},
			Want:    enrichertest.Fact{Type: spec.FactType, Tag: "category:observability", Name: spec.Name, Attribute: "technology", AttrValue: spec.Name},
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
