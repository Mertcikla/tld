// Package materialize projects a codeindex snapshot's dependency graph into the
// workspace model: file facts become nested component views and connectors,
// keyed by canonical logical keys so repeated runs upsert rather than duplicate
// and stale resources are pruned.
package materialize

import (
	"context"

	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

// IndexStore is the codeindex mapping surface the materializer needs.
type IndexStore interface {
	MappingByLogicalKey(ctx context.Context, logicalKey string) (cstore.ResourceMapping, bool, error)
	MappingsByRepository(ctx context.Context, repositoryID string) ([]cstore.ResourceMapping, error)
	SaveMappings(ctx context.Context, mappings []cstore.ResourceMapping) error
	DeleteMapping(ctx context.Context, logicalKey string) error
}

// sourceOnly strips user-owned presentation fields so re-materializing an
// existing resource updates only its code-derived linkage. Name, description,
// tags, technology, external links, and the logo stay as the user set them;
// created resources still get the full generated input.
func sourceOnly(input core.LibraryElement) core.LibraryElement {
	input.Name = ""
	input.Description = nil
	input.Tags = nil
	input.Technology = nil
	input.URL = nil
	input.LogoURL = nil
	return input
}

// connectorSourceOnly keeps the code-derived relationship on an existing
// connector while leaving the user's chosen route style untouched.
func connectorSourceOnly(input core.Connector) core.Connector {
	input.Style = ""
	return input
}
