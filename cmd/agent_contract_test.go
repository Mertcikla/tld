package cmd_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestAgentMutationsConverge(t *testing.T) {
	for _, target := range []string{"local", "remote"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			if target == "remote" {
				cmd.SetupApplyWorkspace(t, dir, cmd.NewMockServer(t, &cmd.MockDiagramService{}))
			} else {
				cmd.MustInitWorkspace(t, dir)
			}
			cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--technology", "Go", "--description", "Handles traffic")
			cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform")
			cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform")
			cmd.MustRunCmd(t, dir, "add", "Public API", "--ref", "api", "--technology", "Rust")
			ws, err := workspace.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			runner, err := exec.NewRunner(ws.Config, "", "", false)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := runner.Close(); err != nil {
					t.Errorf("close runner: %v", err)
				}
			}()
			id := int32(ws.Meta.Elements["api"].ID)
			el, err := runner.GetElement(context.Background(), id)
			if err != nil {
				t.Fatal(err)
			}
			cached := ws.Elements["api"]
			if el.GetName() != "Public API" || cached.Name != el.GetName() || cached.Technology != "rust" || cached.Technology != el.GetTechnology() || cached.Kind != el.GetKind() || cached.Description != "Handles traffic" || len(cached.Placements) != 2 {
				t.Fatalf("target/cache diverged: target=%v cache=%+v", el, cached)
			}
			if _, _, err := cmd.RunCmd(t, dir, "add", "Wrong name", "--ref", "api", "--kind", "database"); err == nil {
				t.Fatal("expected kind conflict")
			}
			el, err = runner.GetElement(context.Background(), id)
			if err != nil || el.GetName() != "Public API" || el.GetKind() != "service" {
				t.Fatalf("rejected add mutated target: %v, %v", el, err)
			}
			cmd.MustRunCmd(t, dir, "add", "Database", "--ref", "db")
			cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--view", "root", "--label", "reads", "--direction", "both", "--style", "straight", "--tags", "storage", "--description", "Confirmed dependency")
			var result cmdutil.JSONOutput
			for range 2 {
				out, _ := cmd.MustRunCmd(t, dir, "connect", "--from", "Public API", "--to", "db", "--view", "root", "--label", "reads", "--format", "json")
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatal(err)
				}
				if result.Items[0].Ref != "root/api~db/reads" || result.Items[0].ID == 0 {
					t.Fatalf("unusable selector: %s", out)
				}
			}
			connectors, err := runner.ListConnectors(context.Background())
			if err != nil || len(connectors) != 1 {
				t.Fatalf("retry duplicated connectors: %v, %v", connectors, err)
			}
			ws, err = workspace.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			cachedConnector := ws.Connectors["root/api~db/reads"]
			if connectors[0].GetDirection() != "both" || cachedConnector.Direction != "both" || cachedConnector.Style != "straight" || cachedConnector.Description != "Confirmed dependency" || strings.Join(cachedConnector.Tags, ",") != "storage" {
				t.Fatalf("retry lost connector metadata: target=%v cache=%+v", connectors[0], cachedConnector)
			}
			for _, args := range [][]string{{"view", "unplace", "root", "api"}, {"view", "place", "platform", "api", "--from", "root", "--dry-run"}} {
				if _, _, err := cmd.RunCmd(t, dir, args...); err == nil {
					t.Fatalf("allowed placement removal with attached connectors: %v", args)
				}
			}
			out, _ := cmd.MustRunCmd(t, dir, "update", "connector", result.Items[0].Ref, "label", "queries", "--format", "json")
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			if result.Items[0].Ref != "root/api~db/queries" || result.Items[0].ID != connectors[0].GetId() {
				t.Fatalf("update returned stale identity: %s", out)
			}
			selector := strconv.Itoa(int(result.Items[0].ID))
			cmd.MustRunCmd(t, dir, "inspect", selector, "--type", "connector", "--format", "json")
			for _, suffix := range [][]string{{"--dry-run"}, nil} {
				out, _ := cmd.MustRunCmd(t, dir, append([]string{"remove", "connector", selector, "--format", "json"}, suffix...)...)
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatalf("invalid removal JSON: %v\n%s", err, out)
				}
				if result.Items[0].ID != connectors[0].GetId() {
					t.Fatalf("removal lost selector: %s", out)
				}
			}
			connectors, err = runner.ListConnectors(context.Background())
			if err != nil || len(connectors) != 0 {
				t.Fatalf("connector was not removed: %v, %v", connectors, err)
			}
			for range 2 {
				cmd.MustRunCmd(t, dir, "view", "place", "platform", "api", "--from", "root")
			}
			ws, err = workspace.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(ws.Elements["api"].Placements) != 1 || ws.Elements["api"].Placements[0].ParentRef != "platform" || ws.Elements["api"].Description != "Handles traffic" {
				t.Fatalf("move changed metadata or other placements: %+v", ws.Elements["api"])
			}
			for range 2 {
				cmd.MustRunCmd(t, dir, "view", "unplace", "platform", "api")
			}
			ws, err = workspace.Load(dir)
			if err != nil {
				t.Fatal(err)
			}
			if ws.Elements["api"] == nil || len(ws.Elements["api"].Placements) != 0 {
				t.Fatalf("unplace deleted resource or retained placement: %+v", ws.Elements["api"])
			}
			if _, err := runner.GetElement(context.Background(), id); err != nil {
				t.Fatalf("unplace deleted target element: %v", err)
			}
			if target == "local" {
				exported, err := runner.ExportWorkspace(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				for _, p := range exported.GetPlacements() {
					if p.GetElementId() == id {
						t.Fatalf("target retained removed placement: %v", p)
					}
				}
			}
		})
	}
}

func TestAgentJSONAndRecoveryContract(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api")
	cmd.MustRunCmd(t, dir, "add", "Database", "--ref", "db")
	cmd.MustRunCmd(t, dir, "connect", "--from", "api", "--to", "db", "--label", "reads")
	for _, args := range [][]string{
		{"add"}, {"nonesuch"}, {"add", "API", "--unknown"},
		{"connect", "--from", "api"}, {"connect", "--from", "missing", "--to", "db"},
		{"add", "Worker", "--parent", "missing"},
		{"connect", "--from", "api", "--to", "db", "--direction", "sideways"},
		{"connect", "--from", "api", "--to", "db", "--direction", "sideways", "--dry-run"},
		{"update", "connector", "root/api~db/reads", "direction", "sideways", "--dry-run"},
		{"validate", "--strictness", "4"}, {"validate", "--strict"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			out, stderr, err := cmd.RunCmd(t, dir, append(args, "--format", "json")...)
			if err == nil {
				t.Fatal("expected failure")
			}
			var response cmdutil.JSONOutput
			if parseErr := json.Unmarshal([]byte(out), &response); parseErr != nil {
				t.Fatalf("invalid JSON: %v\n%s", parseErr, out)
			}
			if response.Status != "error" || response.Error == nil || response.Error.Code == "" || response.Error.Message == "" || response.Error.Hint == "" {
				t.Fatalf("missing recovery details: %s", out)
			}
			if stderr != "" {
				t.Fatalf("duplicate error output: %s", stderr)
			}
		})
	}
	for _, args := range [][]string{{"init", dir}, {"validate"}, {"validate", "rules"}, {"validate", "ARC205"}, {"render", "root"}, {"render", "root", "--to", "mermaid"}} {
		out, _ := cmd.MustRunCmd(t, dir, append(args, "--format", "json")...)
		if !json.Valid([]byte(out)) {
			t.Fatalf("invalid success JSON for %v: %s", args, out)
		}
	}
	if _, _, err := cmd.RunCmd(t, dir, "list", "elements", "--format", "garbage"); err == nil {
		t.Fatal("invalid output format accepted")
	}
	_, _, err := cmd.RunCmd(t, dir, "add")
	if err == nil || !strings.Contains(err.Error(), "tld add <name>") {
		t.Fatalf("syntax error lacks invocation: %v", err)
	}
}

func TestAgentReportsCommittedTargetWhenCacheFails(t *testing.T) {
	dir := t.TempDir()
	svc := &cmd.MockDiagramService{}
	cmd.SetupApplyWorkspace(t, dir, cmd.NewMockServer(t, svc))
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api")
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	id := int32(ws.Meta.Elements["api"].ID)
	svc.UpdateElementFunc = func(req *diagv1.UpdateElementRequest) (*diagv1.UpdateElementResponse, error) {
		el := svc.Element(req.GetElementId())
		el.Description = req.Description
		// Make the cache unwritable only after the target accepted the change.
		cache := filepath.Join(ws.Dir, "elements.yaml")
		if err := os.Remove(cache); err != nil {
			return nil, err
		}
		if err := os.Mkdir(cache, 0700); err != nil {
			return nil, err
		}
		return &diagv1.UpdateElementResponse{Element: el}, nil
	}
	out, stderr, err := cmd.RunCmd(t, dir, "update", "element", "api", "description", "Changed on target", "--format", "json")
	if err == nil || stderr != "" {
		t.Fatalf("expected a single JSON failure: err=%v stderr=%s", err, stderr)
	}
	var response cmdutil.JSONOutput
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatal(err)
	}
	if response.Error == nil || !response.Error.Applied || response.Error.Code != "cache_update_failed" || !strings.Contains(response.Error.Hint, "tld pull") {
		t.Fatalf("missing partial success recovery: %s", out)
	}
	if svc.Element(id).GetDescription() != "Changed on target" {
		t.Fatal("test did not commit the target change")
	}
}

func TestAgentImportRejectsUnknownFieldsBeforeWriting(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	for _, document := range []string{
		"elements:\n  api:\n    name: API\n    descriptionn: typo\n",
		"elements:\n  api:\n    name: API\n    placements:\n      - parent: root\n        postion_x: 50\n",
		"elements:\n  api:\n    name: API\n---\nelements:\n  db:\n    name: DB\n",
	} {
		file := filepath.Join(t.TempDir(), "import.yaml")
		if err := os.WriteFile(file, []byte(document), 0600); err != nil {
			t.Fatal(err)
		}
		for _, suffix := range [][]string{{"--dry-run"}, nil} {
			if _, _, err := cmd.RunCmd(t, dir, append([]string{"import", file}, suffix...)...); err == nil {
				t.Fatalf("invalid document accepted: %s", document)
			}
		}
	}
	ws, err := workspace.Load(dir)
	if err != nil || len(ws.Elements) != 0 {
		t.Fatalf("rejected import changed workspace: %v, %v", ws, err)
	}
}
