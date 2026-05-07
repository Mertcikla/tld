package typescript

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher {
	out := []enrich.Enricher{Prisma()}
	out = append(out, pattern.FromSpecs(Specs())...)
	return out
}

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("ts.typeorm", "TypeScript TypeORM", "typeorm", "DataSource", "typeorm"),
		spec("ts.sequelize", "TypeScript Sequelize", "sequelize", "Sequelize", "sequelize"),
		spec("ts.drizzle", "TypeScript Drizzle", "drizzle-orm", "drizzle(", "drizzle"),
	}
}

func spec(id, name, dependency, token, orm string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "orm",
		Languages:    []string{"typescript", "javascript"},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     "orm.query",
		Relationship: "queries",
		SourceTokens: []string{token},
		Tags:         []string{"orm:" + orm},
		Attributes:   map[string]string{"dependency": dependency, "language": "typescript", "orm": orm},
	}
}
