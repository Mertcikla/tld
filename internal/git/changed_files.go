package git

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Commit is a compact commit record used by commit pickers.
type Commit struct {
	SHA      string
	ShortSHA string
	Subject  string
	Author   string
	Date     string
}

// RecentCommits returns up to limit recent commits, newest first, skipping
// merge commits so pickers show changes a human actually made.
func RecentCommits(repoRoot string, limit int) ([]Commit, error) {
	if limit <= 0 || limit > 200 {
		limit = 30
	}
	out, err := run(repoRoot, "log", "-n", strconv.Itoa(limit), "--no-merges",
		"--format=%H%x1f%h%x1f%an%x1f%ad%x1f%s", "--date=short")
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	var commits []Commit
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.SplitN(line, "\x1f", 5)
		if len(fields) < 5 {
			continue
		}
		commits = append(commits, Commit{
			SHA:      fields[0],
			ShortSHA: fields[1],
			Author:   fields[2],
			Date:     fields[3],
			Subject:  fields[4],
		})
	}
	return commits, nil
}

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
	return parseNameStatus(out), nil
}

// FileChangesBetween returns files changed between two arbitrary refs, keyed by
// repository-relative path. Unlike FileChangesSince it does not assume HEAD.
func FileChangesBetween(repoRoot, base, head string) (map[string]WorktreeChange, error) {
	base = strings.TrimSpace(base)
	head = strings.TrimSpace(head)
	if base == "" || head == "" {
		return nil, fmt.Errorf("file changes between: base and head are required")
	}
	out, err := run(repoRoot, "diff", "--name-status", base+".."+head, "--")
	if err != nil {
		return nil, fmt.Errorf("git diff name-status %s..%s: %w", base, head, err)
	}
	return parseNameStatus(out), nil
}

// MergeBaseBetween returns the best common ancestor commit between two refs.
func MergeBaseBetween(repoRoot, a, b string) (string, error) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return "", fmt.Errorf("merge base between: both refs are required")
	}
	out, err := run(repoRoot, "merge-base", a, b)
	if err != nil {
		return "", fmt.Errorf("git merge-base %s %s: %w", a, b, err)
	}
	return strings.TrimSpace(out), nil
}

func parseNameStatus(out string) map[string]WorktreeChange {
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
	return changes
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
