package rust

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("rust.sqlx", "Rust sqlx", "sqlx", "sqlx::query", "sqlx"),
		spec("rust.diesel", "Rust Diesel", "diesel", "diesel::", "diesel"),
		spec("rust.seaorm", "Rust SeaORM", "sea-orm", "EntityTrait", "seaorm"),
		spec("rust.tokio_postgres", "Rust tokio-postgres", "tokio-postgres", "tokio_postgres", "tokio-postgres"),
	}
}

func spec(id, name, dependency, token, orm string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "orm",
		Languages:    []string{"rust"},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     "orm.query",
		Relationship: "queries",
		SourceTokens: []string{token},
		Tags:         []string{"orm:" + orm},
		Attributes:   map[string]string{"dependency": dependency, "language": "rust", "orm": orm},
	}
}
