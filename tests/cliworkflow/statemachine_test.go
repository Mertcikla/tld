//go:build cliworkflow

package cliworkflow

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"pgregory.net/rapid"
)

func init() {
	// Real subprocesses + migrations make each attempt substantially more
	// expensive than a unit property. CLI flags and RAPID_* still override these
	// package-local defaults; nightly runs can raise both limits.
	for name, value := range map[string]string{"checks": "10", "steps": "20"} {
		if os.Getenv("RAPID_"+strings.ToUpper(name)) == "" {
			if err := flag.Set("rapid."+name, value); err != nil {
				panic(err)
			}
		}
	}
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestWorkspaceStateMachine(t *testing.T) {
	runWorkspaceStateMachine(t, "sqlite")
}

func TestWorkspaceStateMachineRemote(t *testing.T) { runWorkspaceStateMachine(t, "remote") }
func TestWorkspaceStateMachinePostgres(t *testing.T) {
	if os.Getenv("TLD_TEST_POSTGRES_URL") == "" {
		t.Skip("set TLD_TEST_POSTGRES_URL")
	}
	runWorkspaceStateMachine(t, "postgres")
}

func runWorkspaceStateMachine(t *testing.T, backend string) {
	rapid.Check(t, func(t *rapid.T) {
		h := newHarnessFor(t, backend)
		h.add("platform", "root", 0, 0)
		h.createView("platform")
		h.add("api", "platform", 100, 100)
		h.add("db", "platform", 400, 100)
		next := 0
		clean := func() {
			if h.dirty || h.local.canonical() != h.target.canonical() {
				t.Skip("reconcile pending local or external edits first")
			}
		}
		pick := func() string { return rapid.SampledFrom(sortedKeys(h.local.Elements)).Draw(t, "element") }
		pickConnector := func() string {
			if len(h.local.Connectors) == 0 {
				t.Skip("no connectors")
			}
			return rapid.SampledFrom(sortedKeys(h.local.Connectors)).Draw(t, "connector")
		}
		t.Repeat(map[string]func(*rapid.T){
			"": func(*rapid.T) { h.check() },
			"add": func(t *rapid.T) {
				clean()
				next++
				parent := rapid.SampledFrom([]string{"root", "platform"}).Draw(t, "parent")
				h.add(fmt.Sprintf("worker-%d", next), parent, float64(rapid.IntRange(-500, 500).Draw(t, "x")), float64(rapid.IntRange(-500, 500).Draw(t, "y")))
			},
			"update": func(t *rapid.T) {
				clean()
				ref := pick()
				fields := [][2]string{{"description", "reviewed"}, {"kind", "component"}, {"technology", "go"}, {"tags", "backend,reviewed"}, {"url", "https://example.com/api"}, {"logo_url", "https://example.com/logo.svg"}, {"repo", "example/project"}, {"repository_id", "repo-generated"}, {"branch", "main"}, {"language", "go"}, {"file_path", "src/api.go"}}
				change := rapid.SampledFrom(fields).Draw(t, "field")
				if change[0] == "description" {
					change[1] = rapid.SampledFrom([]string{"", "reviewed", "changed twice"}).Draw(t, "description")
				}
				h.update(ref, change[0], change[1])
			},
			"import": func(t *rapid.T) {
				clean()
				next++
				h.importNew(fmt.Sprintf("imported-%d", next), rapid.Bool().Draw(t, "stdin"))
			},
			"inventory": func(*rapid.T) { h.inventory() },
			"render":    func(*rapid.T) { h.observeText("flowchart", "render", "root", "--source", "live") },
			"view_create": func(*rapid.T) {
				clean()
				ref := pick()
				if h.local.Elements[ref].HasView {
					t.Skip("already owns a view")
				}
				h.createView(ref)
			},
			"view_delete": func(*rapid.T) {
				clean()
				ref := pick()
				if ref == "platform" || !h.local.Elements[ref].HasView {
					t.Skip("keep anchor or no view")
				}
				for _, e := range h.local.Elements {
					for _, p := range e.Placements {
						if p.Parent == ref {
							t.Skip("view has placements")
						}
					}
				}
				for _, c := range h.local.Connectors {
					if c.View == ref {
						t.Skip("view has connectors")
					}
				}
				h.deleteView(ref)
			},
			"view_fields": func(t *rapid.T) {
				clean()
				ref := pick()
				if !h.local.Elements[ref].HasView {
					t.Skip("no owned view")
				}
				if rapid.Bool().Draw(t, "name") {
					h.viewField(ref, "rename", "Details")
				} else {
					h.viewField(ref, "set-level", "Container")
				}
			},
			"place_or_move": func(t *rapid.T) {
				clean()
				ref := pick()
				view := rapid.SampledFrom(sortedKeys(h.ids.Views)).Draw(t, "destination")
				if view == ref {
					t.Skip("self view")
				}
				for _, e := range h.local.Elements {
					for _, p := range e.Placements {
						if p.Parent == view {
							t.Skip("destination not empty")
						}
					}
				}
				from := ""
				placements := h.local.Elements[ref].Placements
				if len(placements) > 0 && rapid.Bool().Draw(t, "move") {
					from = rapid.SampledFrom(placements).Draw(t, "old placement").Parent
					for _, c := range h.local.Connectors {
						if c.View == from && (c.Source == ref || c.Target == ref) {
							t.Skip("connected source placement")
						}
					}
				}
				h.placeEmpty(ref, view, from)
			},
			"manual_yaml": func(t *rapid.T) {
				ref := pick()
				if len(h.local.Elements[ref].Placements) == 0 {
					t.Skip("no placement to edit")
				}
				next++
				h.manual(ref, fmt.Sprintf("pending-%d-%d", next, rapid.IntRange(0, 10000).Draw(t, "edit")), float64(rapid.IntRange(-800, 800).Draw(t, "x")), float64(rapid.IntRange(-800, 800).Draw(t, "y")))
			},
			"external_edit": func(t *rapid.T) {
				next++
				h.externalDescription(pick(), fmt.Sprintf("external-%d", next))
			},
			"sync":         func(*rapid.T) { h.sync() },
			"sync_dry_run": func(*rapid.T) { h.unchanged(false, "sync", "--dry-run") },
			"pull": func(*rapid.T) {
				if h.dirty {
					h.reject("local_changes", "pull")
				} else {
					h.pull(false)
				}
			},
			"pull_force":   func(*rapid.T) { h.pull(true) },
			"pull_dry_run": func(*rapid.T) { h.unchanged(false, "pull", "--dry-run") },
			"read":         func(*rapid.T) { h.unchanged(false, "list", "elements", "--source", "live") },
			"connect": func(t *rapid.T) {
				clean()
				refs := sortedKeys(h.local.Elements)
				source := rapid.SampledFrom(refs).Draw(t, "source")
				target := rapid.SampledFrom(refs).Draw(t, "target")
				if source == target {
					t.Skip("choose distinct endpoints")
				}
				h.connect("platform", source, target, "calls")
			},
			"relabel": func(t *rapid.T) {
				clean()
				key := pickConnector()
				c := h.local.Connectors[key]
				label := "calls"
				if c.Label == label {
					label = "queries"
				}
				candidate := c
				candidate.Label = label
				if _, exists := h.local.Connectors[candidate.key()]; exists {
					t.Skip("label already exists")
				}
				h.relabel(key, label)
			},
			"connector_fields": func(t *rapid.T) {
				clean()
				key := pickConnector()
				change := rapid.SampledFrom([][2]string{{"description", "reviewed edge"}, {"relationship", "HTTP"}, {"direction", "none"}, {"style", "smoothstep"}, {"url", "https://example.com/edge"}, {"source_handle", "right"}, {"target_handle", "left"}, {"tags", "runtime,reviewed"}, {"visibility_delta", "1"}}).Draw(t, "connector field")
				h.updateConnector(key, key, change[0], change[1])
			},
			"disconnect": func(*rapid.T) { clean(); h.disconnect(pickConnector()) },
			"remove": func(*rapid.T) {
				clean()
				ref := pick()
				if ref == "platform" {
					t.Skip("keep the anchor view")
				}
				for _, c := range h.local.Connectors {
					if c.Source == ref || c.Target == ref || c.View == ref {
						t.Skip("remove connected resources separately")
					}
				}
				for _, e := range h.local.Elements {
					for _, p := range e.Placements {
						if p.Parent == ref {
							h.unchanged(true, "remove", "element", ref)
							return
						}
					}
				}
				h.remove(ref)
			},
			"share": func(t *rapid.T) {
				clean()
				ref := pick()
				if ref == "platform" {
					t.Skip("cannot place an element in its own view")
				}
				parent := rapid.SampledFrom([]string{"root", "platform"}).Draw(t, "parent")
				// Sharing endpoints can copy their relationships. Exercise placement
				// churn on unconnected elements; scenarios cover shared connectors.
				for _, c := range h.local.Connectors {
					if c.Source == ref || c.Target == ref {
						t.Skip("connected element")
					}
				}
				h.share(ref, parent, float64(rapid.IntRange(-300, 300).Draw(t, "x")), float64(rapid.IntRange(-300, 300).Draw(t, "y")))
			},
			"unplace": func(t *rapid.T) {
				clean()
				ref := pick()
				placements := h.local.Elements[ref].Placements
				if len(placements) == 0 {
					t.Skip("no placements")
				}
				parent := rapid.SampledFrom(placements).Draw(t, "placement").Parent
				for _, c := range h.local.Connectors {
					if c.View == parent && (c.Source == ref || c.Target == ref) {
						t.Skip("connected placement")
					}
				}
				h.unplace(ref, parent)
			},
			"rename": func(*rapid.T) {
				ref := pick()
				if ref == "platform" {
					t.Skip("keep the anchor alias")
				}
				next++
				h.rename(ref, fmt.Sprintf("renamed-%d", next))
			},
			"rejected_write": func(*rapid.T) {
				if !h.dirty {
					t.Skip("requires unapplied edits")
				}
				h.reject("cache_out_of_sync", "update", "element", "platform", "description", "blocked")
			},
		})
		// Every generated history must also converge and survive a second pull.
		h.sync()
		h.pull(false)
		h.pull(false)
	})
}
