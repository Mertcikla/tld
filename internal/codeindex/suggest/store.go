package suggest

import cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"

// The bun-backed codeindex store satisfies the suggester's read surface.
var _ Store = (*cstore.Store)(nil)
