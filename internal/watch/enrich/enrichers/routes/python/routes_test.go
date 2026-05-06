package python

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestPythonRouteEnrichers(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "flask route requires activation and matches route decorator",
		Enricher: PythonFlask(),
		Input: enrich.FileInput{
			RelPath:  "app.py",
			Language: "python",
			Source:   []byte(`@app.route("/users/<id>")`),
		},
		Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "flask"}},
		Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:flask", Name: "/users/<id>"},
	})
}
