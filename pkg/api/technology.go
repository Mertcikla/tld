package api

import (
	"github.com/mertcikla/tld/v2/internal/tech"
	"github.com/mertcikla/tld/v2/pkg/app"
)

// TechnologyLogoURL resolves catalog links consistently for local and hosted
// adapters. A primary icon wins; otherwise the first valid catalog link wins.
func TechnologyLogoURL(links []app.TechnologyConnector) *string {
	var fallback string
	for _, link := range links {
		if link.Type != "catalog" || link.Slug == "" {
			continue
		}
		path, ok := tech.IconURLForSlug(link.Slug)
		if !ok {
			continue
		}
		if link.IsPrimaryIcon {
			return &path
		}
		if fallback == "" {
			fallback = path
		}
	}
	if fallback == "" {
		return nil
	}
	return &fallback
}
