package workspacesource

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

type refResolver struct {
	state             *sqliteState
	elementRefByID    map[int32]string
	viewRefByID       map[int32]string
	connectorRefByID  map[int32]string
	usedElementRefs   map[string]struct{}
	usedViewRefs      map[string]struct{}
	usedConnectorRefs map[string]struct{}
}

func newRefResolver(lock *workspace.WorkspaceSourceLock, state *sqliteState) *refResolver {
	resolver := &refResolver{
		state:             state,
		elementRefByID:    map[int32]string{},
		viewRefByID:       map[int32]string{},
		connectorRefByID:  map[int32]string{},
		usedElementRefs:   map[string]struct{}{},
		usedViewRefs:      map[string]struct{}{"root": {}},
		usedConnectorRefs: map[string]struct{}{},
	}
	for ref, meta := range lock.ManagedElements {
		if meta == nil || meta.ID == 0 {
			continue
		}
		id := int32(meta.ID)
		if state.elementsByID[id] != nil {
			resolver.elementRefByID[id] = ref
			resolver.usedElementRefs[ref] = struct{}{}
		}
	}
	for ref, meta := range lock.ManagedViews {
		if meta == nil || meta.ID == 0 {
			continue
		}
		id := int32(meta.ID)
		if state.viewsByID[id] != nil {
			resolver.viewRefByID[id] = ref
			resolver.usedViewRefs[ref] = struct{}{}
		}
	}
	for ref, meta := range lock.ManagedConnectors {
		if meta == nil || meta.ID == 0 {
			continue
		}
		id := int32(meta.ID)
		if state.connectorsByID[id] != nil {
			resolver.connectorRefByID[id] = ref
			resolver.usedConnectorRefs[ref] = struct{}{}
		}
	}
	return resolver
}

func (r *refResolver) elementRef(element *diagv1.Element) string {
	if element == nil {
		return ""
	}
	if ref := r.elementRefByID[element.GetId()]; ref != "" {
		return ref
	}
	base := element.GetRef()
	if strings.TrimSpace(base) == "" {
		base = slugRef(element.GetName())
	}
	ref := uniqueRef(base, r.usedElementRefs, nil)
	r.elementRefByID[element.GetId()] = ref
	r.usedElementRefs[ref] = struct{}{}
	return ref
}

func (r *refResolver) viewRef(view *diagv1.View) string {
	if view == nil {
		return ""
	}
	if ref := r.viewRefByID[view.GetId()]; ref != "" {
		return ref
	}
	base := slugRef(view.GetName())
	if base == "root" || base == "" {
		base = fmt.Sprintf("view-%d", view.GetId())
	}
	ref := uniqueRef(base, r.usedViewRefs, map[string]struct{}{"root": {}})
	r.viewRefByID[view.GetId()] = ref
	r.usedViewRefs[ref] = struct{}{}
	return ref
}

func (r *refResolver) connectorRef(connector *diagv1.Connector, viewRef, sourceRef, targetRef string) string {
	if connector == nil {
		return ""
	}
	if connector.GetId() != 0 {
		ref := strconv.FormatInt(int64(connector.GetId()), 10)
		r.connectorRefByID[connector.GetId()] = ref
		r.usedConnectorRefs[ref] = struct{}{}
		return ref
	}
	if ref := r.connectorRefByID[connector.GetId()]; ref != "" {
		return ref
	}
	label := slugRef(connector.GetLabel())
	if label == "" {
		label = fmt.Sprintf("connector-%d", connector.GetId())
	}
	base := strings.Join(nonEmptyStrings(viewRef, sourceRef, targetRef, label), ":")
	ref := uniqueRef(base, r.usedConnectorRefs, nil)
	r.connectorRefByID[connector.GetId()] = ref
	r.usedConnectorRefs[ref] = struct{}{}
	return ref
}

var slugRefPattern = regexp.MustCompile(`[^a-z0-9_.-]+`)

func slugRef(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugRefPattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-.")
	return value
}

func uniqueRef(base string, used, reserved map[string]struct{}) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "ref"
	}
	if _, ok := reserved[base]; ok {
		base += "-ref"
	}
	ref := base
	for index := 2; ; index++ {
		if _, ok := used[ref]; !ok {
			if _, reserved := reserved[ref]; !reserved {
				return ref
			}
		}
		ref = fmt.Sprintf("%s-%d", base, index)
	}
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}
