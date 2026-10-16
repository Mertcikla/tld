//go:build cliworkflow

package cliworkflow

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

// Deliberately independent of workspace.Element/Connector, the importer, and
// the export converter. Expectations are changed only by action semantics.
type element struct {
	Name         string      `yaml:"name"`
	Kind         string      `yaml:"kind"`
	Description  string      `yaml:"description"`
	Technology   string      `yaml:"technology"`
	URL          string      `yaml:"url"`
	LogoURL      string      `yaml:"logo_url"`
	Repo         string      `yaml:"repo"`
	RepositoryID string      `yaml:"repository_id"`
	Branch       string      `yaml:"branch"`
	FilePath     string      `yaml:"file_path"`
	Language     string      `yaml:"-"`
	Tags         []string    `yaml:"tags"`
	HasView      bool        `yaml:"has_view"`
	ViewName     string      `yaml:"view_name"`
	ViewLabel    string      `yaml:"view_label"`
	Placements   []placement `yaml:"placements"`
}
type placement struct {
	Parent string  `yaml:"parent"`
	X      float64 `yaml:"position_x"`
	Y      float64 `yaml:"position_y"`
}
type connector struct {
	View            string   `yaml:"view"`
	Source          string   `yaml:"source"`
	Target          string   `yaml:"target"`
	Label           string   `yaml:"label"`
	Description     string   `yaml:"description"`
	Direction       string   `yaml:"direction"`
	Style           string   `yaml:"style"`
	Tags            []string `yaml:"tags"`
	Relationship    string   `yaml:"relationship"`
	URL             string   `yaml:"url"`
	SourceHandle    string   `yaml:"source_handle"`
	TargetHandle    string   `yaml:"target_handle"`
	VisibilityDelta int      `yaml:"visibility_delta"`
}

func (c connector) key() string {
	key := c.View + "/" + c.Source + "~" + c.Target
	if c.Label != "" {
		key += "/" + c.Label
	}
	return key
}

type graph struct {
	Elements   map[string]element
	Connectors map[string]connector
}

func emptyGraph() graph { return graph{map[string]element{}, map[string]connector{}} }
func (g graph) clone() graph {
	var copy graph
	if err := json.Unmarshal([]byte(asJSON(g)), &copy); err != nil {
		panic(err)
	}
	return copy
}
func (g graph) canonical() string {
	g = g.clone()
	for ref, el := range g.Elements {
		if el.LogoURL == "" {
			switch el.Technology {
			case "go":
				el.LogoURL = "/icons/go.svg"
			case "rust":
				el.LogoURL = "/icons/rust.svg"
			}
		}
		if el.Tags == nil {
			el.Tags = []string{}
		}
		if el.Placements == nil {
			el.Placements = []placement{}
		}
		sort.Strings(el.Tags)
		sort.Slice(el.Placements, func(i, j int) bool { return el.Placements[i].Parent < el.Placements[j].Parent })
		// An omitted view_name means the element display name.
		if el.HasView && el.ViewName == "" {
			el.ViewName = el.Name
		}
		g.Elements[ref] = el
	}
	for key, c := range g.Connectors {
		if c.Tags == nil {
			c.Tags = []string{}
		}
		sort.Strings(c.Tags)
		g.Connectors[key] = c
	}
	return asJSON(g)
}

func (h *harness) yamlGraph() graph {
	g := emptyGraph()
	h.readYAML("elements.yaml", &g.Elements)
	var connectors []connector
	h.readYAML("connectors.yaml", &connectors)
	for _, c := range connectors {
		if _, exists := g.Connectors[c.key()]; exists {
			h.fail("duplicate YAML connector: %s", c.key())
		}
		g.Connectors[c.key()] = c
	}
	return g
}

func reverse(ids map[string]int32) map[int32]string {
	refs := map[int32]string{}
	for ref, id := range ids {
		refs[id] = ref
	}
	return refs
}

func (h *harness) sqlGraph(db *sql.DB) graph {
	g := emptyGraph()
	elements, views := reverse(h.ids.Elements), reverse(h.ids.Views)
	h.query(db, `SELECT id, name, COALESCE(kind,''), COALESCE(description,''), COALESCE(technology,''), tags, COALESCE(url,''), COALESCE(logo_url,''), COALESCE(repo,''), COALESCE(repository_id,''), COALESCE(branch,''), COALESCE(file_path,''), COALESCE(language,'') FROM elements`, func(r *sql.Rows) {
		var id int32
		var el element
		var tags string
		h.scan(r, &id, &el.Name, &el.Kind, &el.Description, &el.Technology, &tags, &el.URL, &el.LogoURL, &el.Repo, &el.RepositoryID, &el.Branch, &el.FilePath, &el.Language)
		ref, ok := elements[id]
		if !ok {
			h.fail("unexpected SQLite element ID=%d name=%s", id, el.Name)
		}
		if err := json.Unmarshal([]byte(tags), &el.Tags); err != nil {
			h.fail("element tags: %v", err)
		}
		g.Elements[ref] = el
	})
	rootCount := 0
	h.query(db, `SELECT id, owner_element_id, name, COALESCE(level_label,'') FROM views`, func(r *sql.Rows) {
		var id int32
		var owner sql.NullInt32
		var name, label string
		h.scan(r, &id, &owner, &name, &label)
		if !owner.Valid {
			rootCount++
			if name != "Workspace" || label != "Root" {
				h.fail("root view changed: %s %s", name, label)
			}
			views[id] = "root"
			return
		}
		ref, ok := views[id]
		if !ok || h.ids.Elements[ref] != owner.Int32 {
			h.fail("unexpected view ID=%d owner=%d", id, owner.Int32)
		}
		el := g.Elements[ref]
		el.HasView, el.ViewName, el.ViewLabel = true, name, label
		g.Elements[ref] = el
	})
	if rootCount != 1 {
		h.fail("expected one root view, got %d", rootCount)
	}
	h.query(db, `SELECT view_id, element_id, position_x, position_y FROM placements`, func(r *sql.Rows) {
		var viewID, elementID int32
		var p placement
		h.scan(r, &viewID, &elementID, &p.X, &p.Y)
		p.Parent = views[viewID]
		ref, ok := elements[elementID]
		if !ok || p.Parent == "" {
			h.fail("unresolved SQL placement %d/%d", viewID, elementID)
		}
		el := g.Elements[ref]
		for _, old := range el.Placements {
			if old.Parent == p.Parent {
				h.fail("duplicate placement %s/%s", p.Parent, ref)
			}
		}
		el.Placements = append(el.Placements, p)
		g.Elements[ref] = el
	})
	h.query(db, `SELECT c.id, c.view_id, source_element_id, target_element_id, COALESCE(label,''), COALESCE(description,''), direction, style, tags, COALESCE(relationship,''), COALESCE(url,''), COALESCE(source_handle,''), COALESCE(target_handle,''), COALESCE(o.level_delta,0) FROM connectors c LEFT JOIN view_visibility_overrides o ON o.view_id=c.view_id AND o.resource_type='connector' AND o.resource_id=c.id`, func(r *sql.Rows) {
		var id, viewID, sourceID, targetID int32
		var c connector
		var tags string
		h.scan(r, &id, &viewID, &sourceID, &targetID, &c.Label, &c.Description, &c.Direction, &c.Style, &tags, &c.Relationship, &c.URL, &c.SourceHandle, &c.TargetHandle, &c.VisibilityDelta)
		c.View, c.Source, c.Target = views[viewID], elements[sourceID], elements[targetID]
		if c.View == "" || c.Source == "" || c.Target == "" {
			h.fail("unresolved SQL connector %d", id)
		}
		if h.ids.Connectors[c.key()] != id {
			h.fail("connector identity changed or unexpected connector: %s id=%d", c.key(), id)
		}
		if _, exists := g.Connectors[c.key()]; exists {
			h.fail("duplicate SQL connector %s", c.key())
		}
		if err := json.Unmarshal([]byte(tags), &c.Tags); err != nil {
			h.fail("connector tags: %v", err)
		}
		g.Connectors[c.key()] = c
	})
	return g
}

func (h *harness) check() {
	local := h.local.clone()
	for ref, el := range local.Elements {
		el.Language = ""
		local.Elements[ref] = el
	}
	if got, want := h.yamlGraph().canonical(), local.canonical(); got != want {
		h.fail("YAML differs from model\nwant=%s\ngot=%s", want, got)
	}
	db := h.db()
	defer func() { _ = db.Close() }()
	if h.dsn == "" {
		h.query(db, "PRAGMA integrity_check", func(r *sql.Rows) {
			var result string
			h.scan(r, &result)
			if result != "ok" {
				h.fail("SQLite integrity: %s", result)
			}
		})
		h.query(db, "PRAGMA foreign_key_check", func(r *sql.Rows) { h.fail("SQLite foreign key violation") })
	}
	if got, want := h.sqlGraph(db).canonical(), h.target.canonical(); got != want {
		h.fail("SQLite differs from model\nwant=%s\ngot=%s", want, got)
	}
	lock := h.lock()
	current := identities{h.decodeIDs(lock.Elements), h.decodeIDs(lock.Views), h.decodeIDs(lock.Connectors)}
	baseline := identities{h.decodeIDs(lock.Metadata.Elements), h.decodeIDs(lock.Metadata.Views), h.decodeIDs(lock.Metadata.Connectors)}
	if asJSON(current) != asJSON(h.ids) || asJSON(baseline) != asJSON(h.baselineIDs) {
		h.fail("metadata identity drift\nwant current=%s\ncurrent=%s\nwant baseline=%s\nbaseline=%s", asJSON(h.ids), asJSON(current), asJSON(h.baselineIDs), asJSON(baseline))
	}
	if lock.Hash == "" || lock.Hash != h.baseline {
		h.fail("unexpected sync baseline hash: %s, want %s", lock.Hash, h.baseline)
	}
	// This is a cache-consistency assertion, separate from the independent
	// semantic oracle above. Position fields intentionally do not dirty the
	// production sync hash; generated manual edits also change descriptions.
	hash, err := workspace.CalculateWorkspaceHash(h.workspace)
	if err != nil || (hash != h.baseline) != h.dirty {
		h.fail("sync hash disagrees with pending-edit state: hash=%s baseline=%s dirty=%t error=%v", hash, h.baseline, h.dirty, err)
	}
	if lock.Resources.Elements != len(h.target.Elements) || lock.Resources.Views != len(h.ids.Views) || lock.Resources.Connectors != len(h.target.Connectors) {
		h.fail("lock resource counts differ from target: %s", asJSON(lock.Resources))
	}
	// Structural validity is checked even while local edits are pending.
	for _, g := range []graph{h.local, h.target} {
		for ref, el := range g.Elements {
			for _, p := range el.Placements {
				if p.Parent != "root" && !g.Elements[p.Parent].HasView {
					h.fail("%s placed in missing view %s", ref, p.Parent)
				}
			}
		}
		for _, c := range g.Connectors {
			_, source := g.Elements[c.Source]
			_, target := g.Elements[c.Target]
			if !source || !target || (c.View != "root" && !g.Elements[c.View].HasView) {
				h.fail("unresolved modeled connector: %s", c.key())
			}
		}
	}
}

func number(n float64) string { return fmt.Sprintf("%g", n) }
