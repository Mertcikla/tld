// Package repolink resolves the relationship between workspace elements and
// indexed codeindex repositories.
//
// Elements carry three optional reference fields: an explicit repository_id,
// and the legacy repo/file_path strings. The legacy repo field stores either
// an absolute local worktree root (materialized elements) or a remote URL or
// host shorthand (hand-linked elements). This package normalizes those forms
// so all callers share one repository identity.
package repolink

import (
	"context"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repository is an indexed codeindex repository reference.
type Repository struct {
	ID        string
	Root      string
	RemoteURL string
	Name      string
	Managed   bool
}

// NormalizeRemote converts common Git remote forms (https, http, ssh, git and
// scp-like git@host:path) into a canonical browser URL such as
// https://github.com/owner/repo. Credentials are stripped. ok is false when
// raw does not identify a remote repository.
func NormalizeRemote(raw string) (string, bool) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" || strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "~") {
		return "", false
	}
	if looksLikeWindowsPath(cleaned) {
		return "", false
	}
	cleaned = strings.TrimSuffix(cleaned, "/")
	cleaned = strings.TrimSuffix(cleaned, ".git")

	var host, path string
	switch {
	case strings.Contains(cleaned, "://"):
		parsed, err := url.Parse(cleaned)
		if err != nil || parsed.Hostname() == "" {
			return "", false
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https", "ssh", "git":
		default:
			return "", false
		}
		host = parsed.Hostname()
		path = parsed.Path
	case strings.Contains(cleaned, "@") && strings.Contains(cleaned, ":"):
		at := strings.Index(cleaned, "@")
		colon := strings.Index(cleaned[at:], ":") + at
		host = cleaned[at+1 : colon]
		path = cleaned[colon+1:]
	case strings.Contains(cleaned, ":"):
		colon := strings.Index(cleaned, ":")
		host = cleaned[:colon]
		path = cleaned[colon+1:]
	default:
		slash := strings.Index(cleaned, "/")
		if slash < 0 {
			return "", false
		}
		host = cleaned[:slash]
		path = cleaned[slash+1:]
	}
	host = strings.ToLower(strings.TrimSpace(host))
	path = strings.Trim(strings.TrimSpace(path), "/")
	if host == "" || path == "" || (!strings.Contains(host, ".") && host != "localhost") {
		return "", false
	}
	if strings.ContainsAny(host, " \t") || strings.ContainsAny(path, " \t") {
		return "", false
	}
	return "https://" + host + "/" + path, true
}

// RemoteKey returns a normalized host/path identity for a remote URL or a
// legacy element repo value, including GitHub owner/repo shorthand. It returns
// "" when value cannot be interpreted as a repository identity. Matching is
// case-insensitive.
func RemoteKey(value string) string {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" || IsLocalPath(cleaned) {
		return ""
	}
	if normalized, ok := NormalizeRemote(cleaned); ok {
		return strings.ToLower(strings.TrimPrefix(normalized, "https://"))
	}
	parts := strings.Split(strings.Trim(cleaned, "/"), "/")
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" &&
		!strings.Contains(parts[0], ".") && !strings.Contains(parts[0], ":") {
		return "github.com/" + strings.ToLower(parts[0]) + "/" + strings.ToLower(parts[1])
	}
	return ""
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
	clean := filePath
	if before, _, found := strings.Cut(clean, "#"); found {
		clean = before
	}
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

// GitRemoteURL reports the sanitized browser URL of the repository's origin
// remote, or "" when there is no usable origin.
func GitRemoteURL(ctx context.Context, root string) string {
	if strings.TrimSpace(root) == "" {
		return ""
	}
	out, err := exec.CommandContext(ctx, "git", "-C", root, "remote", "get-url", "origin").Output()
	if err != nil {
		return ""
	}
	normalized, ok := NormalizeRemote(strings.TrimSpace(string(out)))
	if !ok {
		return ""
	}
	return normalized
}

func looksLikeWindowsPath(value string) bool {
	return len(value) >= 2 && value[1] == ':' &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z'))
}
