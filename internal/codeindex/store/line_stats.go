package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

// sourceLineStats compares captured contents, so working-tree edits and later
// checkouts cannot change a historical comparison's insertion/deletion counts.
// All pending changes are written into two mirror trees and counted with one
// git invocation: spawning git per changed file dominated large comparisons.
func (s *Store) sourceLineStats(ctx context.Context, changes []*pb.SourceChange) error {
	pending := make([]*pb.SourceChange, 0, len(changes))
	for _, change := range changes {
		if change.GetFactsUnavailable() {
			continue
		}
		if change.LinesAdded != nil && change.LinesRemoved != nil {
			continue
		}
		pending = append(pending, change)
	}
	if len(pending) == 0 {
		return nil
	}
	directory, err := os.MkdirTemp("", "tld-line-diff-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(directory) }()
	beforeDir, afterDir := filepath.Join(directory, "before"), filepath.Join(directory, "after")
	for _, root := range []string{beforeDir, afterDir} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return err
		}
	}
	names := make(map[*pb.SourceChange]string, len(pending))
	byName := make(map[string]*pb.SourceChange, len(pending))
	for i, change := range pending {
		name := safeMirrorName(change.Path)
		if name == "" {
			name = fmt.Sprintf("__change_%d", i)
		}
		names[change] = name
		byName[name] = change
	}
	for _, change := range pending {
		for i, hash := range []string{change.FromHash, change.ToHash} {
			if hash == "" {
				continue
			}
			contents, err := s.Source(ctx, hash)
			if err != nil {
				return fmt.Errorf("read %s for line counts: %w", change.Path, err)
			}
			root := beforeDir
			if i == 1 {
				root = afterDir
			}
			target := filepath.Join(root, filepath.FromSlash(names[change]))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, contents, 0o600); err != nil {
				return err
			}
		}
	}
	cmd := exec.CommandContext(ctx, "git", "-c", "diff.algorithm=myers", "-c", "core.quotePath=false", "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-renames", "--text", "--numstat", "-z", "--", beforeDir, afterDir)
	raw, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return fmt.Errorf("count changed lines: %w", err)
	}
	for name, pair := range parseNumstat(raw, beforeDir, afterDir) {
		change := byName[name]
		if change == nil {
			continue
		}
		added, removed := pair[0], pair[1]
		change.LinesAdded, change.LinesRemoved = &added, &removed
	}
	for _, change := range pending {
		if change.LinesAdded != nil || change.LinesRemoved != nil {
			continue
		}
		added, removed := uint32(0), uint32(0)
		change.LinesAdded, change.LinesRemoved = &added, &removed
	}
	return nil
}

// safeMirrorName normalizes a change path for the mirror tree, rejecting paths
// that would escape it. Unsafe paths get a synthetic name instead.
func safeMirrorName(path string) string {
	rel := filepath.Clean(filepath.FromSlash(path))
	if rel == "." || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return rel
}

// parseNumstat reads git's -z numstat stream. Added/removed files carry a path
// in the header; paired paths have an empty header path followed by two tokens
// because the mirror roots differ. Paths with tabs or braces need no unquoting.
func parseNumstat(raw []byte, beforeDir, afterDir string) map[string][2]uint32 {
	counts := map[string][2]uint32{}
	tokens := strings.Split(string(raw), "\x00")
	for i := 0; i < len(tokens); i++ {
		parts := strings.SplitN(tokens[i], "\t", 3)
		if len(parts) != 3 {
			continue
		}
		added, addedErr := strconv.ParseUint(parts[0], 10, 32)
		removed, removedErr := strconv.ParseUint(parts[1], 10, 32)
		if addedErr != nil || removedErr != nil {
			continue
		}
		src, dst := parts[2], ""
		if src == "" {
			if i+2 >= len(tokens) {
				continue
			}
			src, dst = tokens[i+1], tokens[i+2]
			i += 2
		}
		rel, ok := clipMirrorPath(src, beforeDir, afterDir)
		if !ok {
			rel, ok = clipMirrorPath(dst, beforeDir, afterDir)
		}
		if !ok {
			continue
		}
		counts[rel] = [2]uint32{uint32(added), uint32(removed)}
	}
	return counts
}

// clipMirrorPath maps a numstat path back to its repository-relative form,
// stripping whichever mirror tree git reported.
func clipMirrorPath(path, beforeDir, afterDir string) (string, bool) {
	for _, root := range []string{beforeDir, afterDir} {
		if rel, ok := strings.CutPrefix(path, root+string(filepath.Separator)); ok {
			return filepath.ToSlash(rel), true
		}
	}
	return "", false
}
