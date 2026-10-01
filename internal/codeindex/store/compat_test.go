package store

import "github.com/mertcikla/tld/v2/internal/codeindex/embed"

// The bun store satisfies the embedding pipeline's persistence surface.
var _ embed.Store = (*Store)(nil)
