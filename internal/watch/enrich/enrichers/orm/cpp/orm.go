package cpp

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("cpp.raw_sql", "C++ raw SQL", "sqlite3", "sqlite3_prepare", "raw-sql"),
		spec("cpp.libpqxx", "C++ libpqxx", "libpqxx", "pqxx::", "libpqxx"),
		spec("cpp.soci", "C++ SOCI", "soci", "soci::session", "soci"),
		spec("cpp.sqlite_orm", "C++ sqlite_orm", "sqlite_orm", "make_storage", "sqlite-orm"),
		spec("cpp.odb", "C++ ODB", "odb", "odb::database", "odb"),
	}
}

func spec(id, name, dependency, token, orm string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "orm",
		Languages:    []string{"cpp"},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     "orm.query",
		Relationship: "queries",
		SourceTokens: []string{token},
		Tags:         []string{"orm:" + orm},
		Attributes:   map[string]string{"dependency": dependency, "language": "cpp", "orm": orm},
	}
}
