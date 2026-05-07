package python

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("python.sqlalchemy", "Python SQLAlchemy", "sqlalchemy", "sqlalchemy", "sqlalchemy"),
		spec("python.django_orm", "Django ORM", "django", "django.db.models", "django"),
		spec("python.peewee", "Python Peewee", "peewee", "peewee", "peewee"),
		spec("python.tortoise", "Tortoise ORM", "tortoise-orm", "tortoise", "tortoise"),
	}
}

func spec(id, name, dependency, token, orm string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "orm",
		Languages:    []string{"python"},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     "orm.query",
		Relationship: "queries",
		SourceTokens: []string{token},
		Tags:         []string{"orm:" + orm},
		Attributes:   map[string]string{"dependency": dependency, "language": "python", "orm": orm},
	}
}
