package java

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestJavaORMEnrichers(t *testing.T) {
	byID := enrichersByID()
	for _, spec := range Specs() {
		enrichertest.Run(t, enrichertest.Case{
			Name:     spec.ID,
			Enricher: byID[spec.ID],
			Input:    enrich.FileInput{RelPath: "Model.java", Language: "java", Source: []byte(spec.SourceTokens[0])},
			Signals:  []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: spec.Triggers[0].Value}},
			Want:     enrichertest.Fact{Type: spec.FactType, Tag: "category:orm", Name: spec.Name, Attribute: "technology", AttrValue: spec.Name},
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
