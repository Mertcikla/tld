package golang

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestGoRouteEnrichers(t *testing.T) {
	enrichertest.Run(t,
		enrichertest.Case{
			Name:     "chi route requires activation and matches route call",
			Enricher: GoChi(),
			Input: enrich.FileInput{
				RelPath:  "routes.go",
				Language: "go",
				Source:   []byte(`func routes(r chi.Router) { r.Get("/users/{id}", getUser) }`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalImport, Value: "github.com/go-chi/chi/v5"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:chi", Name: "GET /users/{id}"},
		},
		enrichertest.Case{
			Name:     "echo route",
			Enricher: GoEcho(),
			Input: enrich.FileInput{
				RelPath:  "routes.go",
				Language: "go",
				Source:   []byte(`e.POST("/orders", createOrder)`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "github.com/labstack/echo/v4"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:echo", Name: "POST /orders"},
		},
		enrichertest.Case{
			Name:     "fiber route",
			Enricher: GoFiber(),
			Input: enrich.FileInput{
				RelPath:  "routes.go",
				Language: "go",
				Source:   []byte(`app.Get("/status", status)`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "github.com/gofiber/fiber/v2"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:fiber", Name: "GET /status"},
		},
	)
}
