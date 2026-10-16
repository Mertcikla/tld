package index

import (
	"path/filepath"
	"strings"
)

// watchExcludes prevents a watcher from observing its own SQLite heartbeats,
// snapshots, and server logs when the app's data directory is in the checkout.
// Without this, persisting "debouncing" creates another filesystem event and
// keeps an otherwise idle repository in a debounce/write feedback loop.
func watchExcludes(root, dataDir string, configured []string) []string {
	excludes := append([]string(nil), configured...)
	if dataDir == "" {
		return excludes
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return excludes
	}
	absData, err := filepath.Abs(dataDir)
	if err != nil {
		return excludes
	}
	rel, err := filepath.Rel(absRoot, absData)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return excludes
	}
	if rel == "." {
		// Never exclude the entire checkout when data is stored at its root.
		return append(excludes, "tld.db", "tld.db-wal", "tld.db-shm", "tld.db-journal")
	}
	return append(excludes, filepath.ToSlash(rel))
}
