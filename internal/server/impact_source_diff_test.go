package server

import (
	"context"
	"reflect"
	"testing"
)

func TestImpactSourceDiffUsesGitLineAlignment(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		changes             []string
		oldLines, newLines  []uint32
	}{
		{"replacement", "same\nold\nlast\n", "same\nnew\nlast\n", []string{"unchanged", "removed", "added", "unchanged"}, []uint32{1, 2, 0, 3}, []uint32{1, 0, 2, 3}},
		{"added file", "", "one\ntwo\n", []string{"added", "added"}, []uint32{0, 0}, []uint32{1, 2}},
		{"removed file", "one\ntwo\n", "", []string{"removed", "removed"}, []uint32{1, 2}, []uint32{0, 0}},
		{"whitespace edit", " x\n", "x\n", []string{"removed", "added"}, []uint32{1, 0}, []uint32{0, 1}},
		{"unchanged", "same\n", "same\n", []string{"unchanged"}, []uint32{1}, []uint32{1}},
		{"empty files", "", "", []string{}, []uint32{}, []uint32{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := impactSourceDiff(context.Background(), tc.before, tc.after)
			if err != nil {
				t.Fatal(err)
			}
			changes, oldLines, newLines := []string{}, []uint32{}, []uint32{}
			before, after := "", ""
			for _, row := range rows {
				changes = append(changes, row.Change)
				oldLines = append(oldLines, row.OldLine)
				newLines = append(newLines, row.NewLine)
				if row.OldLine != 0 {
					before += row.Text + "\n"
				}
				if row.NewLine != 0 {
					after += row.Text + "\n"
				}
			}
			if !reflect.DeepEqual(changes, tc.changes) || !reflect.DeepEqual(oldLines, tc.oldLines) || !reflect.DeepEqual(newLines, tc.newLines) {
				t.Fatalf("unexpected rows: %v", rows)
			}
			if before != tc.before || after != tc.after {
				t.Fatalf("diff did not reconstruct its inputs: %q -> %q", before, after)
			}
		})
	}
}

func TestImpactSourceDiffRetainsMissingFinalNewline(t *testing.T) {
	rows, err := impactSourceDiff(context.Background(), "old", "new")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[1].Change != "notice" || rows[3].Change != "notice" {
		t.Fatalf("missing EOF notices: %v", rows)
	}
}
