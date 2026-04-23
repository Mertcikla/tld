package views_test

import (
	"github.com/mertcikla/diag/tld/cmd"
	"encoding/json"
	"slices"
	"testing"

	"github.com/mertcikla/diag/tld/internal/planner"
	"github.com/mertcikla/diag/tld/internal/workspace"
)

func TestViewsCmd_OutputsDerivedViewSummary(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	seedViewsWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "views")
	if err != nil {
		t.Fatalf("views: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if want := "Views: 3 total (2 owned + root)"; !containsLine(stdout, want) {
		t.Fatalf("stdout missing summary %q:\n%s", want, stdout)
	}
	if want := "| root | Synthetic Root | 0 | 1 | 1 | 0 | root |"; !containsLine(stdout, want) {
		t.Fatalf("stdout missing root row %q:\n%s", want, stdout)
	}
	if want := "| platform | Platform | 1 | 2 | 1 | 1 | root/platform |"; !containsLine(stdout, want) {
		t.Fatalf("stdout missing platform row %q:\n%s", want, stdout)
	}
	if want := "| api | API | 2 | 1 | 0 | 0 | root/platform/api |"; !containsLine(stdout, want) {
		t.Fatalf("stdout missing api row %q:\n%s", want, stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestViewsCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	seedViewsWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "views", "--format", "json")
	if err != nil {
		t.Fatalf("views --format json: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var payload planner.JSONOutput
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal json output: %v\nstdout=%s", err, stdout)
	}
	if payload.Command != "views" || payload.Status != "ok" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Summary["total_views"] != 3 || payload.Summary["owned_views"] != 2 {
		t.Fatalf("unexpected summary: %+v", payload.Summary)
	}
	rawViews, ok := payload.Extra["views"].([]any)
	if !ok {
		t.Fatalf("expected extra.views array, got %#v", payload.Extra["views"])
	}
	if len(rawViews) != 3 {
		t.Fatalf("expected 3 views, got %d", len(rawViews))
	}
	rows := make(map[string]map[string]any, len(rawViews))
	for _, rawView := range rawViews {
		row, ok := rawView.(map[string]any)
		if !ok {
			t.Fatalf("expected object row, got %#v", rawView)
		}
		ref, _ := row["ref"].(string)
		rows[ref] = row
	}
	if rows["api"]["depth"].(float64) != 2 {
		t.Fatalf("expected api depth 2, got %#v", rows["api"])
	}
	if rows["platform"]["direct_elements"].(float64) != 2 {
		t.Fatalf("expected platform direct_elements 2, got %#v", rows["platform"])
	}
	if rows["root"]["synthetic"].(bool) != true {
		t.Fatalf("expected root row to be synthetic, got %#v", rows["root"])
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func containsLine(output, want string) bool {
	return slices.Contains(splitLines(output), want)
}

func splitLines(output string) []string {
	lines := make([]string, 0)
	start := 0
	for index, ch := range output {
		if ch != '\n' {
			continue
		}
		lines = append(lines, output[start:index])
		start = index + 1
	}
	if start < len(output) {
		lines = append(lines, output[start:])
	}
	return lines
}

func seedViewsWorkspace(t *testing.T, dir string) {
	t.Helper()
	connector := &workspace.Connector{View: "platform", Source: "api", Target: "db", Label: "reads"}
	ws := &workspace.Workspace{
		Dir: dir,
		Elements: map[string]*workspace.Element{
			"platform": {
				Name:    "Platform",
				Kind:    "workspace",
				HasView: true,
				Placements: []workspace.ViewPlacement{{
					ParentRef: "root",
				}},
			},
			"api": {
				Name:    "API",
				Kind:    "service",
				HasView: true,
				Placements: []workspace.ViewPlacement{{
					ParentRef: "platform",
				}},
			},
			"worker": {
				Name: "Worker",
				Kind: "service",
				Placements: []workspace.ViewPlacement{{
					ParentRef: "api",
				}},
			},
			"db": {
				Name: "DB",
				Kind: "database",
				Placements: []workspace.ViewPlacement{{
					ParentRef: "platform",
				}},
			},
		},
		Connectors: map[string]*workspace.Connector{
			workspace.ConnectorKey(connector): connector,
		},
	}
	if err := workspace.Save(ws); err != nil {
		t.Fatalf("save workspace: %v", err)
	}
}
