package cpp

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestCPPRouteEnrichers(t *testing.T) {
	enrichertest.Run(t,
		enrichertest.Case{Name: "drogon route", Enricher: Drogon(), Input: input(`METHOD_GET ADD_METHOD_TO(UserController::get, "/users")`), Signals: signal("drogon"), Want: want("framework:drogon", "GET /users")},
		enrichertest.Case{Name: "oatpp route", Enricher: Oatpp(), Input: input(`ENDPOINT("POST", "/orders", createOrder)`), Signals: signal("oatpp"), Want: want("framework:oatpp", "POST /orders")},
		enrichertest.Case{Name: "pistache route", Enricher: Pistache(), Input: input(`Routes::Get(router, "/health", handler)`), Signals: signal("pistache"), Want: want("framework:pistache", "GET /health")},
		enrichertest.Case{Name: "crow route", Enricher: Crow(), Input: input(`CROW_ROUTE(app, "/metrics")`), Signals: signal("crow"), Want: want("framework:crow", "/metrics")},
		enrichertest.Case{Name: "cpprestsdk route", Enricher: CppRestSDK(), Input: input(`listener.support(methods::GET, handler)`), Signals: signal("cpprestsdk"), Want: want("framework:cpprestsdk", "GET handler")},
	)
}

func input(source string) enrich.FileInput {
	return enrich.FileInput{RelPath: "src/routes.cpp", Language: "cpp", Source: []byte(source)}
}

func signal(value string) []enrich.ActivationSignal {
	return []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: value}}
}

func want(tag, name string) enrichertest.Fact {
	return enrichertest.Fact{Type: "http.route", Tag: tag, Name: name}
}
