package java

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestJavaRouteEnrichers(t *testing.T) {
	enrichertest.Run(t,
		enrichertest.Case{Name: "spring route", Enricher: Spring(), Input: input(`@GetMapping("/users")`), Signals: signal("spring-boot-starter-web"), Want: want("framework:spring", "GET /users")},
		enrichertest.Case{Name: "jax-rs route", Enricher: JAXRS(), Input: input(`@Path("/users")`), Signals: signal("jakarta.ws.rs-api"), Want: want("framework:jax-rs", "/users")},
		enrichertest.Case{Name: "micronaut route", Enricher: Micronaut(), Input: input(`@Post("/orders")`), Signals: signal("micronaut-http-server"), Want: want("framework:micronaut", "POST /orders")},
		enrichertest.Case{Name: "quarkus route", Enricher: Quarkus(), Input: input(`@Path("/health")`), Signals: signal("quarkus-resteasy-reactive"), Want: want("framework:quarkus", "/health")},
	)
}

func input(source string) enrich.FileInput {
	return enrich.FileInput{RelPath: "Controller.java", Language: "java", Source: []byte(source)}
}

func signal(value string) []enrich.ActivationSignal {
	return []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: value}}
}

func want(tag, name string) enrichertest.Fact {
	return enrichertest.Fact{Type: "http.route", Tag: tag, Name: name}
}
