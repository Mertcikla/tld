// Package git provides utilities for reading git repository context.
// All functions run git as a subprocess no CGO required.
package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

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

// RepoRoot returns the absolute path of the top-level git working tree for the
// repository that contains dir.
func RepoRoot(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("repo root: %w", err)
	}
	return strings.TrimSpace(out), nil
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
