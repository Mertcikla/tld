package cmdutil

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/planner"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func WantsJSON(format string) bool {
	return strings.EqualFold(format, "json")
}

func WriteJSON(w io.Writer, compact bool, payload planner.JSONOutput) error {
	enc := json.NewEncoder(w)
	if !compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(payload)
}

func WriteCommandError(w io.Writer, compact bool, command string, err error) error {
	return WriteJSON(w, compact, planner.JSONOutput{
		Command: command,
		Status:  "error",
		Errors:  []string{err.Error()},
	})
}

func WriteMutation(w io.Writer, compact bool, command, action, ref string) error {
	return WriteJSON(w, compact, planner.JSONOutput{
		Command: command,
		Status:  "ok",
		Items: []planner.JSONItem{
			{
				Action: action,
				Ref:    ref,
			},
		},
	})
}

func BuildStatusJSON(lockFile *workspace.LockFile, localModified, serverDrift bool, conflicts int, serverResp *diagv1.ApplyPlanResponse) planner.JSONOutput {
	status := "no_history"
	if lockFile != nil {
		status = statusLabel(localModified, serverDrift, conflicts)
	}
	isMod := localModified || conflicts > 0 || serverDrift
	output := planner.JSONOutput{
		Command: "status",
		Status:  status,
		Summary: map[string]int{
			"conflicts":      conflicts,
			"local_modified": boolToInt(localModified),
			"server_drift":   boolToInt(serverDrift),
		},
		IsModified: &isMod,
	}
	if lockFile != nil {
		output.Extra = map[string]any{
			"version_id": lockFile.VersionID,
			"applied_by": lockFile.AppliedBy,
			"last_apply": lockFile.LastApply,
		}
	}
	if serverResp != nil {
		for _, drift := range serverResp.GetDrift() {
			output.Warnings = append(output.Warnings, fmt.Sprintf("%s %s: %s", drift.GetResourceType(), drift.GetRef(), drift.GetReason()))
		}
		for _, conflict := range serverResp.GetConflicts() {
			output.Warnings = append(output.Warnings, fmt.Sprintf("%s %s: remote newer", conflict.GetResourceType(), conflict.GetRef()))
		}
	}
	return output
}

func BuildDiffJSON(wdir, tempDir string) (planner.JSONOutput, error) {
	diffFiles, err := collectDiffFiles(wdir, tempDir)
	if err != nil {
		return planner.JSONOutput{}, err
	}
	return planner.JSONOutput{
		Command:   "diff",
		Status:    "ok",
		Summary:   map[string]int{"changed_files": len(diffFiles)},
		DiffFiles: diffFiles,
	}, nil
}

func IncludedElementRefs(ws *workspace.Workspace) map[string]bool {
	included := make(map[string]bool, len(ws.Elements))
	for ref, element := range ws.Elements {
		if ws.ActiveRepo != "" && element.Owner != "" && element.Owner != ws.ActiveRepo {
			continue
		}
		included[ref] = true
	}
	return included
}

func collectDiffFiles(wdir, tempDir string) ([]planner.JSONDiffFile, error) {
	cmd := exec.Command("git", "diff", "--no-index", "--unified=0", "--src-prefix=server/", "--dst-prefix=local/", tempDir, wdir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return nil, fmt.Errorf("git diff: %w", err)
		}
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}

	var files []planner.JSONDiffFile
	var current *planner.JSONDiffFile
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil && current.Path != "" {
				files = append(files, *current)
			}
			current = &planner.JSONDiffFile{}
			continue
		}
		if current == nil {
			continue
		}
		if after, ok := strings.CutPrefix(line, "+++ "); ok {
			current.Path = normalizeDiffPath(after, wdir, tempDir)
			continue
		}
		if current.Path == "" && strings.HasPrefix(line, "--- ") {
			current.Path = normalizeDiffPath(strings.TrimPrefix(line, "--- "), wdir, tempDir)
			continue
		}
		if strings.HasPrefix(line, "@@") {
			current.Hunks = append(current.Hunks, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan diff output: %w", err)
	}
	if current != nil && current.Path != "" {
		files = append(files, *current)
	}
	return files, nil
}

func normalizeDiffPath(rawPath, wdir, tempDir string) string {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" || rawPath == "/dev/null" {
		return ""
	}
	rawPath = strings.TrimPrefix(rawPath, "local/")
	rawPath = strings.TrimPrefix(rawPath, "server/")
	if filepath.IsAbs(rawPath) {
		if rel, err := filepath.Rel(wdir, rawPath); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
		if rel, err := filepath.Rel(tempDir, rawPath); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(strings.TrimPrefix(rawPath, "/"))
}

func statusLabel(localModified, serverDrift bool, conflicts int) string {
	if serverDrift {
		return "drifted"
	}
	if localModified || conflicts > 0 {
		return "modified"
	}
	return "in_sync"
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
