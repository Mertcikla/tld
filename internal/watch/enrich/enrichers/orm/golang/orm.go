package golang

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("go.gorm", "Go GORM", "github.com/go-gorm/gorm", "gorm.Open", "gorm"),
		spec("go.sqlc", "Go sqlc", "github.com/sqlc-dev/sqlc", "sqlc", "sqlc"),
		spec("go.ent", "Go ent", "entgo.io/ent", "ent.Client", "ent"),
		spec("go.database_sql", "Go database/sql", "database/sql", "sql.Open", "database-sql"),
	}
}

func spec(id, name, dependency, token, orm string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "orm",
		Languages:    []string{"go"},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     "orm.query",
		Relationship: "queries",
		SourceTokens: []string{token},
		Tags:         []string{"orm:" + orm},
		Attributes:   map[string]string{"dependency": dependency, "language": "go", "orm": orm},
	}
}
