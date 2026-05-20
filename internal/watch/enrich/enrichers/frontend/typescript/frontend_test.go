package typescript

import (
	"testing"

	"github.com/mertcikla/tld/v2/internal/watch/enrich"
	"github.com/mertcikla/tld/v2/internal/watch/enrich/enrichertest"
)

func TestTypeScriptFrontendEnrichers(t *testing.T) {
	enrichertest.Run(t,
		enrichertest.Case{
			Name:     "nextjs file route requires activation and matches app route path",
			Enricher: NextJS(),
			Input: enrich.FileInput{
				RelPath:  "src/app/users/[id]/page.tsx",
				Language: "typescript",
				Source:   []byte(`export default function Page() { return null }`),
			},
			Signals: []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: "next"}},
			Want:    enrichertest.Fact{Type: "frontend.route", Tag: "framework:nextjs", Name: "/users/:id"},
		},
		enrichertest.Case{
			Name:     "onclick trigger",
			Enricher: BrowserTriggers(),
			Input: enrich.FileInput{
				RelPath:  "CheckoutButton.tsx",
				Language: "typescript",
				Source:   []byte(`export function CheckoutButton() { return <button onClick={pay}>Pay</button> }`),
			},
			Want: enrichertest.Fact{Type: "frontend.trigger", Name: "UI: button_click", Attribute: "trigger", AttrValue: "UI: button_click"},
		},
	)
}
