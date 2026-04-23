package git

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FilesChangedSince returns the list of files modified between fromSHA and HEAD.
func FilesChangedSince(repoRoot, fromSHA string) ([]string, error) {
	out, err := run(repoRoot, "diff", "--name-only", fromSHA+"..HEAD")
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil, nil
	}
	var files []string
	for line := range strings.SplitSeq(trimmed, "\n") {
		if line == "" {
			continue
		}
		files = append(files, filepath.Join(repoRoot, line))
	}
	return files, nil
}
