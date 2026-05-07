package typescript

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestTypeScriptRouteEnrichers(t *testing.T) {
	enrichertest.Run(t,
		enrichertest.Case{
			Name:     "express route requires activation and matches router call",
			Enricher: Express(),
			Input: enrich.FileInput{
				RelPath:  "server.ts",
				Language: "typescript",
				Source:   []byte(`router.post("/api/users", createUser)`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "express"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:express", Name: "POST /api/users"},
		},
		enrichertest.Case{
			Name:     "fastify route",
			Enricher: Fastify(),
			Input: enrich.FileInput{
				RelPath:  "server.ts",
				Language: "typescript",
				Source:   []byte(`fastify.get("/health", handler)`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "fastify"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:fastify", Name: "GET /health"},
		},
		enrichertest.Case{
			Name:     "nestjs route",
			Enricher: NestJS(),
			Input: enrich.FileInput{
				RelPath:  "users.controller.ts",
				Language: "typescript",
				Source:   []byte(`@Get("users/:id")`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "@nestjs/common"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:nestjs", Name: "GET users/:id"},
		},
		enrichertest.Case{
			Name:     "hono route",
			Enricher: Hono(),
			Input: enrich.FileInput{
				RelPath:  "server.ts",
				Language: "typescript",
				Source:   []byte(`app.post("/events", handler)`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "hono"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:hono", Name: "POST /events"},
		},
	)
}
