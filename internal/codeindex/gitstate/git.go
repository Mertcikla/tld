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

type State struct {
	Revision, Branch, Signature string
	Paths                       []string
	Blobs                       map[string]string
	Dirty                       map[string]bool
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
