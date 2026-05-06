package python

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestPythonTrafficEnrichers(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "locust client call requires activation and matches request",
		Enricher: PythonLocust(),
		Input: enrich.FileInput{
			RelPath:  "load_test.py",
			Language: "python",
			Source:   []byte(`self.client.get("/checkout")`),
		},
		Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "locust"}},
		Want:    enrichertest.Fact{Type: "http.client", Tag: "framework:locust", Name: "GET /checkout"},
	})
}
