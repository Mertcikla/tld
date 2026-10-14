package workspace

import (
	"fmt"
	"sort"
)

// ViewDiagnostics describes which stored relationships an export can display.
type ViewDiagnostics struct {
	View               string   `json:"view"`
	Elements           int      `json:"elements"`
	Connectors         int      `json:"connectors"`
	RenderedConnectors int      `json:"rendered_connectors"`
	OmittedConnectors  []string `json:"omitted_connectors"`
}

// ElementPlacedInView uses the same implicit root placement as workspace renders.
func (ws *Workspace) ElementPlacedInView(ref, view string) bool {
	el := ws.Elements[ref]
	if el == nil {
		return false
	}
	if view == "" {
		view = RootRef
	}
	if len(el.Placements) == 0 {
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
	d := ViewDiagnostics{View: view, OmittedConnectors: []string{}}
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
		if ws.ElementPlacedInView(c.Source, view) && ws.ElementPlacedInView(c.Target, view) {
			d.RenderedConnectors++
		} else {
			d.OmittedConnectors = append(d.OmittedConnectors, key)
		}
	}
	sort.Strings(d.OmittedConnectors)
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

// CheckConnectorOwner rejects the canvas owner as a child endpoint. The user
// can connect the owner in its containing view or connect actual children here.
func CheckConnectorOwner(view, source, target string) error {
	if view == "" || view == RootRef {
		return nil
	}
	if source == view || target == view {
		return fmt.Errorf("view owner %q cannot be an endpoint inside its own canvas; connect its child components in --view %s, or connect %s in its containing view", view, view, view)
	}
	return nil
}
