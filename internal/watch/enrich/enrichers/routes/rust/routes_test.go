package rust

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestRustRouteEnrichers(t *testing.T) {
	enrichertest.Run(t,
		enrichertest.Case{Name: "axum route", Enricher: Axum(), Input: input(`route("/users", get(handler))`), Signals: signal("axum"), Want: want("framework:axum", "GET /users")},
		enrichertest.Case{Name: "actix route", Enricher: ActixWeb(), Input: input(`#[post("/orders")]`), Signals: signal("actix-web"), Want: want("framework:actix-web", "POST /orders")},
		enrichertest.Case{Name: "rocket route", Enricher: Rocket(), Input: input(`#[get("/health")]`), Signals: signal("rocket"), Want: want("framework:rocket", "GET /health")},
		enrichertest.Case{Name: "warp route", Enricher: Warp(), Input: input(`warp::path!("metrics")`), Signals: signal("warp"), Want: want("framework:warp", "metrics")},
	)
}

func input(source string) enrich.FileInput {
	return enrich.FileInput{RelPath: "src/routes.rs", Language: "rust", Source: []byte(source)}
}

func signal(value string) []enrich.ActivationSignal {
	return []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: value}}
}

func want(tag, name string) enrichertest.Fact {
	return enrichertest.Fact{Type: "http.route", Tag: tag, Name: name}
}
