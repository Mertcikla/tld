// Package identity maps a local checkout root to the stable logical repository
// identity shared across checkouts and machines.
//
// Repository identity is the normalized remote key (see repolink.RemoteKey)
// when a checkout has a remote, so another developer's clone of the same
// repository resolves to the same repository id. Checkouts without a remote
// fall back to the explicit id from .tld.yaml and finally to a path-derived id,
// preserving single-machine behavior.
package identity

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/repolink"
)

// Resolved is a repository's stable identity plus the local checkout it was
// resolved from. All fields except Managed are populated by Resolve.
type Resolved struct {
	ID        string
	Root      string
	RemoteURL string
	RemoteKey string
	Managed   bool
}

// Resolve maps root to a stable repository id. Resolution order:
//  1. an existing repository with the same normalized remote key (dedupes the
//     same repository across checkouts and machines),
//  2. the explicit id from .tld.yaml,
//  3. a path-derived id when the repository has no remote or is not registered.
//
// remoteURL, when empty, is read from the checkout's origin remote.
func Resolve(ctx context.Context, idx *cstore.Store, root, explicitID, remoteURL string) (Resolved, error) {
	root = filepath.Clean(root)
	if strings.TrimSpace(remoteURL) == "" {
		remoteURL = repolink.GitRemoteURL(ctx, root)
	}
	key := repolink.RemoteKey(remoteURL)
	if key != "" {
		id, ok, err := idx.RepositoryByRemoteKey(ctx, key)
		if err != nil {
			return Resolved{}, err
		}
		if ok {
			return Resolved{ID: id, Root: root, RemoteURL: remoteURL, RemoteKey: key}, nil
		}
	}
	if id := strings.TrimSpace(explicitID); id != "" {
		exists, err := idx.RepositoryExists(ctx, id)
		if err != nil {
			return Resolved{}, err
		}
		if exists {
			return Resolved{ID: id, Root: root, RemoteURL: remoteURL, RemoteKey: key}, nil
		}
	}
	return Resolved{ID: graph.RepositoryID(root), Root: root, RemoteURL: remoteURL, RemoteKey: key}, nil
}

// Apply resolves then persists the identity so a scan from another checkout
// resolves to the same repository. managed marks a tld-owned clone.
func Apply(ctx context.Context, idx *cstore.Store, root, explicitID, remoteURL string, managed bool) (Resolved, error) {
	resolved, err := Resolve(ctx, idx, root, explicitID, remoteURL)
	if err != nil {
		return Resolved{}, err
	}
	resolved.Managed = managed
	if err := idx.EnsureRepositoryIdentity(ctx, resolved.ID, resolved.Root, resolved.RemoteURL, resolved.RemoteKey, resolved.Managed); err != nil {
		// Another checkout registered the same remote first (the unique
		// remote_key index rejected the insert). Adopt the winning repository
		// instead of failing the scan.
		if resolved.RemoteKey != "" {
			if id, ok, lookupErr := idx.RepositoryByRemoteKey(ctx, resolved.RemoteKey); lookupErr == nil && ok && id != "" {
				resolved.ID = id
				return resolved, nil
			}
		}
		return Resolved{}, err
	}
	return resolved, nil
}
