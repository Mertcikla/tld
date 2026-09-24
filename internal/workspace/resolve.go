package workspace

import (
	"os"
	"path/filepath"
)

// workspaceMarkerFiles are the files that identify a directory as a workspace.
var workspaceMarkerFiles = []string{".tld.yaml", "elements.yaml", "connectors.yaml", ".tld.lock"}

// ResolveDir maps a workspace root to the directory that actually holds the
// workspace files. A root that already looks like a workspace is returned
// unchanged; otherwise the conventional nested "<root>/.tld" directory is used
// when it exists. Empty input is returned as-is so callers keep their
// current-directory semantics.
func ResolveDir(root string) string {
	if root == "" {
		return ""
	}
	if IsWorkspaceDir(root) {
		return root
	}
	nested := filepath.Join(root, ".tld")
	if IsWorkspaceDir(nested) {
		return nested
	}
	return root
}

// IsWorkspaceDir reports whether dir directly contains workspace files.
func IsWorkspaceDir(dir string) bool {
	for _, name := range workspaceMarkerFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}
