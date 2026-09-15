package git

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// CommitFile describes a single file touched by a commit.
type CommitFile struct {
	Path    string
	Change  WorktreeChange
	Added   int
	Removed int
}

// graphLogFormat emits one record per commit: fields separated by \x1f,
// records terminated by \x1e. The body is last so embedded newlines cannot
// break record splitting.
const graphLogFormat = "%H%x1f%h%x1f%P%x1f%D%x1f%an%x1f%ae%x1f%ad%x1f%s%x1f%b%x1e"

// HistoryGraph returns up to limit recent commits, newest first, including
// merge commits and carrying parent links plus ref decorations so callers can
// render a branching commit graph.
func HistoryGraph(repoRoot string, limit int) ([]Commit, error) {
	if limit <= 0 || limit > 200 {
		limit = 60
	}
	out, err := run(repoRoot, "log", "-n", strconv.Itoa(limit),
		"--decorate=short", "--format="+graphLogFormat, "--date=short")
	if err != nil {
		return nil, fmt.Errorf("git log: %w", err)
	}
	return parseGraphLog(out), nil
}

// ShowCommit returns the header of a single commit plus the files it changed.
// Merge commits are diffed against their first parent.
func ShowCommit(repoRoot, sha string) (Commit, []CommitFile, int, int, error) {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return Commit{}, nil, 0, 0, fmt.Errorf("show commit: empty sha")
	}
	if err := ensureCommit(repoRoot, sha); err != nil {
		return Commit{}, nil, 0, 0, err
	}
	out, err := run(repoRoot, "show", "-s", "--decorate=short",
		"--format="+graphLogFormat, "--date=short", sha)
	if err != nil {
		return Commit{}, nil, 0, 0, fmt.Errorf("git show: %w", err)
	}
	parsed := parseGraphLog(out)
	if len(parsed) == 0 {
		return Commit{}, nil, 0, 0, fmt.Errorf("show commit: %q not found", sha)
	}
	header := parsed[0]

	var statusOut, numstatOut string
	if len(header.Parents) > 1 {
		statusOut, err = run(repoRoot, "diff", "--name-status", header.Parents[0], sha, "--")
		if err != nil {
			return Commit{}, nil, 0, 0, fmt.Errorf("git diff name-status: %w", err)
		}
		numstatOut, err = run(repoRoot, "diff", "--numstat", "--no-renames", header.Parents[0], sha, "--")
		if err != nil {
			return Commit{}, nil, 0, 0, fmt.Errorf("git diff numstat: %w", err)
		}
	} else {
		statusOut, err = run(repoRoot, "diff-tree", "--no-commit-id", "--name-status", "-r", sha, "--")
		if err != nil {
			return Commit{}, nil, 0, 0, fmt.Errorf("git diff-tree name-status: %w", err)
		}
		numstatOut, err = run(repoRoot, "diff-tree", "--no-commit-id", "--numstat", "-r", "--no-renames", sha, "--")
		if err != nil {
			return Commit{}, nil, 0, 0, fmt.Errorf("git diff-tree numstat: %w", err)
		}
	}

	statuses := parseNameStatus(statusOut)
	counts := parseNumstat(numstatOut)
	files := make([]CommitFile, 0, len(statuses))
	added, removed := 0, 0
	for path, change := range statuses {
		count := counts[path]
		files = append(files, CommitFile{
			Path:    path,
			Change:  change,
			Added:   count.Added,
			Removed: count.Removed,
		})
		added += count.Added
		removed += count.Removed
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return header, files, added, removed, nil
}

// RangeCommitCount returns the number of commits in base..head, exclusive of
// base, following first-parent history like most graph views.
func RangeCommitCount(repoRoot, base, head string) (int, error) {
	base = strings.TrimSpace(base)
	head = strings.TrimSpace(head)
	if base == "" || head == "" {
		return 0, fmt.Errorf("range commit count: base and head are required")
	}
	out, err := run(repoRoot, "rev-list", "--count", base+".."+head, "--")
	if err != nil {
		return 0, fmt.Errorf("git rev-list --count %s..%s: %w", base, head, err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return 0, fmt.Errorf("git rev-list --count: %w", err)
	}
	return count, nil
}

func ensureCommit(repoRoot, sha string) error {
	if err := func() error {
		_, err := run(repoRoot, "cat-file", "-e", sha+"^{commit}")
		return err
	}(); err != nil {
		return fmt.Errorf("show commit: %q is not a commit: %w", sha, err)
	}
	return nil
}

func parseGraphLog(out string) []Commit {
	var commits []Commit
	for _, record := range strings.Split(out, "\x1e") {
		if strings.TrimSpace(record) == "" {
			continue
		}
		fields := strings.SplitN(record, "\x1f", 9)
		if len(fields) < 8 {
			continue
		}
		commit := Commit{
			SHA:         strings.TrimSpace(fields[0]),
			ShortSHA:    strings.TrimSpace(fields[1]),
			Author:      strings.TrimSpace(fields[4]),
			AuthorEmail: strings.TrimSpace(fields[5]),
			Date:        strings.TrimSpace(fields[6]),
			Subject:     strings.TrimSpace(fields[7]),
		}
		if commit.SHA == "" {
			continue
		}
		commit.Parents = append(commit.Parents, strings.Fields(strings.TrimSpace(fields[2]))...)
		for _, ref := range strings.Split(strings.TrimSpace(fields[3]), ",") {
			if ref = strings.TrimSpace(ref); ref != "" {
				commit.Refs = append(commit.Refs, ref)
			}
		}
		if len(fields) == 9 {
			commit.Body = strings.TrimSpace(fields[8])
		}
		commits = append(commits, commit)
	}
	return commits
}

// parseNumstat parses `git diff --numstat` output into per-path line counts.
// Binary files (reported as "-\t-") are skipped.
func parseNumstat(out string) map[string]LineDiff {
	stats := map[string]LineDiff{}
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 || fields[0] == "-" || fields[1] == "-" {
			continue
		}
		added, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		removed, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		stats[filepath.ToSlash(fields[2])] = LineDiff{Added: added, Removed: removed}
	}
	return stats
}
