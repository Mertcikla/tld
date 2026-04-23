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
	"sort"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/workspace"
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

func BuildPlanJSON(ws *workspace.Workspace, resp *diagv1.ApplyPlanResponse, warnings []planner.WarningGroup) planner.JSONOutput {
	items, summary := planJSONItems(ws)
	output := planner.JSONOutput{
		Command: "plan",
		Status:  "ok",
		Summary: summary,
		Items:   items,
	}
	if len(resp.GetConflicts()) > 0 {
		output.Status = "conflict"
		for _, conflict := range resp.GetConflicts() {
			output.Items = append(output.Items, planner.JSONItem{
				Ref:          conflict.GetRef(),
				ResourceType: conflict.GetResourceType(),
				Action:       "conflict",
				Reason:       conflict.GetResolutionHint(),
			})
		}
	}
	for _, drift := range resp.GetDrift() {
		output.Warnings = append(output.Warnings, fmt.Sprintf("%s %s: %s", drift.GetResourceType(), drift.GetRef(), drift.GetReason()))
	}
	for _, group := range warnings {
		for _, violation := range group.Violations {
			output.Warnings = append(output.Warnings, fmt.Sprintf("[%s] %s: %s", group.RuleCode, group.RuleName, violation))
		}
	}
	return output
}

func BuildApplyJSON(ws *workspace.Workspace, resp *diagv1.ApplyPlanResponse, retries int) planner.JSONOutput {
	items, summary := planJSONItems(ws)
	output := planner.JSONOutput{
		Command: "apply",
		Status:  "ok",
		Summary: summary,
		Items:   items,
		Retries: retries,
	}
	if len(resp.GetConflicts()) > 0 {
		output.Status = "conflict"
		for _, conflict := range resp.GetConflicts() {
			output.Items = append(output.Items, planner.JSONItem{
				Ref:          conflict.GetRef(),
				ResourceType: conflict.GetResourceType(),
				Action:       "conflict",
				Reason:       conflict.GetResolutionHint(),
			})
		}
	}
	if len(resp.GetDrift()) > 0 {
		output.Status = "error"
		for _, drift := range resp.GetDrift() {
			output.Errors = append(output.Errors, fmt.Sprintf("%s %s: %s", drift.GetResourceType(), drift.GetRef(), drift.GetReason()))
		}
	}
	return output
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

func planJSONItems(ws *workspace.Workspace) ([]planner.JSONItem, map[string]int) {
	items := make([]planner.JSONItem, 0, len(ws.Elements)+len(ws.Connectors))
	summary := map[string]int{"created": 0, "updated": 0, "deleted": 0}
	included := IncludedElementRefs(ws)
	refs := make([]string, 0, len(included))
	for ref := range included {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	for _, ref := range refs {
		element := ws.Elements[ref]
		action := resourceAction(ws.Meta, elementMeta(ws), ref)
		summary[actionSummaryKey(action)]++
		items = append(items, planner.JSONItem{Ref: ref, ResourceType: "element", Action: action, Name: element.Name})
		if element.HasView {
			viewAction := resourceAction(ws.Meta, viewMeta(ws), ref)
			summary[actionSummaryKey(viewAction)]++
			items = append(items, planner.JSONItem{Ref: ref, ResourceType: "view", Action: viewAction, Name: element.ViewLabel})
		}
	}
	connectorRefs := make([]string, 0, len(ws.Connectors))
	for ref, connector := range ws.Connectors {
		if !included[connector.Source] || !included[connector.Target] {
			continue
		}
		connectorRefs = append(connectorRefs, ref)
	}
	sort.Strings(connectorRefs)
	for _, ref := range connectorRefs {
		connector := ws.Connectors[ref]
		action := resourceAction(ws.Meta, connectorMeta(ws), ref)
		summary[actionSummaryKey(action)]++
		items = append(items, planner.JSONItem{Ref: ref, ResourceType: "connector", Action: action, Name: connector.Label})
	}
	return items, summary
}

func resourceAction(meta *workspace.Meta, bucket map[string]*workspace.ResourceMetadata, ref string) string {
	if meta != nil && bucket != nil {
		if _, ok := bucket[ref]; ok {
			return "update"
		}
	}
	return "create"
}

func actionSummaryKey(action string) string {
	if action == "update" {
		return "updated"
	}
	return "created"
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

func elementMeta(ws *workspace.Workspace) map[string]*workspace.ResourceMetadata {
	if ws == nil || ws.Meta == nil {
		return nil
	}
	return ws.Meta.Elements
}

func viewMeta(ws *workspace.Workspace) map[string]*workspace.ResourceMetadata {
	if ws == nil || ws.Meta == nil {
		return nil
	}
	return ws.Meta.Views
}

func connectorMeta(ws *workspace.Workspace) map[string]*workspace.ResourceMetadata {
	if ws == nil || ws.Meta == nil {
		return nil
	}
	return ws.Meta.Connectors
}
