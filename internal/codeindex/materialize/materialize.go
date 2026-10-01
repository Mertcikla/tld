// Package materialize projects a codeindex snapshot into the workspace model:
// visible candidate elements and connectors become workspace resources, keyed
// by their canonical logical keys so repeated runs upsert rather than duplicate
// and stale resources are pruned. It is the deterministic replacement for the
// legacy watch represent/materialize step.
package materialize

import (
	"context"
	"fmt"
	"strings"

	"github.com/mertcikla/tld/v2/internal/codeindex/project"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/codeindex/visibility"
	"github.com/mertcikla/tld/v2/internal/core"
)

// IndexStore is the codeindex mapping surface the materializer needs.
type IndexStore interface {
	MappingByLogicalKey(ctx context.Context, logicalKey string) (cstore.ResourceMapping, bool, error)
	MappingsByRepository(ctx context.Context, repositoryID string) ([]cstore.ResourceMapping, error)
	SaveMappings(ctx context.Context, mappings []cstore.ResourceMapping) error
	DeleteMapping(ctx context.Context, logicalKey string) error
}

// Options configures a materialization run.
type Options struct {
	RepositoryID   string
	RepositoryName string
	SnapshotID     string
	ViewName       string
}

// Result summarises what changed.
type Result struct {
	ViewID     int64
	Elements   int
	Connectors int
	Pruned     int
}

// Apply projects the visible candidates into the workspace and prunes resources
// whose logical keys no longer appear.
func Apply(ctx context.Context, ws core.Store, idx IndexStore, proj project.Result, decisions []visibility.Decision, opts Options) (Result, error) {
	// Every candidate is materialized so identity mappings stay stable and
	// pruning reflects graph membership rather than a transient visibility
	// decision. Visibility drives the element's noise-gate bypass instead: a
	// visible candidate always shows, a hidden one is ranked and capped by the
	// density engine like any other generated resource.
	elementVisible := map[string]bool{}
	for _, d := range decisions {
		if d.Kind == "element" {
			elementVisible[d.Ref] = d.Visible
		}
	}
	visible := func(ref string) bool {
		if v, ok := elementVisible[ref]; ok {
			return v
		}
		return true
	}

	viewID, _, err := ensureView(ctx, ws, idx, opts)
	if err != nil {
		return Result{}, err
	}
	res := Result{ViewID: viewID}

	existing, err := idx.MappingsByRepository(ctx, opts.RepositoryID)
	if err != nil {
		return Result{}, err
	}
	byKey := make(map[string]cstore.ResourceMapping, len(existing))
	for _, m := range existing {
		byKey[m.LogicalKey] = m
	}

	placements, err := ws.Placements(ctx, viewID)
	if err != nil {
		return Result{}, err
	}
	placed := map[int64]bool{}
	for _, p := range placements {
		placed[p.ElementID] = true
	}

	kept := map[string]bool{}
	elemIDByRef := map[string]int64{}
	var pending []cstore.ResourceMapping
	position := 0
	for _, el := range proj.Elements {
		input := elementInput(el, opts, visible(el.Ref))
		var id int64
		if m, ok := byKey[el.Ref]; ok && m.Kind == cstore.MappingElement {
			if updated, err := ws.UpdateElement(ctx, m.ResourceID, input); err == nil {
				id = updated.ID
			}
		}
		if id == 0 {
			created, err := ws.CreateElement(ctx, input)
			if err != nil {
				return res, fmt.Errorf("create element %q: %w", el.Ref, err)
			}
			id = created.ID
		}
		elemIDByRef[el.Ref] = id
		kept[el.Ref] = true
		pending = append(pending, cstore.ResourceMapping{LogicalKey: el.Ref, Kind: cstore.MappingElement, ResourceID: id, RepositoryID: opts.RepositoryID, SnapshotID: opts.SnapshotID})
		if !placed[id] {
			x, y := gridPosition(position)
			if _, err := ws.AddPlacement(ctx, viewID, id, x, y); err != nil {
				return res, fmt.Errorf("place element %q: %w", el.Ref, err)
			}
			placed[id] = true
		}
		position++
		res.Elements++
	}

	for _, c := range proj.Connectors {
		fromID := elemIDByRef[c.FromRef]
		toID := elemIDByRef[c.ToRef]
		if fromID == 0 || toID == 0 {
			continue
		}
		input := connectorInput(viewID, fromID, toID, c)
		var id int64
		if m, ok := byKey[c.Ref]; ok && m.Kind == cstore.MappingConnector {
			if updated, err := ws.UpdateConnector(ctx, m.ResourceID, input); err == nil {
				id = updated.ID
			}
		}
		if id == 0 {
			created, err := ws.CreateConnector(ctx, input)
			if err != nil {
				return res, fmt.Errorf("create connector %q: %w", c.Ref, err)
			}
			id = created.ID
		}
		kept[c.Ref] = true
		pending = append(pending, cstore.ResourceMapping{LogicalKey: c.Ref, Kind: cstore.MappingConnector, ResourceID: id, RepositoryID: opts.RepositoryID, SnapshotID: opts.SnapshotID})
		res.Connectors++
	}

	// Prune resources that no longer appear, but never the view itself.
	for key, m := range byKey {
		if m.Kind == cstore.MappingView || kept[key] {
			continue
		}
		switch m.Kind {
		case cstore.MappingElement:
			_ = ws.DeleteElement(ctx, m.ResourceID)
		case cstore.MappingConnector:
			_ = ws.DeleteConnector(ctx, m.ResourceID)
		}
		if err := idx.DeleteMapping(ctx, key); err != nil {
			return res, err
		}
		res.Pruned++
	}

	if err := idx.SaveMappings(ctx, pending); err != nil {
		return res, err
	}
	return res, nil
}

// ScopedOptions configures a scoped apply into an existing view.
type ScopedOptions struct {
	RepositoryID   string
	RepositoryName string
	SnapshotID     string
	ViewID         int64
}

// ApplyScoped upserts a candidate subset into an existing view without pruning
// or adding placements. It backs populate: the matched facts are projected into
// workspace resources so the caller can place them, while resources outside the
// candidate set are left untouched.
func ApplyScoped(ctx context.Context, ws core.Store, idx IndexStore, proj project.Result, opts ScopedOptions) (Result, error) {
	if opts.ViewID == 0 {
		return Result{}, fmt.Errorf("scoped materialize requires a view")
	}
	res := Result{ViewID: opts.ViewID}
	base := Options{RepositoryID: opts.RepositoryID, RepositoryName: opts.RepositoryName, SnapshotID: opts.SnapshotID}

	existing, err := idx.MappingsByRepository(ctx, opts.RepositoryID)
	if err != nil {
		return res, err
	}
	byKey := make(map[string]cstore.ResourceMapping, len(existing))
	for _, m := range existing {
		byKey[m.LogicalKey] = m
	}

	elemIDByRef := map[string]int64{}
	var pending []cstore.ResourceMapping
	for _, el := range proj.Elements {
		input := elementInput(el, base, true)
		var id int64
		if m, ok := byKey[el.Ref]; ok && m.Kind == cstore.MappingElement {
			if updated, err := ws.UpdateElement(ctx, m.ResourceID, input); err == nil {
				id = updated.ID
			}
		}
		if id == 0 {
			created, err := ws.CreateElement(ctx, input)
			if err != nil {
				return res, fmt.Errorf("create element %q: %w", el.Ref, err)
			}
			id = created.ID
		}
		// Track the new resource in byKey so a repeated ref within this batch
		// updates it instead of creating a duplicate element.
		byKey[el.Ref] = cstore.ResourceMapping{LogicalKey: el.Ref, Kind: cstore.MappingElement, ResourceID: id, RepositoryID: opts.RepositoryID, SnapshotID: opts.SnapshotID}
		elemIDByRef[el.Ref] = id
		pending = append(pending, byKey[el.Ref])
		res.Elements++
	}

	for _, c := range proj.Connectors {
		fromID := elemIDByRef[c.FromRef]
		toID := elemIDByRef[c.ToRef]
		if fromID == 0 || toID == 0 {
			continue
		}
		input := connectorInput(opts.ViewID, fromID, toID, c)
		var id int64
		if m, ok := byKey[c.Ref]; ok && m.Kind == cstore.MappingConnector {
			if updated, err := ws.UpdateConnector(ctx, m.ResourceID, input); err == nil {
				id = updated.ID
			}
		}
		if id == 0 {
			created, err := ws.CreateConnector(ctx, input)
			if err != nil {
				return res, fmt.Errorf("create connector %q: %w", c.Ref, err)
			}
			id = created.ID
		}
		byKey[c.Ref] = cstore.ResourceMapping{LogicalKey: c.Ref, Kind: cstore.MappingConnector, ResourceID: id, RepositoryID: opts.RepositoryID, SnapshotID: opts.SnapshotID}
		pending = append(pending, byKey[c.Ref])
		res.Connectors++
	}

	if err := idx.SaveMappings(ctx, pending); err != nil {
		return res, err
	}
	return res, nil
}

func ensureView(ctx context.Context, ws core.Store, idx IndexStore, opts Options) (int64, bool, error) {
	viewKey := "view|" + opts.RepositoryID
	if m, ok, err := idx.MappingByLogicalKey(ctx, viewKey); err != nil {
		return 0, false, err
	} else if ok {
		if _, err := ws.ViewByID(ctx, m.ResourceID); err == nil {
			return m.ResourceID, false, nil
		}
	}
	name := opts.ViewName
	if name == "" {
		name = "Indexed Diagram"
		if opts.RepositoryName != "" {
			name = "Indexed: " + opts.RepositoryName
		}
	}
	label := "Indexed"
	view, err := ws.CreateView(ctx, name, &label, nil)
	if err != nil {
		return 0, false, fmt.Errorf("create view: %w", err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{{LogicalKey: viewKey, Kind: cstore.MappingView, ResourceID: view.ID, RepositoryID: opts.RepositoryID, SnapshotID: opts.SnapshotID}}); err != nil {
		return 0, false, err
	}
	return view.ID, true, nil
}

func elementInput(el project.Element, opts Options, visible bool) core.LibraryElement {
	kind := strings.ToLower(project.KindLabel(el.Kind))
	input := core.LibraryElement{
		Name:               elementName(el),
		Kind:               &kind,
		BypassNoiseGate:    visible,
		BypassNoiseGateSet: true,
	}
	repo := opts.RepositoryName
	if repo == "" {
		repo = el.Repository
	}
	if repo != "" {
		input.Repo = &repo
	}
	if el.FilePath != "" {
		path := el.FilePath
		input.FilePath = &path
	}
	if el.Language != "" {
		lang := el.Language
		input.Language = &lang
	}
	return input
}

func elementName(el project.Element) string {
	if strings.TrimSpace(el.Name) != "" {
		return el.Name
	}
	if el.SymbolKey != "" {
		return el.SymbolKey
	}
	return el.Ref
}

func connectorInput(viewID, fromID, toID int64, c project.Connector) core.Connector {
	rel := strings.ToLower(project.KindLabelEdge(c.Kind))
	return core.Connector{
		ViewID:          viewID,
		SourceElementID: fromID,
		TargetElementID: toID,
		Relationship:    &rel,
		Direction:       "forward",
		Style:           "bezier",
	}
}

func gridPosition(index int) (float64, float64) {
	const cols = 5
	col := index % cols
	row := index / cols
	return float64(120 + col*240), float64(120 + row*180)
}
