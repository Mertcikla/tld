package osintegration

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("os.uri_schemes", "Custom URI schemes", []string{"info.plist", "androidmanifest.xml", "electron"}, []string{"CFBundleURLSchemes"}, "os.uri_scheme", "handles_deep_link"),
		spec("os.android_intents", "Android Intents", []string{"androidmanifest.xml"}, []string{"<intent-filter"}, "os.intent", "broadcasts_intent"),
	}
}

func spec(id, name string, pathTokens, sourceTokens []string, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "os-integration",
		Languages:    []string{"xml", "json"},
		Mode:         enrich.ActivationAlways,
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: sourceTokens,
		PathTokens:   pathTokens,
		Tags:         []string{"os-integration:" + id},
		Attributes:   map[string]string{"platform": "desktop-mobile"},
	}
}
