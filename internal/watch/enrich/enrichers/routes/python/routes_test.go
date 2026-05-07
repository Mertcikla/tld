package python

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestPythonRouteEnrichers(t *testing.T) {
	enrichertest.Run(t,
		enrichertest.Case{
			Name:     "flask route requires activation and matches route decorator",
			Enricher: PythonFlask(),
			Input: enrich.FileInput{
				RelPath:  "app.py",
				Language: "python",
				Source:   []byte(`@app.route("/users/<id>")`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "flask"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:flask", Name: "/users/<id>"},
		},
		enrichertest.Case{
			Name:     "fastapi route",
			Enricher: PythonFastAPI(),
			Input: enrich.FileInput{
				RelPath:  "app.py",
				Language: "python",
				Source:   []byte(`@app.post("/orders")`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "fastapi"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:fastapi", Name: "POST /orders"},
		},
		enrichertest.Case{
			Name:     "django route",
			Enricher: PythonDjango(),
			Input: enrich.FileInput{
				RelPath:  "urls.py",
				Language: "python",
				Source:   []byte(`path("users/<int:id>/", view)`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "django"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:django", Name: "users/<int:id>/"},
		},
		enrichertest.Case{
			Name:     "starlette route",
			Enricher: PythonStarlette(),
			Input: enrich.FileInput{
				RelPath:  "routes.py",
				Language: "python",
				Source:   []byte(`Route("/health", health)`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "starlette"}},
			Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:starlette", Name: "/health"},
		},
	)
}
