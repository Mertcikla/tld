package api

import (
	"github.com/google/uuid"
	"testing"

	"github.com/mertcikla/tld/v2/pkg/app"
)

func TestAdaptersDeriveTechnologyIcons(t *testing.T) {
	t.Parallel()
	links := []app.TechnologyConnector{{Type: "catalog", Slug: "go"}, {Type: "catalog", Slug: "rust", IsPrimaryIcon: true}}
	element := elementToProto(app.LibraryElement{TechnologyConnectors: links}, uuid.Nil)
	placement := placedElementToProto(app.PlacedElement{TechnologyConnectors: links})
	if element.GetLogoUrl() != "/icons/rust.svg" || placement.GetLogoUrl() != element.GetLogoUrl() {
		t.Fatalf("derived icons differ: element=%q placement=%q", element.GetLogoUrl(), placement.GetLogoUrl())
	}
	for _, custom := range []string{"https://example.com/custom.svg", ""} {
		element = elementToProto(app.LibraryElement{LogoURL: &custom, TechnologyConnectors: links}, uuid.Nil)
		placement = placedElementToProto(app.PlacedElement{LogoURL: &custom, TechnologyConnectors: links})
		if element.GetLogoUrl() != custom || placement.GetLogoUrl() != custom {
			t.Fatalf("explicit icon %q was replaced: element=%q placement=%q", custom, element.GetLogoUrl(), placement.GetLogoUrl())
		}
	}
}
