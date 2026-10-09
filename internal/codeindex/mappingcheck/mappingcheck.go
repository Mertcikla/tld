// Package mappingcheck resolves, live against the local database, whether a
// workspace element was materialized from the codeindex by consulting the
// codeindex_elements mapping table.
package mappingcheck

import (
	"context"
	"os"

	assets "github.com/mertcikla/tld/v2"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/localserver"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// Classifier returns a predicate that reports whether a workspace element is a
// codeindex-materialized resource. The check is a live read of the local
// database's codeindex_elements table joined to the elements it points at, so
// it reflects actual mappings rather than a repository_id field.
//
// It returns nil when no local database is available or it cannot be read, in
// which case no elements are treated as codeindex-owned.
func Classifier(ctx context.Context, dataDir string) func(*workspace.Element) bool {
	idx, closeStore, ok := OpenStore(ctx, dataDir)
	if !ok {
		return nil
	}
	defer closeStore()
	index, err := idx.MappedElementIndex(ctx)
	if err != nil || (len(index.Sources) == 0 && len(index.Names) == 0) {
		return nil
	}
	return func(element *workspace.Element) bool {
		if element == nil {
			return false
		}
		if _, ok := index.Sources[cstore.ElementSourceKey(element.RepositoryID, element.FilePath)]; ok {
			return true
		}
		_, ok := index.Names[cstore.ElementNameKey(element.Kind, element.Name)]
		return ok
	}
}

// OpenStore opens the local codeindex store for dataDir. It returns ok=false
// when no local database exists or it cannot be opened, so callers can treat
// codeindex features as unavailable rather than failing.
func OpenStore(ctx context.Context, dataDir string) (*cstore.Store, func(), bool) {
	if dataDir == "" || ctx == nil {
		return nil, func() {}, false
	}
	if _, err := os.Stat(localserver.DatabasePath(dataDir)); err != nil {
		return nil, func() {}, false
	}
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		return nil, func() {}, false
	}
	sq, err := localstore.OpenLocal(ctx, cfg, dataDir, assets.FS)
	if err != nil {
		return nil, func() {}, false
	}
	idx := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())
	return idx, func() { _ = sq.Close() }, true
}
