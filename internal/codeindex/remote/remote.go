// Package remote parses remote repository references and manages tld-owned
// clones. It intentionally stays provider-thin: GitHub gets shorthand support,
// any other host works through its Git URL.
package remote

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mertcikla/tld/v2/internal/repolink"
)

type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderGit    Provider = "git"
)

// Spec describes a parsed remote repository reference.
type Spec struct {
	Provider Provider
	CloneURL string
	WebURL   string
}

var ErrInvalidRemote = errors.New("invalid repository URL")

// Parse validates a remote reference and derives its clone and browser URLs.
// It accepts GitHub owner/repo shorthand, GitHub/GitLab/Git web URLs, SSH URLs,
// and scp-like git@host:path remotes. Credentials embedded in the URL are
// rejected.
func Parse(raw string) (Spec, error) {
	cleaned := strings.TrimSpace(raw)
	if cleaned == "" {
		return Spec{}, fmt.Errorf("%w: empty value", ErrInvalidRemote)
	}
	if strings.HasPrefix(cleaned, "/") || strings.HasPrefix(cleaned, "~") {
		return Spec{}, fmt.Errorf("%w: %q is a local path, not a remote", ErrInvalidRemote, raw)
	}
	if looksLikeWindowsPath(cleaned) {
		return Spec{}, fmt.Errorf("%w: %q is a local path, not a remote", ErrInvalidRemote, raw)
	}

	if strings.Contains(cleaned, "://") {
		parsed, err := url.Parse(cleaned)
		if err != nil {
			return Spec{}, fmt.Errorf("%w: %v", ErrInvalidRemote, err)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https":
			if parsed.User != nil {
				return Spec{}, fmt.Errorf("%w: remove credentials from the URL", ErrInvalidRemote)
			}
		case "ssh", "git":
			if parsed.User != nil && parsed.User.String() != "git" {
				return Spec{}, fmt.Errorf("%w: only the git ssh user is supported", ErrInvalidRemote)
			}
		default:
			return Spec{}, fmt.Errorf("%w: unsupported scheme %q", ErrInvalidRemote, parsed.Scheme)
		}
	}

	webURL, ok := repolink.NormalizeRemote(cleaned)
	if !ok {
		// owner/repo shorthand.
		parts := strings.Split(strings.Trim(cleaned, "/"), "/")
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" &&
			!strings.Contains(parts[0], ".") && !strings.Contains(parts[0], ":") {
			return githubSpec("github.com/" + parts[0] + "/" + parts[1])
		}
		return Spec{}, fmt.Errorf("%w: %q is not a remote repository URL", ErrInvalidRemote, raw)
	}

	parsedWeb, err := url.Parse(webURL)
	if err != nil || parsedWeb.Hostname() == "" {
		return Spec{}, fmt.Errorf("%w: %q is not a remote repository URL", ErrInvalidRemote, raw)
	}
	if strings.EqualFold(parsedWeb.Hostname(), "github.com") {
		return githubSpec(strings.TrimPrefix(webURL, "https://"))
	}

	spec := Spec{Provider: ProviderGit, WebURL: webURL, CloneURL: webURL + ".git"}
	lower := strings.ToLower(cleaned)
	if strings.HasPrefix(lower, "ssh://") || strings.HasPrefix(lower, "git://") || isSCPLike(cleaned) {
		spec.CloneURL = cleaned
	}
	return spec, nil
}

func githubSpec(webKey string) (Spec, error) {
	webURL := "https://" + strings.TrimPrefix(webKey, "https://")
	parsed, err := url.Parse(webURL)
	if err != nil || parsed.Hostname() == "" {
		return Spec{}, fmt.Errorf("%w: invalid GitHub repository", ErrInvalidRemote)
	}
	return Spec{Provider: ProviderGitHub, WebURL: webURL, CloneURL: webURL + ".git"}, nil
}

// ManagedRoot is the directory that holds tld-cloned checkouts.
func ManagedRoot(dataDir string) string {
	return filepath.Join(dataDir, "repositories")
}

// ManagedDir returns the deterministic checkout path for a remote spec.
func ManagedDir(dataDir string, spec Spec) string {
	sum := sha256.Sum256([]byte(spec.WebURL))
	slug := strings.TrimPrefix(spec.WebURL, "https://")
	slug = strings.NewReplacer("/", "-", ":", "-", "@", "-").Replace(slug)
	slug = strings.Trim(slug, "-.")
	return filepath.Join(ManagedRoot(dataDir), slug+"-"+hex.EncodeToString(sum[:4]))
}

// IsManagedPath reports whether root is one of tld's cloned checkouts.
func IsManagedPath(dataDir, root string) bool {
	managedRoot := filepath.Clean(ManagedRoot(dataDir))
	clean := filepath.Clean(root)
	return clean != managedRoot && strings.HasPrefix(clean, managedRoot+string(filepath.Separator))
}

// Clone checks out spec into dest. An existing checkout is reused. It prefers a
// blob-filtered clone and falls back to a full clone for older git versions.
func Clone(ctx context.Context, spec Spec, dest string) error {
	if _, err := os.Stat(filepath.Join(dest, ".git")); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create clone directory: %w", err)
	}

	output, err := runGit(ctx, "clone", "--filter=blob:none", "--", spec.CloneURL, dest)
	if err == nil {
		return nil
	}
	_ = os.RemoveAll(dest)
	fallbackOutput, fallbackErr := runGit(ctx, "clone", "--", spec.CloneURL, dest)
	if fallbackErr == nil {
		return nil
	}
	detail := strings.TrimSpace(fallbackOutput)
	if detail == "" {
		detail = strings.TrimSpace(output)
	}
	if detail == "" {
		detail = fallbackErr.Error()
	}
	return fmt.Errorf("git clone failed: %s", detail)
}

func runGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func isSCPLike(value string) bool {
	if strings.Contains(value, "://") {
		return false
	}
	return strings.Contains(value, "@") && strings.Contains(value, ":")
}

func looksLikeWindowsPath(value string) bool {
	return len(value) >= 2 && value[1] == ':' &&
		((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z'))
}
