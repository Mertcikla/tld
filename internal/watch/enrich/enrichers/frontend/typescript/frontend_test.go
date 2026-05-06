package typescript

import (
	"testing"

	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichertest"
)

func TestTypeScriptFrontendEnrichers(t *testing.T) {
	enrichertest.Run(t, enrichertest.Case{
		Name:     "nextjs file route requires activation and matches app route path",
		Enricher: NextJS(),
		Input: enrich.FileInput{
			RelPath:  "src/app/users/[id]/page.tsx",
			Language: "typescript",
			Source:   []byte(`export default function Page() { return null }`),
		},
		Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "next"}},
		Want:    enrichertest.Fact{Type: "frontend.route", Tag: "framework:nextjs", Name: "/users/:id"},
	})
}
