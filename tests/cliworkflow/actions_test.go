//go:build cliworkflow

package cliworkflow

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

func (h *harness) committed() {
	h.target = h.local.clone()
	h.dirty = false
	h.acceptBaseline()
	h.check()
}

func (h *harness) add(ref, parent string, x, y float64) {
	h.command(false, "add", ref, "--ref", ref, "--kind", "service", "--description", "description of "+ref, "--technology", "go", "--tags", "backend,critical", "--parent", parent, "--position-x", number(x), "--position-y", number(y))
	h.local.Elements[ref] = element{Name: ref, Kind: "service", Description: "description of " + ref, Technology: "go", LogoURL: "/icons/go.svg", Tags: []string{"backend", "critical"}, Placements: []placement{{Parent: parent, X: x, Y: y}}}
	h.learnID("element", ref)
	if parent != "root" && !h.local.Elements[parent].HasView {
		e := h.local.Elements[parent]
		e.HasView = true
		h.local.Elements[parent] = e
		h.learnID("view", parent)
	}
	h.committed()
}

func (h *harness) createView(ref string) {
	h.command(false, "view", "create", ref)
	el := h.local.Elements[ref]
	el.HasView = true
	h.local.Elements[ref] = el
	h.learnID("view", ref)
	h.committed()
}

func (h *harness) deleteView(ref string) {
	h.command(false, "view", "delete", ref)
	el := h.local.Elements[ref]
	el.HasView, el.ViewName, el.ViewLabel = false, "", ""
	h.local.Elements[ref] = el
	delete(h.ids.Views, ref)
	h.committed()
}

func (h *harness) update(ref, field, value string) {
	h.updateAs(ref, ref, field, value)
}

func (h *harness) updateAs(ref, selector, field, value string) {
	h.command(false, "update", "element", selector, field, value)
	el := h.local.Elements[ref]
	switch field {
	case "description":
		el.Description = value
	case "technology":
		el.Technology = value
	case "name":
		el.Name = value
		if el.HasView && el.ViewName == "" {
			el.ViewName = h.local.Elements[ref].Name
		}
	case "kind":
		el.Kind = value
	case "url":
		el.URL = value
	case "logo_url":
		el.LogoURL = value
	case "repo":
		el.Repo = value
	case "repository_id":
		el.RepositoryID = value
	case "branch":
		el.Branch = value
	case "file_path":
		el.FilePath = value
	case "language":
		el.Language = value
	case "view_name":
		el.ViewName = value
	case "view_label":
		el.ViewLabel = value
	case "tags":
		el.Tags = strings.Split(value, ",")
		if value == "" {
			el.Tags = nil
		}
	default:
		h.fail("unmodeled update field %s", field)
	}
	h.local.Elements[ref] = el
	h.committed()
}

func (h *harness) share(ref, parent string, x, y float64) {
	el := h.local.Elements[ref]
	h.command(false, "add", el.Name, "--ref", ref, "--parent", parent, "--position-x", number(x), "--position-y", number(y))
	found := false
	for i, p := range el.Placements {
		if p.Parent == parent {
			el.Placements[i] = placement{parent, x, y}
			found = true
		}
	}
	if !found {
		el.Placements = append(el.Placements, placement{parent, x, y})
	}
	h.local.Elements[ref] = el
	h.committed()
}

func (h *harness) unplace(ref, parent string) {
	h.command(false, "view", "unplace", parent, ref)
	el := h.local.Elements[ref]
	kept := []placement{}
	for _, p := range el.Placements {
		if p.Parent != parent {
			kept = append(kept, p)
		}
	}
	el.Placements = kept
	h.local.Elements[ref] = el
	h.committed()
}

func (h *harness) remove(ref string) {
	h.command(false, "remove", "element", ref)
	delete(h.local.Elements, ref)
	delete(h.ids.Elements, ref)
	delete(h.ids.Views, ref)
	h.committed()
}

func (h *harness) connect(view, source, target, label string) string {
	key := (connector{View: view, Source: source, Target: target, Label: label}).key()
	c := h.local.Connectors[key] // An upsert preserves fields omitted from flags.
	c.View, c.Source, c.Target, c.Label = view, source, target, label
	c.Description, c.Direction, c.Style, c.Tags = "database access", "both", "straight", []string{"data"}
	h.command(false, "connect", "--view", view, "--from", source, "--to", target, "--label", label, "--description", c.Description, "--direction", c.Direction, "--style", c.Style, "--tags", "data", "--no-reposition")
	h.local.Connectors[c.key()] = c
	h.learnID("connector", c.key())
	h.committed()
	return c.key()
}

func (h *harness) relabel(key, label string) string {
	h.command(false, "update", "connector", key, "label", label)
	c := h.local.Connectors[key]
	delete(h.local.Connectors, key)
	c.Label = label
	h.local.Connectors[c.key()] = c
	h.ids.Connectors[c.key()] = h.ids.Connectors[key]
	delete(h.ids.Connectors, key)
	h.committed()
	return c.key()
}

func (h *harness) disconnect(key string) {
	h.command(false, "remove", "connector", key)
	delete(h.local.Connectors, key)
	delete(h.ids.Connectors, key)
	h.committed()
}

func (h *harness) updateConnectorDescription(key, description string) {
	h.command(false, "update", "connector", key, "description", description)
	c := h.local.Connectors[key]
	c.Description = description
	h.local.Connectors[key] = c
	h.committed()
}

func (h *harness) externalConnectorDescription(key, description string) {
	h.sqlExec("UPDATE connectors SET description = ?, updated_at = ? WHERE id = ?", description, time.Now().UTC().Format(time.RFC3339Nano), h.ids.Connectors[key])
	c := h.target.Connectors[key]
	c.Description = description
	h.target.Connectors[key] = c
	h.trace = append(h.trace, "external SQLite connector edit "+key+" description="+description)
	h.check()
}

func renameIn(g *graph, from, to string) {
	g.Elements[to] = g.Elements[from]
	delete(g.Elements, from)
	for ref, el := range g.Elements {
		for i := range el.Placements {
			if el.Placements[i].Parent == from {
				el.Placements[i].Parent = to
			}
		}
		g.Elements[ref] = el
	}
	connectors := map[string]connector{}
	for _, c := range g.Connectors {
		if c.View == from {
			c.View = to
		}
		if c.Source == from {
			c.Source = to
		}
		if c.Target == from {
			c.Target = to
		}
		connectors[c.key()] = c
	}
	g.Connectors = connectors
}

func (h *harness) rename(from, to string) {
	h.command(false, "update", "element", from, "ref", to)
	oldConnectors := h.local.clone().Connectors
	rename := func(g graph) graph {
		g = g.clone()
		renameIn(&g, from, to)
		return g
	}
	h.local, h.target = rename(h.local), rename(h.target)
	h.ids.Elements[to] = h.ids.Elements[from]
	delete(h.ids.Elements, from)
	if id, ok := h.ids.Views[from]; ok {
		h.ids.Views[to] = id
		delete(h.ids.Views, from)
	}
	ids := map[string]int32{}
	for key, c := range oldConnectors {
		if c.View == from {
			c.View = to
		}
		if c.Source == from {
			c.Source = to
		}
		if c.Target == from {
			c.Target = to
		}
		ids[c.key()] = h.ids.Connectors[key]
	}
	h.ids.Connectors = ids
	// The sync baseline still records the aliases used at the last target
	// reconciliation; only current aliases and the cache hash change here.
	h.baseline = h.lock().Hash
	h.check()
}

// Edit the actual YAML document, preserving fields outside the edit. No CLI
// command, workspace writer, importer, or model serialization performs it.
func (h *harness) manual(ref, description string, x, y float64) {
	var raw map[string]any
	h.readYAML("elements.yaml", &raw)
	el := raw[ref].(map[string]any)
	el["description"] = description
	placements := el["placements"].([]any)
	parent := h.local.Elements[ref].Placements[0].Parent
	var first map[string]any
	for _, p := range placements {
		candidate := p.(map[string]any)
		if candidate["parent"] == parent {
			first = candidate
			break
		}
	}
	if first == nil {
		h.fail("manual edit: missing placement %s/%s", parent, ref)
	}
	first["position_x"], first["position_y"] = x, y
	b, err := yaml.Marshal(raw)
	if err != nil {
		h.fail("manual YAML marshal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(h.workspace, "elements.yaml"), b, 0o600); err != nil {
		h.fail("manual YAML write: %v", err)
	}
	h.trace = append(h.trace, "manual YAML edit "+ref+":\n"+string(b))
	expected := h.local.Elements[ref]
	expected.Description = description
	for i := range expected.Placements {
		if expected.Placements[i].Parent == parent {
			expected.Placements[i].X, expected.Placements[i].Y = x, y
		}
	}
	h.local.Elements[ref] = expected
	h.dirty = true
	h.check()
}

func (h *harness) sync() {
	h.command(false, "sync")
	// Sync shares import's patch semantics: an omitted/empty description is
	// not an instruction to clear a value subsequently changed on the target.
	// Clearing is an explicit `update element ... description ""` operation.
	for ref, el := range h.local.Elements {
		if el.Description == "" {
			el.Description = h.target.Elements[ref].Description
			h.local.Elements[ref] = el
		}
	}
	h.committed()
}

func (h *harness) pull(force bool) {
	args := []string{"pull"}
	if force {
		args = append(args, "--force")
	}
	h.command(false, args...)
	// Export preserves workspace-only visibility preferences across pull,
	// including --force. They reach the target only through sync.
	visibility := map[string]int{}
	for key, c := range h.local.Connectors {
		visibility[key] = c.VisibilityDelta
	}
	h.local = h.target.clone()
	for key, c := range h.local.Connectors {
		c.VisibilityDelta = visibility[key]
		h.local.Connectors[key] = c
	}
	h.dirty = false
	h.acceptBaseline()
	h.check()
}

// Simulate another local client/UI without using the cache/export under test.
func (h *harness) externalDescription(ref, description string) {
	h.sqlExec("UPDATE elements SET description = ?, updated_at = ? WHERE id = ?", description, time.Now().UTC().Format(time.RFC3339Nano), h.ids.Elements[ref])
	el := h.target.Elements[ref]
	el.Description = description
	h.target.Elements[ref] = el
	h.trace = append(h.trace, "external SQLite edit "+ref+" description="+description)
	h.check()
}
