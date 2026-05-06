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
