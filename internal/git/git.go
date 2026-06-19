// Package git provides utilities for reading git repository context.
// All functions run git as a subprocess no CGO required.
package git

import (
	"bufio"
	"fmt"
	"io"
	"os"
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

type LineHunk struct {
	File         string
	OldStart     int
	OldLineCount int
	NewStart     int
	NewLineCount int
	AddedLines   []int
	RemovedLines []int
}

type WorktreeChange string

const (
	WorktreeAdded   WorktreeChange = "added"
	WorktreeUpdated WorktreeChange = "updated"
	WorktreeDeleted WorktreeChange = "deleted"
)

const (
	FileStateOutsideRepo = "outside_repo"
	FileStateIgnored     = "ignored"
	FileStateUntracked   = "untracked"
	FileStateModified    = "modified"
	FileStateDeleted     = "deleted"
	FileStateTracked     = "tracked"
	FileStateUnknown     = "unknown"
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

// FileBlobHashes returns git blob hashes for tracked files at HEAD/index,
// keyed by repository-relative slash paths.
func FileBlobHashes(dir string) (map[string]string, error) {
	cmd := exec.Command("git", "ls-files", "-s", "-z")
	cmd.Dir = dir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("git ls-files -s: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git ls-files -s: %w", err)
	}
	hashes := map[string]string{}
	reader := bufio.NewReader(stdout)
	for {
		raw, readErr := reader.ReadString(0)
		entry := strings.TrimSuffix(raw, "\x00")
		if entry != "" {
			meta, path, ok := strings.Cut(entry, "\t")
			fields := strings.Fields(meta)
			if ok && len(fields) >= 2 {
				hashes[filepath.ToSlash(path)] = fields[1]
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = cmd.Wait()
			return nil, fmt.Errorf("git ls-files -s: %w", readErr)
		}
	}
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git ls-files -s: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("git ls-files -s: %w", err)
	}
	return hashes, nil
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

// RecentChangedFiles returns unique repository-relative paths touched by local
// commit history, newest commits first. It never contacts remotes.
func RecentChangedFiles(dir string, limit int) ([]string, error) {
	cmd := exec.Command("git", "log", "--diff-filter=ACMR", "--name-only", "-z", "--format=", "--")
	cmd.Dir = dir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("git log recent files: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("git log recent files: %w", err)
	}
	seen := map[string]struct{}{}
	var files []string
	reader := bufio.NewReader(stdout)
	for {
		raw, readErr := reader.ReadString(0)
		entry := filepath.ToSlash(strings.TrimSpace(strings.TrimSuffix(raw, "\x00")))
		if entry != "" {
			if _, ok := seen[entry]; !ok {
				seen[entry] = struct{}{}
				// Verify if the file exists on disk.
				if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(entry))); err == nil {
					files = append(files, entry)
					if limit > 0 && len(files) >= limit {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
						return files, nil
					}
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			_ = cmd.Wait()
			return nil, fmt.Errorf("git log recent files: %w", readErr)
		}
	}
	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git log recent files: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("git log recent files: %w", err)
	}
	return files, nil
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

// FileState returns a compact git state for a single path. filePath may be
// absolute or relative to dir. The returned path is repository-relative.
func FileState(dir, filePath string) (string, string, error) {
	root, err := RepoRoot(dir)
	if err != nil {
		return FileStateOutsideRepo, "", nil
	}
	absPath := filePath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(dir, filePath)
	}
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return FileStateUnknown, "", err
	}
	relRoot := root
	if evaluated, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
		relRoot = evaluated
	}
	relPath := absPath
	if evaluated, evalErr := filepath.EvalSymlinks(absPath); evalErr == nil {
		relPath = evaluated
	} else if parent, parentErr := filepath.EvalSymlinks(filepath.Dir(absPath)); parentErr == nil {
		relPath = filepath.Join(parent, filepath.Base(absPath))
	}
	rel, err := filepath.Rel(relRoot, relPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return FileStateOutsideRepo, "", nil
	}
	rel = filepath.ToSlash(rel)
	if ignored, err := IsIgnored(root, rel); err != nil {
		return FileStateUnknown, rel, err
	} else if ignored {
		return FileStateIgnored, rel, nil
	}
	status, err := StatusSnapshot(root)
	if err != nil {
		return FileStateUnknown, rel, err
	}
	if stringSliceContains(status.Deleted, rel) {
		return FileStateDeleted, rel, nil
	}
	if stringSliceContains(status.Untracked, rel) {
		return FileStateUntracked, rel, nil
	}
	if stringSliceContains(status.Staged, rel) || stringSliceContains(status.Unstaged, rel) {
		return FileStateModified, rel, nil
	}
	if _, err := FileBlobHash(root, rel); err == nil {
		return FileStateTracked, rel, nil
	}
	return FileStateUntracked, rel, nil
}

func IsIgnored(dir, filePath string) (bool, error) {
	cmd := exec.Command("git", "check-ignore", "-q", "--", filepath.ToSlash(filePath))
	cmd.Dir = dir
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return false, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return false, fmt.Errorf("git check-ignore: %s", strings.TrimSpace(string(exitErr.Stderr)))
	}
	return false, fmt.Errorf("git check-ignore: %w", err)
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func WorktreeChangesAgainstHead(dir string) (map[string]WorktreeChange, error) {
	status, err := StatusSnapshot(dir)
	if err != nil {
		return nil, err
	}
	changes := map[string]WorktreeChange{}
	if status.HeadCommit != "" {
		out, err := run(dir, "diff", "--name-status", "HEAD", "--")
		if err != nil {
			return nil, fmt.Errorf("git diff name-status: %w", err)
		}
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
	}
	for _, path := range status.Untracked {
		changes[filepath.ToSlash(path)] = WorktreeAdded
	}
	if status.HeadCommit == "" {
		for _, path := range status.Staged {
			changes[filepath.ToSlash(path)] = WorktreeAdded
		}
		for _, path := range status.Unstaged {
			if _, ok := changes[filepath.ToSlash(path)]; !ok {
				changes[filepath.ToSlash(path)] = WorktreeAdded
			}
		}
		for _, path := range status.Deleted {
			changes[filepath.ToSlash(path)] = WorktreeDeleted
		}
	}
	return changes, nil
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

func LineHunksAgainstHead(dir string) (map[string][]LineHunk, error) {
	out, err := run(dir, "diff", "--unified=0", "HEAD", "--")
	if err != nil {
		return nil, fmt.Errorf("git diff hunks: %w", err)
	}
	return ParseLineHunks(out), nil
}

func ParseLineHunks(diff string) map[string][]LineHunk {
	hunks := map[string][]LineHunk{}
	file := ""
	var current *LineHunk
	oldLine, newLine := 0, 0
	flush := func() {
		if current != nil && file != "" {
			current.File = file
			hunks[file] = append(hunks[file], *current)
		}
		current = nil
	}
	for line := range strings.SplitSeq(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flush()
			file = parseDiffGitPath(line)
		case strings.HasPrefix(line, "+++ b/"):
			file = filepath.ToSlash(strings.TrimPrefix(line, "+++ b/"))
		case strings.HasPrefix(line, "@@ "):
			flush()
			hunk, ok := parseHunkHeader(line)
			if !ok {
				continue
			}
			current = &hunk
			oldLine = hunk.OldStart
			newLine = hunk.NewStart
		case current != nil && strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			current.AddedLines = append(current.AddedLines, newLine)
			newLine++
		case current != nil && strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			current.RemovedLines = append(current.RemovedLines, oldLine)
			oldLine++
		case current != nil && strings.HasPrefix(line, " "):
			oldLine++
			newLine++
		}
	}
	flush()
	return hunks
}

func parseDiffGitPath(line string) string {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return ""
	}
	path := strings.TrimPrefix(fields[3], "b/")
	return filepath.ToSlash(path)
}

func parseHunkHeader(line string) (LineHunk, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || !strings.HasPrefix(fields[1], "-") || !strings.HasPrefix(fields[2], "+") {
		return LineHunk{}, false
	}
	oldStart, oldCount, ok := parseHunkRange(strings.TrimPrefix(fields[1], "-"))
	if !ok {
		return LineHunk{}, false
	}
	newStart, newCount, ok := parseHunkRange(strings.TrimPrefix(fields[2], "+"))
	if !ok {
		return LineHunk{}, false
	}
	return LineHunk{OldStart: oldStart, OldLineCount: oldCount, NewStart: newStart, NewLineCount: newCount}, true
}

func parseHunkRange(value string) (int, int, bool) {
	parts := strings.SplitN(value, ",", 2)
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	count := 1
	if len(parts) == 2 {
		count, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, false
		}
	}
	return start, count, true
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
