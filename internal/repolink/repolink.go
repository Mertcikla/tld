// Package repolink resolves the relationship between workspace elements and
// indexed codeindex repositories.
//
// Elements carry three optional reference fields: an explicit repository_id,
// and the legacy repo/file_path strings. The legacy repo field stores either
// an absolute local worktree root (materialized elements) or a remote URL or
// host shorthand (hand-linked elements). This package normalizes those forms
// so all callers share one repository identity.
//
// Remote normalization (NormalizeRemote, RemoteKey, GitRemoteURL) delegates to
// the codeindex module, which is the single implementation used by both
// repository identity resolution and the keys persisted by the store.
package repolink

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/mertcikla/codeindex/remote"
	"github.com/mertcikla/tld/v2/internal/sourcelink"
)

// NormalizeRemote delegates to codeindex/remote so every consumer derives the
// same canonical browser URL.
func NormalizeRemote(raw string) (string, bool) { return remote.NormalizeRemote(raw) }

// RemoteKey delegates to codeindex/remote so keys written by the store and
// keys looked up by identity resolution can never drift apart.
func RemoteKey(value string) string { return remote.RemoteKey(value) }

// GitRemoteURL delegates to codeindex/remote.
func GitRemoteURL(ctx context.Context, root string) string { return remote.GitRemoteURL(ctx, root) }

// Repository is an indexed codeindex repository reference.
type Repository struct {
	ID        string
	Root      string
	RemoteURL string
	Name      string
	Managed   bool
}

// IsLocalPath reports whether value looks like a filesystem path rather than a
// remote repository reference.
func IsLocalPath(value string) bool {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return false
	}
	if strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "~") {
		return true
	}
	return looksLikeWindowsPath(cleaned)
}

// ByID returns the repository with the given explicit identifier.
func ByID(id string, repos []Repository) (Repository, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Repository{}, false
	}
	for _, repo := range repos {
		if repo.ID == id {
			return repo, true
		}
	}
	return Repository{}, false
}

// ByRemote matches a legacy element repo value (worktree root, remote URL, or
// owner/repo shorthand) against indexed repositories.
func ByRemote(value string, repos []Repository) (Repository, bool) {
	needle := strings.TrimSpace(value)
	if needle == "" {
		return Repository{}, false
	}
	needlePath := filepath.Clean(needle)
	key := RemoteKey(needle)
	for _, repo := range repos {
		if repo.Root != "" && strings.EqualFold(filepath.Clean(repo.Root), needlePath) {
			return repo, true
		}
		if key != "" && repo.RemoteURL != "" && RemoteKey(repo.RemoteURL) == key {
			return repo, true
		}
	}
	return Repository{}, false
}

// Resolve links an element to an indexed repository, preferring the explicit
// repository id, then the legacy repo value, then an absolute file path that
// lives inside a repository root.
func Resolve(repositoryID, repo, filePath string, repos []Repository) (Repository, bool) {
	if resolved, ok := ByID(repositoryID, repos); ok {
		return resolved, true
	}
	if resolved, ok := ByRemote(repo, repos); ok {
		return resolved, true
	}
	clean := sourcelink.BasePath(filePath)
	if !filepath.IsAbs(strings.TrimSpace(clean)) {
		return Repository{}, false
	}
	clean = filepath.Clean(clean)
	for _, candidate := range repos {
		root := filepath.Clean(candidate.Root)
		if root == "" || root == "." {
			continue
		}
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return candidate, true
		}
	}
	return Repository{}, false
}

func looksLikeWindowsPath(value string) bool {
	return len(value) >= 2 && value[1] == ':' &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z'))
}
