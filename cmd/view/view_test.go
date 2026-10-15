package view_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestPlacePreservesTargetCoordinates(t *testing.T) {
	for _, scenario := range []string{"stale coordinates", "missing cached placement", "move to existing placement"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			cmd.MustInitWorkspace(t, dir)
			cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--position-x", "10", "--position-y", "20")
			if scenario == "move to existing placement" {
				cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform")
				cmd.MustRunCmd(t, dir, "view", "create", "platform")
				cmd.MustRunCmd(t, dir, "view", "place", "platform", "api")
			}
			ws, err := workspace.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			runner, err := exec.NewRunner(ws.Config, "local", "", false)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := runner.Close(); err != nil {
					t.Error(err)
				}
			}()
			ctx := context.Background()
			rootID, err := exec.RootViewID(ctx, runner)
			if err != nil {
				t.Fatal(err)
			}
			elementID := int32(ws.Meta.Elements["api"].ID)
			if scenario == "missing cached placement" {
				cmd.MustRunCmd(t, dir, "view", "unplace", "root", "api")
				if err := runner.AddPlacement(ctx, rootID, elementID, 800, 900); err != nil {
					t.Fatal(err)
				}
			} else if err := runner.UpdatePlacement(ctx, rootID, elementID, 800, 900); err != nil {
				t.Fatal(err)
			}
			args := []string{"view", "place", "root", "api"}
			if scenario == "move to existing placement" {
				args = append(args, "--from", "platform")
			}
			cmd.MustRunCmd(t, dir, args...)
			ws, err = workspace.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			placements := ws.Elements["api"].Placements
			if len(placements) != 1 || placements[0].ParentRef != "root" || placements[0].PositionX != 800 || placements[0].PositionY != 900 {
				t.Fatalf("target placement was not preserved: %+v", placements)
			}
		})
	}
}

func TestUnplaceFinalPlacementStaysOutOfRoot(t *testing.T) {
	for _, parent := range []string{"root", "platform"} {
		t.Run(parent, func(t *testing.T) {
			dir := t.TempDir()
			cmd.MustInitWorkspace(t, dir)
			if parent != "root" {
				cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", parent)
			}
			cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "class", "--parent", parent)
			cmd.MustRunCmd(t, dir, "view", "create", "api")
			cmd.MustRunCmd(t, dir, "view", "unplace", parent, "api")
			cmd.MustRunCmd(t, dir, "validate", "ARC007", "--strict", "--source", "live")
			for _, source := range []string{"cache", "live"} {
				out, _ := cmd.MustRunCmd(t, dir, "render", "root", "--source", source)
				if strings.Contains(out, "n_api[") {
					t.Fatalf("%s render resurrected unplaced element:\n%s", source, out)
				}
				out, _ = cmd.MustRunCmd(t, dir, "list", "views", "--source", source, "--format", "json")
				var result struct {
					Extra struct {
						Views []struct {
							Ref  string `json:"ref"`
							Path string `json:"path"`
						} `json:"views"`
					} `json:"extra"`
				}
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatal(err)
				}
				for _, view := range result.Extra.Views {
					if view.Ref == "api" && view.Path != "unreachable" {
						t.Fatalf("%s listing attached unplaced owner to root: %s", source, out)
					}
				}
			}
			cmd.MustRunCmd(t, dir, "view", "place", "root", "api")
			out, _ := cmd.MustRunCmd(t, dir, "render", "root", "--source", "live")
			if !strings.Contains(out, "n_api[") {
				t.Fatalf("explicitly placed element missing:\n%s", out)
			}
		})
	}
}

func TestViewCmdCreateRenameSetLevelDelete(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "view", "create", "api", "--name", "API Diagram", "--label", "Container")
	el := loadElement(t, dir, "api")
	if !el.HasView {
		t.Fatalf("api has_view = false after view create")
	}
	if el.ViewName != "API Diagram" || el.ViewLabel != "Container" {
		t.Fatalf("view fields = (%q, %q), want (API Diagram, Container)", el.ViewName, el.ViewLabel)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Meta == nil || ws.Meta.Views["api"] == nil {
		t.Fatalf("view metadata for api missing: %+v", ws.Meta)
	}

	cmd.MustRunCmd(t, dir, "view", "rename", "api", "API v2")
	if got := loadElement(t, dir, "api").ViewName; got != "API v2" {
		t.Fatalf("view_name = %q, want API v2", got)
	}

	cmd.MustRunCmd(t, dir, "view", "set-level", "api", "Component")
	if got := loadElement(t, dir, "api").ViewLabel; got != "Component" {
		t.Fatalf("view_label = %q, want Component", got)
	}

	cmd.MustRunCmd(t, dir, "view", "delete", "api")
	el = loadElement(t, dir, "api")
	if el.HasView {
		t.Fatalf("api has_view = true after view delete")
	}
	if el.ViewName != "" || el.ViewLabel != "" {
		t.Fatalf("view fields not cleared: (%q, %q)", el.ViewName, el.ViewLabel)
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Meta != nil && ws.Meta.Views["api"] != nil {
		t.Fatalf("view metadata for api still present after delete")
	}
}

func TestViewCmdCreateDryRunDoesNotMutate(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	stdout, _, err := cmd.RunCmd(t, dir, "view", "create", "api", "--dry-run")
	if err != nil {
		t.Fatalf("view create --dry-run: %v", err)
	}
	if !strings.Contains(stdout, "dry-run") {
		t.Fatalf("stdout = %q, want dry-run confirmation", stdout)
	}
	if loadElement(t, dir, "api").HasView {
		t.Fatalf("dry-run created a view")
	}
}

func TestViewCmdCreateRequiresElement(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "view", "create", "ghost")
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want element-not-found error", err)
	}
}

func TestViewCmdDeleteRefusesNonEmptyView(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "view", "delete", "platform")
	if err == nil || !strings.Contains(err.Error(), "still has") {
		t.Fatalf("err = %v, want non-empty view refusal", err)
	}
}

func loadElement(t *testing.T, dir, ref string) *workspace.Element {
	t.Helper()
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	el := ws.Elements[ref]
	if el == nil {
		t.Fatalf("element %q missing", ref)
	}
	return el
}
