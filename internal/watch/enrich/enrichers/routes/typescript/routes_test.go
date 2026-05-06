package typescript

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestTypeScriptRouteEnrichers(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "express route requires activation and matches router call",
		Enricher: Express(),
		Input: enrich.FileInput{
			RelPath:  "server.ts",
			Language: "typescript",
			Source:   []byte(`router.post("/api/users", createUser)`),
		},
		Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "express"}},
		Want:    enrichertest.Fact{Type: "http.route", Tag: "framework:express", Name: "POST /api/users"},
	})
}
