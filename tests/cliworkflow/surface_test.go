//go:build cliworkflow

package cliworkflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func (h *harness) updateConnector(key, selector, field, value string) string {
	h.command(false, "update", "connector", selector, field, value)
	c := h.local.Connectors[key]
	switch field {
	case "view":
		c.View = value
	case "source":
		c.Source = value
	case "target":
		c.Target = value
	case "label":
		c.Label = value
	case "description":
		c.Description = value
	case "relationship":
		c.Relationship = value
	case "direction":
		c.Direction = value
	case "style":
		c.Style = value
	case "url":
		c.URL = value
	case "source_handle":
		c.SourceHandle = value
	case "target_handle":
		c.TargetHandle = value
	case "tags":
		c.Tags = strings.Split(value, ",")
		if value == "" {
			c.Tags = nil
		}
	case "visibility_delta":
		c.VisibilityDelta, _ = strconv.Atoi(value)
	default:
		h.fail("unmodeled connector field %s", field)
	}
	id := h.ids.Connectors[key]
	delete(h.ids.Connectors, key)
	h.ids.Connectors[c.key()] = id
	delete(h.local.Connectors, key)
	h.local.Connectors[c.key()] = c
	if field == "visibility_delta" {
		h.acceptBaseline()
		h.check() // Workspace-only until sync applies it.
	} else {
		h.committed()
	}
	return c.key()
}

func (h *harness) viewField(ref, command, value string) {
	h.command(false, "view", command, ref, value)
	e := h.local.Elements[ref]
	if command == "rename" {
		e.ViewName = value
	} else {
		e.ViewLabel = value
	}
	h.local.Elements[ref] = e
	h.committed()
}

func (h *harness) placeEmpty(ref, destination, from string) {
	args := []string{"view", "place", destination, ref}
	if from != "" {
		args = append(args, "--from", from)
	}
	h.command(false, args...)
	e := h.local.Elements[ref]
	ps := []placement{}
	for _, p := range e.Placements {
		if p.Parent != from && p.Parent != destination {
			ps = append(ps, p)
		}
	}
	e.Placements = append(ps, placement{Parent: destination})
	h.local.Elements[ref] = e
	h.committed()
}

func (h *harness) importNew(ref string, stdin bool) {
	doc := fmt.Sprintf("elements:\n  %s:\n    name: %s\n    kind: service\n    description: imported\n    placements:\n      - parent: root\n        position_x: 12\n        position_y: 34\n", ref, ref)
	if stdin {
		h.commandInput(false, doc, "import", "-")
	} else {
		path := filepath.Join(h.dir, "batch.yaml")
		if err := os.WriteFile(path, []byte(doc), 0600); err != nil {
			h.fail("write import: %v", err)
		}
		h.unchanged(false, "import", path, "--dry-run")
		h.command(false, "import", path)
	}
	e := h.local.Elements[ref]
	e.Name = ref
	e.Kind = "service"
	e.Description = "imported"
	found := false
	for i, p := range e.Placements {
		if p.Parent == "root" {
			e.Placements[i] = placement{"root", 12, 34}
			found = true
		}
	}
	if !found {
		e.Placements = append(e.Placements, placement{"root", 12, 34})
	}
	h.local.Elements[ref] = e
	h.learnID("element", ref)
	h.committed()
}

func (h *harness) observeText(needle string, args ...string) {
	before := h.fingerprint()
	out, stderr, err := h.runRaw(h.dir, "text", "", args...)
	if err != nil || !strings.Contains(out, needle) {
		h.fail("unexpected text result for %q: error=%v stdout=%s stderr=%s", args, err, out, stderr)
	}
	if h.fingerprint() != before {
		h.fail("text read changed workspace: %q", args)
	}
	h.check()
}

func (h *harness) inventory() {
	for _, source := range []string{"cache", "live"} {
		got := h.unchanged(false, "list", "elements", "--source", source)
		extra := got["extra"].(map[string]any)
		rows, _ := extra["elements"].([]any)
		expected := h.local
		if source == "live" {
			expected = h.target
		}
		if len(rows) != len(expected.Elements) {
			h.fail("inventory count mismatch for %s", source)
		}
		for _, row := range rows {
			r := row.(map[string]any)
			e, ok := expected.Elements[r["ref"].(string)]
			if !ok || r["name"] != e.Name || int32(r["id"].(float64)) != h.ids.Elements[r["ref"].(string)] {
				h.fail("inventory content mismatch: %s", asJSON(r))
			}
		}
	}
	for _, kind := range []string{"connectors", "views"} {
		result := h.unchanged(false, "list", kind, "--source", "live")
		rows, _ := result["extra"].(map[string]any)[kind].([]any)
		if kind == "connectors" {
			if len(rows) != len(h.target.Connectors) {
				h.fail("connector inventory count mismatch")
			}
			for _, row := range rows {
				r := row.(map[string]any)
				key := r["key"].(string)
				want, ok := h.target.Connectors[key]
				if !ok || r["source"] != want.Source || r["target"] != want.Target || int32(r["id"].(float64)) != h.ids.Connectors[key] {
					h.fail("connector inventory content mismatch: %s", asJSON(r))
				}
			}
		} else {
			if len(rows) != len(h.ids.Views)+1 {
				h.fail("view inventory count mismatch: %s", asJSON(result))
			}
			for _, row := range rows {
				r := row.(map[string]any)
				ref := r["ref"].(string)
				if ref != "root" {
					e, ok := h.target.Elements[ref]
					if !ok || !e.HasView || int32(r["id"].(float64)) != h.ids.Views[ref] {
						h.fail("view inventory content mismatch: %s", asJSON(r))
					}
				}
			}
		}
	}
}

func TestLifecycleFullSurface(t *testing.T) {
	for _, backend := range []string{"sqlite", "remote", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			if backend == "postgres" && os.Getenv("TLD_TEST_POSTGRES_URL") == "" {
				t.Skip("set TLD_TEST_POSTGRES_URL for isolated PostgreSQL databases")
			}
			h := newHarnessFor(t, backend)
			h.add("platform", "root", 0, 0)
			h.createView("platform")
			h.add("other", "root", 600, 0)
			h.createView("other")
			h.add("api", "platform", 100, 100)
			h.add("db", "platform", 400, 100)
			h.add("cache", "root", 800, 100)
			for _, args := range [][]string{
				{"add", "preview", "--ref", "preview", "--dry-run"},
				{"update", "element", "api", "description", "preview", "--dry-run"},
				{"view", "create", "cache", "--dry-run"},
				{"view", "rename", "platform", "Preview", "--dry-run"},
				{"view", "set-level", "platform", "Container", "--dry-run"},
				{"view", "delete", "other", "--dry-run"},
			} {
				h.unchanged(false, args...)
			}
			h.importNew("batch", false)
			h.importNew("batch", true)
			h.update("batch", "logo_url", "https://example.com/custom.svg")
			h.update("batch", "description", "keeps a custom logo without technology links")
			h.updateAs("api", strconv.Itoa(int(h.ids.Elements["api"])), "description", "selected by ID")
			h.updateAs("api", "api", "name", "Public API")
			h.updateAs("api", "Public API", "description", "selected by name")
			out, stderr, err := h.runRaw(h.dir, "text", "", "update", "element", "api", "description", "updated in text mode")
			if err != nil || !strings.Contains(out, "updated") {
				h.fail("text mutation failed: %v %s %s", err, out, stderr)
			}
			e := h.local.Elements["api"]
			e.Description = "updated in text mode"
			h.local.Elements["api"] = e
			h.committed()
			h.command(false, "update", "element", "api", "tags", "reviewed", "--append")
			e = h.local.Elements["api"]
			e.Tags = append(e.Tags, "reviewed")
			h.local.Elements["api"] = e
			h.committed()
			h.command(false, "update", "element", "api", "tags", "critical", "--remove")
			e = h.local.Elements["api"]
			e.Tags = []string{"backend", "reviewed"}
			h.local.Elements["api"] = e
			h.committed()
			for _, change := range [][2]string{{"kind", "component"}, {"url", "https://example.com/runbook"}, {"logo_url", "https://example.com/icon.svg"}, {"repo", "example/project"}, {"repository_id", "repo-1"}, {"branch", "main"}, {"file_path", "src/api.go"}, {"language", "go"}, {"view_name", "API details"}, {"view_label", "Component"}} {
				if change[0] == "view_name" {
					h.createView("api")
				}
				h.update("api", change[0], change[1])
			}
			h.viewField("api", "rename", "Implementation")
			h.viewField("api", "set-level", "Code")
			h.placeEmpty("cache", "other", "")
			h.placeEmpty("batch", "api", "root")
			h.unchanged(false, "view", "place", "other", "cache", "--dry-run")
			key := h.connect("platform", "api", "db", "reads")
			h.unchanged(false, "update", "connector", key, "label", "preview", "--dry-run")
			for _, change := range [][2]string{{"description", "changed"}, {"relationship", "SQL"}, {"direction", "backward"}, {"style", "step"}, {"url", "https://example.com/edge"}, {"source_handle", "right"}, {"target_handle", "left"}, {"tags", "runtime,storage"}, {"source", "cache"}, {"target", "api"}, {"view", "other"}, {"label", "calls"}} {
				key = h.updateConnector(key, strconv.Itoa(int(h.ids.Connectors[key])), change[0], change[1])
			}
			key = h.connect("other", "cache", "api", "calls") // Retry keeps omitted edge metadata.
			key = h.updateConnector(key, key, "visibility_delta", "2")
			h.pull(true) // Workspace visibility preferences survive force-pull.
			h.sync()
			h.inventory()
			for _, kind := range []string{"element", "view"} {
				result := h.unchanged(false, "inspect", "api", "--type", kind, "--source", "live")
				if result["ref"] != "api" || result["type"] != kind {
					h.fail("inspect selected wrong resource: %s", asJSON(result))
				}
			}
			h.unchanged(false, "inspect", strconv.Itoa(int(h.ids.Connectors[key])), "--type", "connector", "--source", "live")
			h.observeText("Public API", "list", "elements")
			h.observeText("flowchart", "render", "platform", "--source", "live")
			h.unchanged(false, "list", "elements", "--compact=false")
			output := filepath.Join(h.dir, "diagram.mmd")
			h.observeText("wrote", "render", "platform", "-o", output)
			saved, err := os.ReadFile(output)
			if err != nil || !strings.Contains(string(saved), "flowchart") {
				h.fail("render did not write Mermaid output: %v", err)
			}
			h.unchanged(true, "render", "missing-view", "-o", output)
			after, err := os.ReadFile(output)
			if err != nil || string(after) != string(saved) {
				h.fail("rejected render damaged existing output")
			}
			h.observeText("Workspace valid", "validate", "--source", "live")
			h.unchanged(false, "validate", "--source", "live")
			h.unchanged(true, "validate", "--source", "live", "--strict")
			for _, field := range []string{"url", "logo_url", "repo", "repository_id", "branch", "file_path"} {
				h.update("api", field, "")
			}
			for _, field := range []string{"description", "relationship", "url", "source_handle", "target_handle"} {
				key = h.updateConnector(key, key, field, "")
			}
			h.pull(false)
			h.sync()
			h.pull(false)
			h.add("retired", "root", 0, 0)
			h.createView("retired")
			h.remove("retired")
			h.pull(false)
		})
	}
}

func TestLifecycleImportGraphPatches(t *testing.T) {
	for _, backend := range []string{"sqlite", "remote", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			if backend == "postgres" && os.Getenv("TLD_TEST_POSTGRES_URL") == "" {
				t.Skip("set TLD_TEST_POSTGRES_URL")
			}
			h := newHarnessFor(t, backend)
			h.add("keep", "root", 900, 100)
			doc := `elements:
  platform:
    name: platform
    kind: service
    has_view: true
    placements: [{parent: root, position_x: 0, position_y: 0}]
  api:
    name: api
    kind: service
    description: imported API
    placements: [{parent: platform, position_x: 120, position_y: 80}]
  db:
    name: db
    kind: database
    placements: [{parent: platform, position_x: 400, position_y: 80}]
connectors:
  - {view: platform, source: api, target: db, label: reads, direction: both}
`
			h.commandInput(false, doc, "import", "-")
			h.local.Elements["platform"] = element{Name: "platform", Kind: "service", HasView: true, Placements: []placement{{"root", 0, 0}}}
			h.local.Elements["api"] = element{Name: "api", Kind: "service", Description: "imported API", Placements: []placement{{"platform", 120, 80}}}
			h.local.Elements["db"] = element{Name: "db", Kind: "database", Placements: []placement{{"platform", 400, 80}}}
			c := connector{View: "platform", Source: "api", Target: "db", Label: "reads", Direction: "both", Style: "bezier"}
			h.local.Connectors[c.key()] = c
			for _, ref := range []string{"platform", "api", "db"} {
				h.learnID("element", ref)
			}
			h.learnID("view", "platform")
			h.learnID("connector", c.key())
			h.committed()
			h.update("api", "tags", "backend,imported")
			h.update("api", "url", "https://example.com/api")
			// A JSON patch omits existing placements, tags, URL, connector and keep.
			patch := `{"elements":{"api":{"name":"api","description":"patched through JSON"}}}`
			for i := 0; i < 2; i++ {
				h.commandInput(false, patch, "import", "-")
				e := h.local.Elements["api"]
				e.Description = "patched through JSON"
				h.local.Elements["api"] = e
				h.committed()
			}
			h.inventory()
			h.sync()
			h.pull(false)
		})
	}
}

func TestLifecycleLinksAndInputErrors(t *testing.T) {
	h := newHarness(t)
	h.add("api", "root", 0, 0)
	h.add("child", "api", 100, 100) // Implicitly creates api's owned view.
	h.command(false, "link", "api", "src/api.go")
	e := h.local.Elements["api"]
	e.FilePath = "src/api.go"
	h.local.Elements["api"] = e
	h.committed()
	h.command(false, "link", "api", "--unlink")
	e = h.local.Elements["api"]
	e.FilePath = ""
	h.local.Elements["api"] = e
	h.committed()
	h.reject("unknown_field", "update", "element", "api", "unknown", "bad")
	h.add("second", "root", 400, 0)
	h.update("second", "name", "Shared name")
	h.update("api", "name", "Shared name")
	h.unchanged(true, "update", "element", "Shared name", "description", "ambiguous")
	h.unchanged(true, "connect", "--from", "api", "--to", "missing")
	h.unchanged(true, "update", "element", "api", "ref", "Bad Ref")
	before := h.fingerprint()
	result := h.commandInput(true, "elements: [broken", "import", "-")
	if result["error"].(map[string]any)["code"] != "invalid_document" || h.fingerprint() != before {
		h.fail("invalid import changed state")
	}
	h.check()
	path := filepath.Join(h.workspace, "elements.yaml")
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("api: [broken"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"sync"}, {"pull"}, {"add", "unexpected"}} {
		before = h.fingerprint()
		h.command(true, args...)
		if h.fingerprint() != before {
			h.fail("malformed workspace changed on rejected %q", args)
		}
	}
	if err := os.WriteFile(path, saved, 0600); err != nil {
		t.Fatal(err)
	}
	h.check()
	h.sync()
	h.pull(false)
}
