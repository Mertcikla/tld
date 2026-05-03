// Package git provides utilities for reading git repository context.
// All functions run git as a subprocess no CGO required.
package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Status struct {
	Branch      string
	HeadCommit  string
	HeadMessage string
	RemoteURL   string
	Staged      []string
	Unstaged    []string
	Untracked   []string
	Deleted     []string
}

type LineDiff struct {
	Added   int
	Removed int
}

// DetectBranch returns the current branch name for the git repo rooted at dir.
func DetectBranch(dir string) (string, error) {
	out, err := run(dir, "branch", "--show-current")
	if err != nil {
		return "", fmt.Errorf("detect branch: %w", err)
	}
	branch := strings.TrimSpace(out)
	if branch == "" {
		return "", fmt.Errorf("detect branch: HEAD is detached")
	}
	return branch, nil
}

// DetectRemoteURL returns the URL of the "origin" remote for the git repo at dir.
func DetectRemoteURL(dir string) (string, error) {
	out, err := run(dir, "remote", "get-url", "origin")
	if err != nil {
		return "", fmt.Errorf("detect remote url: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// DetectHeadCommit returns the current HEAD commit SHA for the git repo at dir.
func DetectHeadCommit(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("detect head commit: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// DetectHeadMessage returns the subject line for HEAD.
func DetectHeadMessage(dir string) (string, error) {
	out, err := run(dir, "log", "-1", "--format=%s")
	if err != nil {
		return "", fmt.Errorf("detect head message: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// DetectParentCommit returns the first parent commit SHA for HEAD.
func DetectParentCommit(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "HEAD^")
	if err != nil {
		return "", fmt.Errorf("detect parent commit: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// FileBlobHash returns the git blob hash for a tracked file at HEAD/index.
// filePath may be absolute or relative to dir.
func FileBlobHash(dir, filePath string) (string, error) {
	rel := filePath
	if filepath.IsAbs(filePath) {
		var err error
		rel, err = filepath.Rel(dir, filePath)
		if err != nil {
			return "", fmt.Errorf("file blob hash: %w", err)
		}
	}
	rel = filepath.ToSlash(rel)
	out, err := run(dir, "ls-files", "-s", "--", rel)
	if err != nil {
		return "", fmt.Errorf("file blob hash: %w", err)
	}
	fields := strings.Fields(out)
	if len(fields) < 2 {
		return "", fmt.Errorf("file blob hash: %q is not tracked", rel)
	}
	return fields[1], nil
}

// FileLastCommitAt returns the timestamp of the most recent commit that touched filePath
// in the git repo rooted at dir.  filePath may be absolute or relative to dir.
func FileLastCommitAt(dir, filePath string) (time.Time, error) {
	out, err := run(dir, "log", "-1", "--format=%ct", "--", filePath)
	if err != nil {
		return time.Time{}, fmt.Errorf("file last commit: %w", err)
	}
	s := strings.TrimSpace(out)
	if s == "" {
		return time.Time{}, fmt.Errorf("file last commit: no commits found for %q", filePath)
	}
	unix, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("file last commit: parse timestamp %q: %w", s, err)
	}
	return time.Unix(unix, 0).UTC(), nil
}

func StatusSnapshot(dir string) (Status, error) {
	status := Status{
		Branch:      detectBestEffort(func() (string, error) { return DetectBranch(dir) }),
		HeadCommit:  detectBestEffort(func() (string, error) { return DetectHeadCommit(dir) }),
		HeadMessage: detectBestEffort(func() (string, error) { return DetectHeadMessage(dir) }),
		RemoteURL:   detectBestEffort(func() (string, error) { return DetectRemoteURL(dir) }),
	}
	out, err := run(dir, "status", "--porcelain=v1", "-z")
	if err != nil {
		return status, fmt.Errorf("git status: %w", err)
	}
	entries := strings.Split(out, "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		if entry == "" || len(entry) < 4 {
			continue
		}
		x, y := entry[0], entry[1]
		path := strings.TrimSpace(entry[3:])
		if x == 'R' || x == 'C' {
			i++
		}
		if x != ' ' && x != '?' {
			status.Staged = append(status.Staged, filepath.ToSlash(path))
		}
		if y != ' ' && y != '?' {
			status.Unstaged = append(status.Unstaged, filepath.ToSlash(path))
		}
		if x == '?' && y == '?' {
			status.Untracked = append(status.Untracked, filepath.ToSlash(path))
		}
		if x == 'D' || y == 'D' {
			status.Deleted = append(status.Deleted, filepath.ToSlash(path))
		}
	}
	return status, nil
}

func LineDiffsAgainstHead(dir string) (map[string]LineDiff, error) {
	out, err := run(dir, "diff", "--numstat", "HEAD", "--")
	if err != nil {
		return nil, fmt.Errorf("git diff numstat: %w", err)
	}
	diffs := map[string]LineDiff{}
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
		diffs[filepath.ToSlash(fields[2])] = LineDiff{Added: added, Removed: removed}
	}
	return diffs, nil
}

// RepoRoot returns the absolute path of the top-level git working tree for the
// repository that contains dir.
func RepoRoot(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("repo root: %w", err)
	}
	return strings.TrimSpace(out), nil
}

func detectBestEffort(fn func() (string, error)) string {
	value, err := fn()
	if err != nil {
		return ""
	}
	return value
}

// run executes git with the given args in dir and returns the combined stdout output.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(out), nil
}
