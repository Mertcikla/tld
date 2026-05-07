package httpclient

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestHTTPClientEnrichers(t *testing.T) {
	byID := enrichersByID()
	for _, spec := range Specs() {
		tc := enrichertest.Case{
			Name:     spec.ID,
			Enricher: byID[spec.ID],
			Input: enrich.FileInput{
				RelPath:  "src/client",
				Language: spec.Languages[0],
				Source:   []byte(spec.SourceTokens[0]),
			},
			Want: enrichertest.Fact{Type: spec.FactType, Tag: "category:http-client", Name: spec.Name, Attribute: "technology", AttrValue: spec.Name},
		}
		if len(spec.Triggers) > 0 {
			tc.Signals = []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: spec.Triggers[0].Value}}
		}
		enrichertest.Run(t, tc)
	}
}

func enrichersByID() map[string]enrich.Enricher {
	out := map[string]enrich.Enricher{}
	for _, enricher := range All() {
		out[enricher.Metadata().ID] = enricher
	}
	return out
}
