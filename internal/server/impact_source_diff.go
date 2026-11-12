package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

func (s *codeIndexService) capturedImpactSource(ctx context.Context, repositoryID, snapshotID, path string, allowAbsent bool) (string, error) {
	snapshot, err := s.store.Snapshot(ctx, snapshotID)
	if err != nil {
		return "", err
	}
	if snapshot.GetRepositoryId() != repositoryID {
		return "", sql.ErrNoRows
	}
	for _, source := range snapshot.GetSources() {
		if source.GetPath() != path {
			continue
		}
		content, err := s.store.Source(ctx, source.GetHash())
		return string(content), err
	}
	for _, file := range snapshot.GetManifest().GetFiles() {
		if file.GetPath() == path {
			return "", fmt.Errorf("source for %s was not captured", path)
		}
	}
	if allowAbsent && snapshot.GetManifest().GetComplete() {
		return "", nil
	}
	return "", sql.ErrNoRows
}

var impactHunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@`)

// Git supplies the alignment, including whitespace-only edits and deleted lines.
// Full context keeps every source line scrollable and retains original anchors.
func impactSourceDiff(ctx context.Context, before, after string) ([]*pb.ImpactDiffLine, error) {
	dir, err := os.MkdirTemp("", "tld-source-diff-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	base, head := filepath.Join(dir, "base"), filepath.Join(dir, "head")
	if err := os.WriteFile(base, []byte(before), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(head, []byte(after), 0o600); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "git", "-c", "diff.algorithm=myers", "-c", "core.autocrlf=false", "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-renames", "--text", "--unified=2147483647", "--", base, head)
	raw, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		return nil, fmt.Errorf("diff captured source: %w", err)
	}
	rows := []*pb.ImpactDiffLine{}
	var oldLine, newLine uint32
	inHunk := false
	for _, line := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if match := impactHunkHeader.FindStringSubmatch(line); match != nil {
			old, _ := strconv.ParseUint(match[1], 10, 32)
			next, _ := strconv.ParseUint(match[2], 10, 32)
			oldLine, newLine, inHunk = uint32(old), uint32(next), true
			continue
		}
		if !inHunk || line == "" {
			continue
		}
		row := &pb.ImpactDiffLine{Text: line[1:]}
		switch line[0] {
		case ' ':
			row.Change, row.OldLine, row.NewLine = "unchanged", oldLine, newLine
			oldLine++
			newLine++
		case '-':
			row.Change, row.OldLine = "removed", oldLine
			oldLine++
		case '+':
			row.Change, row.NewLine = "added", newLine
			newLine++
		case '\\':
			row.Change, row.Text = "notice", line
		default:
			continue
		}
		rows = append(rows, row)
	}
	if len(raw) == 0 && after != "" {
		for i, line := range strings.Split(strings.TrimSuffix(after, "\n"), "\n") {
			rows = append(rows, &pb.ImpactDiffLine{Change: "unchanged", OldLine: uint32(i + 1), NewLine: uint32(i + 1), Text: line})
		}
	}
	return rows, nil
}
