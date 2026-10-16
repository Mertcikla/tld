package workspace

import (
	"sort"
	"strings"
)

// ViewDiagnostics describes direct Mermaid export coverage. Off-view connectors
// remain valid UI context relationships; this does not simulate UI filters.
type ViewDiagnostics struct {
	View                 string                         `json:"view"`
	Elements             int                            `json:"elements"`
	Connectors           int                            `json:"connectors"`
	RenderedConnectors   int                            `json:"rendered_connectors"`
	OmittedConnectors    []string                       `json:"omitted_connectors"`
	OffViewConnectors    []string                       `json:"off_view_connectors"`
	UnresolvedConnectors []string                       `json:"unresolved_connectors"`
	ConnectorVisibility  map[string]ConnectorVisibility `json:"connector_visibility"`
}

// OffViewHint describes UI behavior, not a promise that current filters expose
// every relationship. Mermaid intentionally exports only direct placements.
const OffViewHint = "Off-view relationships preserve component placement. The UI can show them through ancestor/context nodes or +N badges, depending on visible hierarchy, off-view filters, and connector budget. Mermaid exports only directly placed endpoints."

type ConnectorVisibility struct {
	UIHint           string              `json:"ui_hint,omitempty"`
	Mode             string              `json:"mode"`
	OffViewEndpoints []string            `json:"off_view_endpoints"`
	MissingEndpoints []string            `json:"missing_endpoints"`
	EndpointViews    map[string][]string `json:"endpoint_views"`
}

func (ws *Workspace) DiagnoseConnector(c *Connector) ConnectorVisibility {
	d := ConnectorVisibility{Mode: "on-view", OffViewEndpoints: []string{}, MissingEndpoints: []string{}, EndpointViews: map[string][]string{}}
	view := c.View
	if view == "" {
		view = RootRef
	}
	for _, ref := range []string{c.Source, c.Target} {
		if _, seen := d.EndpointViews[ref]; seen {
			continue
		}
		d.EndpointViews[ref] = []string{}
		el := ws.Elements[ref]
		if el == nil {
			d.MissingEndpoints = append(d.MissingEndpoints, ref)
			continue
		}
		views := map[string]bool{}
		if len(el.Placements) == 0 && ws.ElementPlacedInView(ref, RootRef) {
			views[RootRef] = true
		}
		for _, p := range el.Placements {
			parent := p.ParentRef
			if parent == "" {
				parent = RootRef
			}
			views[parent] = true
		}
		for parent := range views {
			d.EndpointViews[ref] = append(d.EndpointViews[ref], parent)
		}
		sort.Strings(d.EndpointViews[ref])
		if !views[view] {
			d.OffViewEndpoints = append(d.OffViewEndpoints, ref)
		}
	}
	if len(d.MissingEndpoints) > 0 {
		d.Mode = "unresolved"
	} else if len(d.OffViewEndpoints) > 0 {
		d.Mode = "off-view"
		d.UIHint = OffViewHint
	}
	return d
}

func (d ConnectorVisibility) Description() string {
	details := []string{}
	for _, ref := range d.OffViewEndpoints {
		if views := d.EndpointViews[ref]; len(views) > 0 {
			details = append(details, ref+" in "+strings.Join(views, ", "))
		} else {
			details = append(details, ref+" unplaced")
		}
	}
	for _, ref := range d.MissingEndpoints {
		details = append(details, ref+" missing")
	}
	if len(details) == 0 {
		return d.Mode
	}
	return d.Mode + " (" + strings.Join(details, "; ") + ")"
}

// ElementPlacedInView gives unapplied YAML elements an implicit root placement.
// Persisted elements with no placements remain unplaced in every view.
func (ws *Workspace) ElementPlacedInView(ref, view string) bool {
	el := ws.Elements[ref]
	if el == nil {
		return false
	}
	if view == "" {
		view = RootRef
	}
	if len(el.Placements) == 0 {
		if ws.Meta != nil {
			if meta := ws.Meta.Elements[ref]; meta != nil && meta.ID != 0 {
				return false
			}
		}
		return view == RootRef
	}
	for _, p := range el.Placements {
		parent := p.ParentRef
		if parent == "" {
			parent = RootRef
		}
		if parent == view {
			return true
		}
	}
	return false
}

func (ws *Workspace) DiagnoseView(view string) ViewDiagnostics {
	if view == "" {
		view = RootRef
	}
	d := ViewDiagnostics{View: view, OmittedConnectors: []string{}, OffViewConnectors: []string{}, UnresolvedConnectors: []string{}, ConnectorVisibility: map[string]ConnectorVisibility{}}
	for ref := range ws.Elements {
		if ws.ElementPlacedInView(ref, view) {
			d.Elements++
		}
	}
	for key, c := range ws.Connectors {
		if c == nil {
			continue
		}
		parent := c.View
		if parent == "" {
			parent = RootRef
		}
		if parent != view {
			continue
		}
		d.Connectors++
		visibility := ws.DiagnoseConnector(c)
		d.ConnectorVisibility[key] = visibility
		if visibility.Mode == "on-view" {
			d.RenderedConnectors++
		} else {
			d.OmittedConnectors = append(d.OmittedConnectors, key)
			if visibility.Mode == "off-view" {
				d.OffViewConnectors = append(d.OffViewConnectors, key)
			} else {
				d.UnresolvedConnectors = append(d.UnresolvedConnectors, key)
			}
		}
	}
	sort.Strings(d.OmittedConnectors)
	sort.Strings(d.OffViewConnectors)
	sort.Strings(d.UnresolvedConnectors)
	return d
}

func (ws *Workspace) ViewDiagnostics() []ViewDiagnostics {
	refs := map[string]bool{RootRef: true}
	for ref, el := range ws.Elements {
		if el != nil && el.HasView {
			refs[ref] = true
		}
	}
	for _, c := range ws.Connectors {
		if c != nil && c.View != "" {
			refs[c.View] = true
		}
	}
	ordered := make([]string, 0, len(refs))
	for ref := range refs {
		ordered = append(ordered, ref)
	}
	sort.Strings(ordered)
	out := make([]ViewDiagnostics, 0, len(ordered))
	for _, ref := range ordered {
		out = append(out, ws.DiagnoseView(ref))
	}
	return out
}
