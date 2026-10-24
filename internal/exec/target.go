package exec

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/mertcikla/tld/v2/internal/client"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// CacheTarget identifies a destination independently of credentials. Hashing
// keeps connection details out of the workspace's portable lockfile.
func CacheTarget(cfg workspace.Config, targetOverride, dataDirFlag string) (string, error) {
	target, err := ResolveTarget(cfg, targetOverride)
	if err != nil {
		return "", err
	}
	var identity string
	if target == TargetRemote {
		identity = client.NormalizeURL(strings.TrimSpace(cfg.ServerURL)) + "\n" + strings.TrimSpace(cfg.WorkspaceID)
	} else {
		driver := strings.ToLower(strings.TrimSpace(cfg.Database.Driver))
		if driver == "postgres" || driver == "postgresql" {
			target = "postgres"
			u, err := url.Parse(strings.TrimSpace(cfg.Database.DatabaseURL))
			if err != nil {
				return "", fmt.Errorf("invalid postgres destination")
			}
			if u.User != nil {
				u.User = url.User(u.User.Username())
			}
			query := u.Query()
			query.Del("password")
			u.RawQuery = query.Encode()
			identity = u.String()
		} else {
			target = "sqlite"
			dir, err := workspace.ResolveDataDir(&cfg, dataDirFlag)
			if err != nil {
				return "", err
			}
			identity = canonicalTargetPath(filepath.Join(dir, "tld.db"))
		}
	}
	return fmt.Sprintf("%s:%x", target, sha256.Sum256([]byte(identity))), nil
}

// Resolve the existing prefix even before the database or data directory has
// been created, so the first write doesn't change a symlinked target's key.
func canonicalTargetPath(path string) string {
	parent := path
	var suffix []string
	for {
		if resolved, err := filepath.EvalSymlinks(parent); err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return resolved
		}
		next := filepath.Dir(parent)
		if next == parent {
			return path
		}
		suffix = append(suffix, filepath.Base(parent))
		parent = next
	}
}
