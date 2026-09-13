package git

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FileChangesSince returns files changed between fromSHA and HEAD, keyed by
// repository-relative path.
func FileChangesSince(repoRoot, fromSHA string) (map[string]WorktreeChange, error) {
	fromSHA = strings.TrimSpace(fromSHA)
	if fromSHA == "" {
		return nil, nil
	}
	out, err := run(repoRoot, "diff", "--name-status", fromSHA+"..HEAD", "--")
	if err != nil {
		return nil, fmt.Errorf("git diff name-status: %w", err)
	}
	changes := map[string]WorktreeChange{}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		code := strings.TrimSpace(fields[0])
		switch {
		case strings.HasPrefix(code, "R") || strings.HasPrefix(code, "C"):
			if len(fields) >= 3 {
				changes[filepath.ToSlash(fields[1])] = WorktreeDeleted
				changes[filepath.ToSlash(fields[2])] = WorktreeAdded
			}
		case strings.HasPrefix(code, "A"):
			changes[filepath.ToSlash(fields[1])] = WorktreeAdded
		case strings.HasPrefix(code, "D"):
			changes[filepath.ToSlash(fields[1])] = WorktreeDeleted
		default:
			changes[filepath.ToSlash(fields[1])] = WorktreeUpdated
		}
	}
	return changes, nil
}

// MergeBase returns the best common ancestor commit between ref and HEAD.
func MergeBase(repoRoot, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("merge base: empty ref")
	}
	out, err := run(repoRoot, "merge-base", ref, "HEAD")
	if err != nil {
		return "", fmt.Errorf("git merge-base: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// FileChangesAgainstBase returns files changed between the merge base of ref and
// HEAD, keyed by repository-relative path.
func FileChangesAgainstBase(repoRoot, ref string) (map[string]WorktreeChange, error) {
	base, err := MergeBase(repoRoot, ref)
	if err != nil {
		return nil, err
	}
	return FileChangesSince(repoRoot, base)
}

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
