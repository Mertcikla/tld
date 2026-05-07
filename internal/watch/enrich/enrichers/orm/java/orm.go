package java

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("java.hibernate", "Java Hibernate", "org.hibernate", "org.hibernate", "hibernate"),
		spec("java.jpa", "Java JPA", "jakarta.persistence", "@Entity", "jpa"),
		spec("java.spring_data_jpa", "Spring Data JPA", "spring-data-jpa", "JpaRepository", "spring-data-jpa"),
		spec("java.mybatis", "Java MyBatis", "mybatis", "@Mapper", "mybatis"),
		spec("java.jooq", "Java jOOQ", "jooq", "DSLContext", "jooq"),
	}
}

func spec(id, name, dependency, token, orm string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "orm",
		Languages:    []string{"java"},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     "orm.query",
		Relationship: "queries",
		SourceTokens: []string{token},
		Tags:         []string{"orm:" + orm},
		Attributes:   map[string]string{"dependency": dependency, "language": "java", "orm": orm},
	}
}
