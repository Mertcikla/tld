package exec

import (
	"context"
	"errors"
	"fmt"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
)

// errElementNotOnServer indicates the server list succeeded but the element was
// not present, so callers may safely create it from the YAML spec.
var errElementNotOnServer = errors.New("element not present on server")

// ResolveElementID returns the server ID for an element ref, using YAML metadata
// first and falling back to a server list+name match.
func ResolveElementID(ctx context.Context, runner Runner, ws *workspace.Workspace, ref string) (int32, error) {
	if ws.Meta != nil {
		if m, ok := ws.Meta.Elements[ref]; ok && m != nil && m.ID != 0 {
			return int32(m.ID), nil
		}
	}
	el, ok := ws.Elements[ref]
	if !ok || el == nil {
		return 0, fmt.Errorf("element %q not found", ref)
	}
	els, err := runner.ListElements(ctx, el.Name)
	if err != nil {
		return 0, fmt.Errorf("list elements: %w", err)
	}
	var fallback *diagv1.Element
	for _, e := range els {
		if e.GetName() != el.Name {
			continue
		}
		// Prefer a kind match to disambiguate elements that share a name.
		if el.Kind != "" && e.GetKind() == el.Kind {
			return e.GetId(), nil
		}
		if fallback == nil {
			fallback = e
		}
	}
	if fallback != nil {
		return fallback.GetId(), nil
	}
	return 0, fmt.Errorf("%w: element %q has no server ID; run `tld pull` to resync", errElementNotOnServer, ref)
}

// EnsureElementID returns the server ID for an element ref, creating the
// element on the server from the YAML spec when it has no cached ID yet.
// This keeps hand-written YAML usable: commands self-heal missing server state.
// Server errors are surfaced instead of masked with a create, so an unreachable
// server does not produce duplicate resources.
func EnsureElementID(ctx context.Context, runner Runner, ws *workspace.Workspace, wdir, ref string) (int32, error) {
	id, err := ResolveElementID(ctx, runner, ws, ref)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, errElementNotOnServer) {
		return 0, err
	}
	el, ok := ws.Elements[ref]
	if !ok || el == nil {
		return 0, fmt.Errorf("element %q not found", ref)
	}
	bypass := true
	created, err := runner.CreateElement(ctx, api.ElementInput{
		Name:            el.Name,
		Description:     strOrNil(el.Description),
		Kind:            strOrNil(el.Kind),
		Technology:      strOrNil(el.Technology),
		URL:             strOrNil(el.URL),
		LogoURL:         strOrNil(el.LogoURL),
		Tags:            el.Tags,
		Repo:            strOrNil(el.Repo),
		Branch:          strOrNil(el.Branch),
		Language:        strOrNil(el.Language),
		FilePath:        strOrNil(el.FilePath),
		BypassNoiseGate: &bypass,
		HasView:         el.HasView,
		ViewLabel:       strOrNil(el.ViewLabel),
	})
	if err != nil {
		return 0, fmt.Errorf("auto-create missing element %q: %w", ref, err)
	}
	if err := RecordElementMeta(wdir, ref, created, 0, nil); err != nil {
		return 0, err
	}
	return created.GetId(), nil
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// RootViewID returns the ID of the synthetic root view.
func RootViewID(ctx context.Context, runner Runner) (int32, error) {
	views, err := runner.ListViews(ctx)
	if err != nil {
		return 0, err
	}
	for _, v := range views {
		if v.ParentViewId == nil && v.OwnerElementId == nil {
			return v.GetId(), nil
		}
	}
	// Fall back to any root-level view.
	for _, v := range views {
		if v.ParentViewId == nil {
			return v.GetId(), nil
		}
	}
	return 0, fmt.Errorf("root view not found")
}

// EnsureElementView returns the view owned by the given element, creating it if needed.
func EnsureElementView(ctx context.Context, runner Runner, elementID int32, elementName string, label *string) (int32, error) {
	views, err := runner.ListViews(ctx)
	if err != nil {
		return 0, err
	}
	for _, v := range views {
		if v.OwnerElementId != nil && *v.OwnerElementId == elementID {
			return v.GetId(), nil
		}
	}
	created, err := runner.CreateView(ctx, &elementID, elementName, label)
	if err != nil {
		return 0, err
	}
	return created.GetId(), nil
}

// ResolveParentViewID maps a YAML placement parent ref to a server view ID,
// ensuring the parent element has a view (mirrors the legacy
// canonical-view promotion). Missing elements are auto-created from YAML.
func ResolveParentViewID(ctx context.Context, runner Runner, ws *workspace.Workspace, wdir, parentRef string) (int32, error) {
	if parentRef == "" || parentRef == workspace.RootRef {
		return RootViewID(ctx, runner)
	}
	if ws.Meta != nil {
		if m, ok := ws.Meta.Views[parentRef]; ok && m != nil && m.ID != 0 {
			return int32(m.ID), nil
		}
	}
	parentID, err := EnsureElementID(ctx, runner, ws, wdir, parentRef)
	if err != nil {
		return 0, err
	}
	parentEl := ws.Elements[parentRef]
	name := parentRef
	var label *string
	if parentEl != nil {
		name = parentEl.Name
		if parentEl.ViewLabel != "" {
			label = &parentEl.ViewLabel
		}
	}
	return EnsureElementView(ctx, runner, parentID, name, label)
}

// RecordElementMeta stores element IDs in the workspace meta (plus the owned
// view ID when ownedViewID != 0) and persists the YAML cache + lockfile hash.
func RecordElementMeta(wdir, ref string, element *diagv1.Element, ownedViewID int32, view *diagv1.View) error {
	ws, err := workspace.Load(wdir)
	if err != nil {
		return err
	}
	if ws.Meta == nil {
		ws.Meta = &workspace.Meta{
			Elements:   map[string]*workspace.ResourceMetadata{},
			Views:      map[string]*workspace.ResourceMetadata{},
			Connectors: map[string]*workspace.ResourceMetadata{},
		}
	}
	updatedAt := time.Now()
	if element.UpdatedAt != nil {
		updatedAt = element.UpdatedAt.AsTime()
	}
	ws.Meta.Elements[ref] = &workspace.ResourceMetadata{ID: workspace.ResourceID(element.GetId()), UpdatedAt: updatedAt}
	if ownedViewID != 0 {
		viewUpdated := updatedAt
		if view != nil && view.UpdatedAt != nil {
			viewUpdated = view.UpdatedAt.AsTime()
		}
		ws.Meta.Views[ref] = &workspace.ResourceMetadata{ID: workspace.ResourceID(ownedViewID), UpdatedAt: viewUpdated}
	}
	return persistCache(wdir, ws)
}

// RecordViewMeta stores the owned-view ID for a ref (e.g. after promoting a
// placement parent to a view) and persists the YAML cache + lockfile hash.
func RecordViewMeta(wdir, ref string, viewID int32) error {
	ws, err := workspace.Load(wdir)
	if err != nil {
		return err
	}
	if ws.Meta == nil {
		ws.Meta = &workspace.Meta{
			Elements:   map[string]*workspace.ResourceMetadata{},
			Views:      map[string]*workspace.ResourceMetadata{},
			Connectors: map[string]*workspace.ResourceMetadata{},
		}
	}
	if ws.Meta.Views == nil {
		ws.Meta.Views = map[string]*workspace.ResourceMetadata{}
	}
	ws.Meta.Views[ref] = &workspace.ResourceMetadata{ID: workspace.ResourceID(viewID), UpdatedAt: time.Now()}
	return persistCache(wdir, ws)
}

// RecordConnectorMeta stores a connector ID in the workspace meta.
func RecordConnectorMeta(wdir, key string, connector *diagv1.Connector) error {
	ws, err := workspace.Load(wdir)
	if err != nil {
		return err
	}
	if ws.Meta == nil {
		ws.Meta = &workspace.Meta{
			Elements:   map[string]*workspace.ResourceMetadata{},
			Views:      map[string]*workspace.ResourceMetadata{},
			Connectors: map[string]*workspace.ResourceMetadata{},
		}
	}
	updatedAt := time.Now()
	if connector.UpdatedAt != nil {
		updatedAt = connector.UpdatedAt.AsTime()
	}
	if ws.Meta.Connectors == nil {
		ws.Meta.Connectors = map[string]*workspace.ResourceMetadata{}
	}
	ws.Meta.Connectors[key] = &workspace.ResourceMetadata{ID: workspace.ResourceID(connector.GetId()), UpdatedAt: updatedAt}
	return persistCache(wdir, ws)
}

// DropElementMeta removes element/view meta entries after deletion.
func DropElementMeta(wdir, ref string) error {
	ws, err := workspace.Load(wdir)
	if err != nil {
		return err
	}
	if ws.Meta != nil {
		delete(ws.Meta.Elements, ref)
		delete(ws.Meta.Views, ref)
	}
	return persistCache(wdir, ws)
}

// DropConnectorMeta removes a connector meta entry after deletion.
func DropConnectorMeta(wdir string, keys ...string) error {
	ws, err := workspace.Load(wdir)
	if err != nil {
		return err
	}
	if ws.Meta != nil {
		for _, k := range keys {
			delete(ws.Meta.Connectors, k)
		}
	}
	return persistCache(wdir, ws)
}

func persistCache(wdir string, ws *workspace.Workspace) error {
	if err := workspace.Save(ws); err != nil {
		return err
	}
	hash, err := workspace.CalculateWorkspaceHash(wdir)
	if err != nil {
		return err
	}
	lockFile, err := workspace.LoadLockFile(wdir)
	if err != nil {
		return err
	}
	if lockFile == nil {
		lockFile = &workspace.LockFile{Version: "v1"}
	}
	workspace.UpdateLockFile(lockFile, lockFile.VersionID, "cli", &workspace.ResourceCounts{
		Elements:   len(ws.Elements),
		Views:      countViews(ws),
		Connectors: len(ws.Connectors),
	}, hash, nil, ws.Meta)
	if lockFile.VersionID == "" {
		lockFile.VersionID = fmt.Sprintf("cli-%s", time.Now().UTC().Format(time.RFC3339))
	}
	return workspace.WriteLockFile(wdir, lockFile)
}

func countViews(ws *workspace.Workspace) int {
	n := 0
	for _, el := range ws.Elements {
		if el != nil && el.HasView {
			n++
		}
	}
	return n
}
