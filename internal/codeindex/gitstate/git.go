// Package gitstate captures Git's view of repository inputs without walking directories.
package gitstate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

func Run(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	raw, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(exit.Stderr)))
		}
		return "", err
	}
	return string(raw), nil
}

func Resolve(ctx context.Context, root, revision string) (string, error) {
	raw, err := Run(ctx, root, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	return strings.TrimSpace(raw), err
}

// CurrentCommit resolves the checked-out commit and its branch, for capturing a
// saved snapshot of HEAD without touching the working tree.
func CurrentCommit(ctx context.Context, root string) (branch, revision string, err error) {
	revision, err = Resolve(ctx, root, "HEAD")
	if err != nil {
		return "", "", fmt.Errorf("repository has no commit to capture: %w", err)
	}
	branch, _ = Run(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	return strings.TrimSpace(branch), revision, nil
}

type State struct {
	Revision, Branch, Signature string
	Paths                       []string
	Blobs                       map[string]string
	Dirty                       map[string]bool
}

// QuickState reads only dirty paths to detect repeated edits without scanning
// the complete repository.
type QuickState struct {
	Revision, Branch string
	// Dirty maps every changed (tracked, staged, or untracked) path to true.
	Dirty map[string]bool
	// Paths is Dirty's keys sorted for deterministic signatures.
	Paths []string
	// StatusByPath holds the two-character porcelain code for each path.
	StatusByPath  map[string]string
	ContentHashes map[string]string
}

// Signature derives a stable value that changes whenever HEAD, the branch, or
// dirty paths, their status, or their contents change.
func (q QuickState) Signature() string {
	parts := []string{q.Revision, q.Branch}
	for _, path := range q.Paths {
		parts = append(parts, path, q.StatusByPath[path], q.ContentHashes[path])
	}
	return graph.ID(parts...)
}

// CaptureQuick captures Git status and hashes only changed paths. It backs
// the watcher's poll loop and fsnotify fallback.
func CaptureQuick(ctx context.Context, root string) (QuickState, error) {
	qs := QuickState{Dirty: map[string]bool{}, StatusByPath: map[string]string{}, ContentHashes: map[string]string{}}
	revision, err := Resolve(ctx, root, "HEAD")
	if err != nil {
		return qs, fmt.Errorf("watch requires a Git repository with an existing commit: %w", err)
	}
	qs.Revision = revision
	branch, _ := Run(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	qs.Branch = strings.TrimSpace(branch)
	status, err := Run(ctx, root, "-c", "status.relativePaths=true", "status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=all", "--ignore-submodules=all", "--", ".")
	if err != nil {
		return qs, err
	}
	for _, record := range strings.Split(status, "\x00") {
		if len(record) < 4 {
			continue
		}
		code, path := record[:2], record[3:]
		if strings.Contains(code, "U") || code == "AA" || code == "DD" {
			return qs, fmt.Errorf("resolve merge conflicts before indexing: %s", path)
		}
		qs.Dirty[path] = true
		qs.StatusByPath[path] = code
	}
	for path := range qs.Dirty {
		qs.Paths = append(qs.Paths, path)
	}
	sort.Strings(qs.Paths)
	for _, path := range qs.Paths {
		if err := ctx.Err(); err != nil {
			return qs, err
		}
		full := filepath.Join(root, filepath.FromSlash(path))
		info, err := os.Lstat(full)
		if errors.Is(err, os.ErrNotExist) {
			qs.ContentHashes[path] = "deleted"
			continue
		}
		if err != nil {
			return qs, err
		}
		switch {
		case info.Mode().IsRegular():
			raw, err := os.ReadFile(full)
			if err != nil {
				return qs, err
			}
			qs.ContentHashes[path] = graph.Hash(raw)
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(full)
			if err != nil {
				return qs, err
			}
			qs.ContentHashes[path] = graph.Hash([]byte(target))
		default:
			qs.ContentHashes[path] = info.Mode().String()
		}
	}
	return qs, nil
}

// PathChange is one entry of a name-status diff.
type PathChange struct {
	Path   string
	Status byte
}

// NameStatus lists the paths that differ between the given tree-ish and the
// working tree, including staged and unstaged edits but excluding untracked
// files. Deleted paths are reported with Status 'D'.
func NameStatus(ctx context.Context, root, base string) ([]PathChange, error) {
	args := []string{"diff", "--name-status", "-z", "--no-renames"}
	if base != "" {
		args = append(args, "--end-of-options", base)
	}
	args = append(args, "--", ".")
	raw, err := Run(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(raw, "\x00")
	out := make([]PathChange, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		status, path := strings.TrimSpace(fields[i]), fields[i+1]
		if status == "" || path == "" {
			continue
		}
		out = append(out, PathChange{Path: path, Status: status[0]})
	}
	return out, nil
}

func Capture(ctx context.Context, root string) (State, error) {
	state := State{Blobs: map[string]string{}, Dirty: map[string]bool{}}
	revision, err := Resolve(ctx, root, "HEAD")
	if err != nil {
		return state, fmt.Errorf("watch requires a Git repository with an existing commit: %w", err)
	}
	state.Revision = revision
	branch, _ := Run(ctx, root, "symbolic-ref", "--quiet", "--short", "HEAD")
	state.Branch = strings.TrimSpace(branch)
	status, err := Run(ctx, root, "-c", "status.relativePaths=true", "status", "--porcelain=v1", "-z", "--no-renames", "--untracked-files=all", "--ignore-submodules=all", "--", ".")
	if err != nil {
		return state, err
	}
	parts := []string{revision, state.Branch, status}
	for _, record := range strings.Split(status, "\x00") {
		if len(record) < 4 {
			continue
		}
		code, path := record[:2], record[3:]
		if strings.Contains(code, "U") || code == "AA" || code == "DD" {
			return state, fmt.Errorf("resolve merge conflicts before indexing: %s", path)
		}
		state.Dirty[path] = true
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path)))
		if errors.Is(err, os.ErrNotExist) {
			parts = append(parts, path, "deleted")
			continue
		}
		if err != nil {
			return state, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return state, err
		}
		parts = append(parts, path, graph.Hash(raw))
	}
	listed, err := Run(ctx, root, "ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", ".")
	if err != nil {
		return state, err
	}
	seen := map[string]bool{}
	for _, path := range strings.Split(listed, "\x00") {
		if path != "" && !seen[path] {
			seen[path] = true
			state.Paths = append(state.Paths, path)
		}
	}
	sort.Strings(state.Paths)
	prefix, err := Run(ctx, root, "rev-parse", "--show-prefix")
	if err != nil {
		return state, err
	}
	prefix = strings.TrimSuffix(prefix, "\n")
	tree, err := Run(ctx, root, "ls-tree", "-r", "--full-tree", "-z", revision)
	if err != nil {
		return state, err
	}
	for _, record := range strings.Split(tree, "\x00") {
		meta, path, ok := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || fields[1] != "blob" || !strings.HasPrefix(path, prefix) {
			continue
		}
		state.Blobs[strings.TrimPrefix(path, prefix)] = fields[2]
	}
	// Index membership changes matter even if the net working-tree diff is empty.
	parts = append(parts, state.Paths...)
	state.Signature = graph.ID(parts...)
	return state, nil
}

// Commits returns newly reachable commits in dependency order, including commits
// created between polls. A reset or unrelated checkout captures its current tip.
func Commits(ctx context.Context, root, previous, current string) ([]string, error) {
	if previous == current {
		return nil, nil
	}
	if previous == "" {
		return []string{current}, nil
	}
	if _, err := Run(ctx, root, "merge-base", "--is-ancestor", previous, current); err != nil {
		return []string{current}, nil
	}
	raw, err := Run(ctx, root, "rev-list", "--reverse", "--topo-order", previous+".."+current)
	return strings.Fields(raw), err
}

func WithCommit(ctx context.Context, root, revision string, run func(string) error) (err error) {
	parent, err := os.MkdirTemp("", "tld-index-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(parent)) }()
	// Git canonicalizes macOS temporary paths through /private; match its
	// registration path when cleaning up even after checkout fails.
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	checkout := filepath.Join(parent, "checkout")
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		registered, listErr := Run(cleanupCtx, root, "worktree", "list", "--porcelain")
		if listErr != nil {
			err = errors.Join(err, listErr)
			return
		}
		if strings.Contains(registered, "worktree "+checkout+"\n") {
			_, cleanupErr := Run(cleanupCtx, root, "worktree", "remove", "--force", checkout)
			err = errors.Join(err, cleanupErr)
		}
	}()
	if _, err = Run(ctx, root, "worktree", "add", "--detach", checkout, revision); err != nil {
		return err
	}
	prefix, err := Run(ctx, root, "rev-parse", "--show-prefix")
	if err != nil {
		return err
	}
	return run(filepath.Join(checkout, filepath.FromSlash(strings.TrimSpace(prefix))))
}
