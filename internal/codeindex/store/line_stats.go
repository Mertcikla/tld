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
func (s *Store) sourceLineStats(ctx context.Context, changes []*pb.SourceChange) error {
	var directory string
	defer func() {
		if directory != "" {
			_ = os.RemoveAll(directory)
		}
	}()
	for _, change := range changes {
		if change.LinesAdded != nil && change.LinesRemoved != nil {
			continue
		}
		contents := [2][]byte{}
		for i, hash := range []string{change.FromHash, change.ToHash} {
			if hash == "" {
				continue
			}
			var err error
			contents[i], err = s.Source(ctx, hash)
			if err != nil {
				return fmt.Errorf("read %s for line counts: %w", change.Path, err)
			}
		}
		if directory == "" {
			var err error
			directory, err = os.MkdirTemp("", "tld-line-diff-")
			if err != nil {
				return err
			}
		}
		before, after := filepath.Join(directory, "before"), filepath.Join(directory, "after")
		for i, path := range []string{before, after} {
			if err := os.WriteFile(path, contents[i], 0600); err != nil {
				return err
			}
		}
		cmd := exec.CommandContext(ctx, "git", "-c", "diff.algorithm=myers", "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-renames", "--text", "--numstat", "--", before, after)
		raw, err := cmd.Output()
		var exit *exec.ExitError
		if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
			return fmt.Errorf("count changed lines: %w", err)
		}
		added, removed := uint32(0), uint32(0)
		if fields := strings.Fields(string(raw)); len(fields) >= 2 {
			a, err := strconv.ParseUint(fields[0], 10, 32)
			if err != nil {
				return err
			}
			r, err := strconv.ParseUint(fields[1], 10, 32)
			if err != nil {
				return err
			}
			added, removed = uint32(a), uint32(r)
		}
		change.LinesAdded, change.LinesRemoved = &added, &removed
	}
	return nil
}
