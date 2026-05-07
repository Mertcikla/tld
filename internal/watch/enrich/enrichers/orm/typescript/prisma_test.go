package typescript

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestPrismaEnricher(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "prisma query requires activation and matches model operation",
		Enricher: Prisma(),
		Input: enrich.FileInput{
			RelPath:  "db.ts",
			Language: "typescript",
			Source:   []byte(`await prisma.user.findMany()`),
		},
		Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "@prisma/client"}},
		Want:    enrichertest.Fact{Type: "orm.query", Tag: "orm:prisma", Name: "user.findMany", Attribute: "operation", AttrValue: "findMany"},
	})
}

func TestTypeScriptORMCatalogEnrichers(t *testing.T) {
	byID := enrichersByID()
	for _, spec := range Specs() {
		enrichertest.Run(t, enrichertest.Case{
			Name:     spec.ID,
			Enricher: byID[spec.ID],
			Input: enrich.FileInput{
				RelPath:  "db.ts",
				Language: "typescript",
				Source:   []byte(spec.SourceTokens[0]),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: spec.Triggers[0].Value}},
			Want:    enrichertest.Fact{Type: spec.FactType, Tag: "category:orm", Name: spec.Name, Attribute: "technology", AttrValue: spec.Name},
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
