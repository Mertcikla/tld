package config

import (
	"testing"

	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichertest"
)

func TestConfigEnrichers(t *testing.T) {
	byID := enrichersByID()
	for _, spec := range Specs() {
		tc := enrichertest.Case{
			Name:     spec.ID,
			Enricher: byID[spec.ID],
			Input: enrich.FileInput{
				RelPath:  "src/config",
				Language: spec.Languages[0],
				Source:   []byte(spec.SourceTokens[0]),
			},
			Want: enrichertest.Fact{Type: spec.FactType, Tag: "category:config", Name: spec.Name, Attribute: "technology", AttrValue: spec.Name},
		}
		if len(spec.Triggers) > 0 {
			tc.Signals = []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: spec.Triggers[0].Value}}
		}
		enrichertest.Run(t, tc)
	}
}

func TestConcreteBindingsExtractKeys(t *testing.T) {
	enrichertest.Run(t,
		enrichertest.Case{
			Name:     "go getenv",
			Enricher: ConcreteBindings(),
			Input: enrich.FileInput{
				RelPath:  "config.go",
				Language: "go",
				Source:   []byte(`package main; func load() { _ = os.Getenv("DB_HOST") }`),
			},
			Want: enrichertest.Fact{Type: "config.env", Name: "DB_HOST", Attribute: "key", AttrValue: "DB_HOST"},
		},
		enrichertest.Case{
			Name:     "typescript local storage",
			Enricher: ConcreteBindings(),
			Input: enrich.FileInput{
				RelPath:  "session.ts",
				Language: "typescript",
				Source:   []byte(`localStorage.getItem("checkout_session")`),
			},
			Want: enrichertest.Fact{Type: "config.env", Name: "checkout_session", Attribute: "binding_kind", AttrValue: "browser_storage"},
		},
	)
}

func enrichersByID() map[string]enrich.Enricher {
	out := map[string]enrich.Enricher{}
	for _, enricher := range All() {
		out[enricher.Metadata().ID] = enricher
	}
	return out
}
