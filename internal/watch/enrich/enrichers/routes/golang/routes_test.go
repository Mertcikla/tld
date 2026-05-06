package golang

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestGoRouteEnrichers(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "chi route requires activation and matches route call",
		Enricher: GoChi(),
		Input: enrich.FileInput{
			RelPath:  "routes.go",
			Language: "go",
			Source:   []byte(`func routes(r chi.Router) { r.Get("/users/{id}", getUser) }`),
		},
		Signals: []enrich.ActivationSignal{{Kind: enrich.SignalImport, Value: "github.com/go-chi/chi/v5"}},
		Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:chi", Name: "GET /users/{id}"},
	})
}
