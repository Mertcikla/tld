package warnings

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/tech"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// syntheticRootViewRef is the ref used for the root-level (synthetic) view.
const syntheticRootViewRef = "root"

// groundingMinScore is the 0-10 score below which the ARC205 rule warns at
// strict (level 3) validation.
const groundingMinScore = 5

// WarningGroup represents a collection of similar architectural warnings.
type WarningGroup struct {
	RuleCode    string
	RuleName    string
	Description string
	Mediation   string
	Violations  []string
	// Score is optional and only populated by rules that rank the workspace on
	// a 0-10 scale (currently ARC205).
	Score *WarningScore
}

// WarningScore is a 0-10 quality score for a rule, carrying the reasoning that
// produced it so users know how to improve.
type WarningScore struct {
	Value     int
	Grounded  int
	Eligible  int
	External  int
	Reasoning []string
	Views     []ViewScore
}

// GroundingElement is a per-element view of the source grounding state. It is
// used to surface the next element that needs a source link.
type GroundingElement struct {
	Ref            string
	Name           string
	Kind           string
	Views          []string
	Depth          int
	Grounded       bool
	Eligible       bool
	CodeindexOwned bool
	External       bool
}

// ViewScore is the source grounding score for a single view.
type ViewScore struct {
	ViewRef    string
	Value      int
	Grounded   int
	Eligible   int
	Ungrounded []string
}

// Option customizes how Analyze and Grounding evaluate a workspace.
type Option func(*warningContext)

// WithCodeindexElementClassifier supplies a predicate that reports whether an
// element was materialized from the codeindex (a row in codeindex_elements).
// Those elements are silently excluded from the ARC205 source-grounding score
// so it reflects user-authored diagrams only. When unset, no elements are
// excluded.
func WithCodeindexElementClassifier(classify func(*workspace.Element) bool) Option {
	return func(ctx *warningContext) {
		ctx.codeindexClassifier = classify
	}
}

type warningRule struct {
	Code        string
	Name        string
	Description string
	Mediation   string
	Level       int
	Check       func(ctx *warningContext, rule warningRule)
}

var warningRules = []warningRule{
	{
		Code:        "ARC001",
		Name:        "High Density",
		Description: "View exceeds the element density limit",
		Mediation:   "Split the view into nested views to reduce cognitive load.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			for viewRef, elements := range ctx.viewElements {
				densityLimit := 20
				if ctx.level >= 2 {
					densityLimit = 15
				}
				if len(elements) > densityLimit {
					ctx.addWarning(rule.Code, fmt.Sprintf("View %q has %d elements", viewRef, len(elements)))
				}
			}
		},
	},
	{
		Code:        "ARC002",
		Name:        "Isolated Element",
		Description: "Element has 0 connectors in a view",
		Mediation:   "Explore its relationships further and add connectors in the view where it appears.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			if ctx.isSingleSystemRootContext() {
				return
			}
			for elementRef, element := range ctx.ws.Elements {
				if element == nil || len(element.Placements) != 1 {
					continue
				}
				for _, placement := range element.Placements {
					viewRef := normalizeWarningViewRef(placement.ParentRef)
					if ctx.elementViews[elementRef][viewRef] == 0 {
						ctx.addWarning(rule.Code, fmt.Sprintf("Element %q in View %q", elementRef, viewRef))
					}
				}
			}
		},
	},
	{
		Code:        "ARC003",
		Name:        "Shared Context",
		Description: "Shared element has no connectors in a specific view",
		Mediation:   "Add connectors to the shared element in this view or remove the placement from this view.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element == nil || len(element.Placements) <= 1 {
					continue
				}
				for _, placement := range element.Placements {
					viewRef := normalizeWarningViewRef(placement.ParentRef)
					if ctx.elementViews[elementRef][viewRef] == 0 {
						ctx.addWarning(rule.Code, fmt.Sprintf("Element %q in View %q", elementRef, viewRef))
					}
				}
			}
		},
	},
	{
		Code:        "ARC004",
		Name:        "Depth Mismatch",
		Description: "View hierarchy is flat",
		Mediation:   "Create nested views to establish a zoomable hierarchy.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			viewCount := 0
			for _, element := range ctx.ws.Elements {
				if element != nil && element.HasView {
					viewCount++
				}
			}
			if ctx.maxDepth < 1 && viewCount > 1 {
				ctx.addWarning(rule.Code, "Workspace views")
			}
		},
	},
	{
		Code:        "ARC005",
		Name:        "Low Insight Ratio",
		Description: "Connectors < Elements",
		Mediation:   "Add more connectors to illustrate how elements interact, rather than just listing them.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			if ctx.allowLowInsight {
				return
			}
			allowSingleSystemRootContext := ctx.isSingleSystemRootContext()
			for viewRef, elements := range ctx.viewElements {
				if len(elements) == 0 {
					continue
				}
				if allowSingleSystemRootContext && viewRef == syntheticRootViewRef {
					continue
				}

				// Information-heavy views (types, structs, interfaces) often have low connectivity.
				// If more than 80% of elements are structural, we exempt the view.
				structuralCount := 0
				for _, ref := range elements {
					if el := ctx.ws.Elements[ref]; el != nil {
						kind := strings.ToLower(el.Kind)
						if kind == "struct" || kind == "interface" || kind == "type" || kind == "file" || kind == "folder" {
							structuralCount++
						}
					}
				}
				if structuralCount > len(elements)*8/10 {
					continue
				}

				connectorCount := ctx.viewConnectors[viewRef]
				if connectorCount*2 < len(elements) {
					ctx.addWarning(rule.Code, fmt.Sprintf("View %q (Elements: %d, Connectors: %d)", viewRef, len(elements), connectorCount))
				}
			}
		},
	},
	{
		Code:        "ARC006",
		Name:        "Dead-End Drilldown",
		Description: "Element owns a view but it has no content",
		Mediation:   "Add nested elements or connectors to the element's view.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			for ref, element := range ctx.ws.Elements {
				if element == nil || !element.HasView {
					continue
				}
				if len(ctx.viewElements[ref]) == 0 && ctx.viewConnectors[ref] == 0 {
					ctx.addWarning(rule.Code, fmt.Sprintf("Element %q owns an empty view", ref))
				}
			}
		},
	},
	{
		Code:        "ARC007",
		Name:        "Abstraction Leak",
		Description: "Implementation types at Root level",
		Mediation:   "Move functions and classes into sub-diagrams. Keep root views at the Service/Subsystem level.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element == nil {
					continue
				}
				isRootLevel := len(element.Placements) == 0
				for _, placement := range element.Placements {
					if normalizeWarningViewRef(placement.ParentRef) == syntheticRootViewRef {
						isRootLevel = true
						break
					}
				}
				if isRootLevel {
					lowerType := strings.ToLower(element.Kind)
					if lowerType == "function" || lowerType == "class" {
						ctx.addWarning(rule.Code, fmt.Sprintf("Element %q (Kind: %s)", elementRef, element.Kind))
					}
				}
			}
		},
	},
	{
		Code:        "ARC102",
		Name:        "Missing Tech",
		Description: "No `technology` field",
		Mediation:   "Add a 'technology' field to the following elements. (e.g. Go, React)",
		Level:       2,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element != nil && element.Technology == "" {
					// Repositories are structural and don't require a specific technology tag.
					if strings.ToLower(element.Kind) == "repository" {
						continue
					}
					ctx.addWarning(rule.Code, fmt.Sprintf("%q", elementRef))
				}
			}
		},
	},
	{
		Code:        "ARC103",
		Name:        "Unknown Technology",
		Description: "Catalog mismatch",
		Mediation:   "Use recognized technology names (e.g. Go, React) or double check spelling.",
		Level:       2,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element == nil || element.Technology == "" {
					continue
				}
				missing := tech.Validate(element.Technology)
				if len(missing) > 0 {
					ctx.addWarning(rule.Code, fmt.Sprintf("Element %q has unknown: %s", elementRef, strings.Join(missing, ", ")))
				}
			}
		},
	},
	{
		Code:        "ARC201",
		Name:        "Missing Desc",
		Description: "`description` field is empty",
		Mediation:   "Add a one-sentence summary to help readers understand the responsibility.",
		Level:       3,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element != nil && element.Description == "" {
					ctx.addWarning(rule.Code, fmt.Sprintf("Element %q", elementRef))
				}
			}
		},
	},
	{
		Code:        "ARC202",
		Name:        "Generic Naming",
		Description: "Vague names make the map harder to understand",
		Mediation:   "Rename the element with a domain-specific, descriptive name.",
		Level:       3,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element != nil && isGenericName(element.Name) {
					ctx.addWarning(rule.Code, fmt.Sprintf("Element %q (Name: %q)", elementRef, element.Name))
				}
			}
		},
	},
	{
		Code:        "ARC203",
		Name:        "Missing Label",
		Description: "Connector has no `label`",
		Mediation:   "A connector without a label is just a line. Add a 'label' field to tell what it does.",
		Level:       3,
		Check: func(ctx *warningContext, rule warningRule) {
			for connectorRef, connector := range ctx.ws.Connectors {
				if connector != nil && connector.Label == "" {
					ctx.addWarning(rule.Code, fmt.Sprintf("Connector %q in View %q", connectorRef, normalizeWarningViewRef(connector.View)))
				}
			}
		},
	},
	{
		Code:        "ARC204",
		Name:        "Duplicate Name",
		Description: "Multiple elements share the same display name",
		Mediation:   "Rename elements so each display name is unique and unambiguous.",
		Level:       3,
		Check: func(ctx *warningContext, rule warningRule) {
			type nameGroup struct {
				plain     []string
				qualified map[string][]string
			}
			groups := map[string]*nameGroup{}
			for ref, element := range ctx.ws.Elements {
				if element == nil || strings.TrimSpace(element.Name) == "" {
					continue
				}
				group := groups[element.Name]
				if group == nil {
					group = &nameGroup{qualified: map[string][]string{}}
					groups[element.Name] = group
				}
				if identity := elementCodeIdentity(element); identity != "" {
					group.qualified[identity] = append(group.qualified[identity], ref)
				} else {
					group.plain = append(group.plain, ref)
				}
			}
			names := make([]string, 0, len(groups))
			for name := range groups {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				group := groups[name]
				if len(group.plain) > 0 {
					// A hand-authored name is ambiguous with everything else
					// that shares it, code-backed or not.
					sort.Strings(group.plain)
					firstRef := group.plain[0]
					for _, ref := range group.plain[1:] {
						ctx.addWarning(rule.Code, fmt.Sprintf("Element %q (Name: %q, also used by %q)", ref, name, firstRef))
					}
					var qualifiedRefs []string
					for _, refs := range group.qualified {
						qualifiedRefs = append(qualifiedRefs, refs...)
					}
					sort.Strings(qualifiedRefs)
					for _, ref := range qualifiedRefs {
						ctx.addWarning(rule.Code, fmt.Sprintf("Element %q (Name: %q, also used by %q)", ref, name, firstRef))
					}
					continue
				}
				// Code-backed elements sharing a display name are only
				// duplicates when they resolve to the same source location.
				identities := make([]string, 0, len(group.qualified))
				for identity := range group.qualified {
					identities = append(identities, identity)
				}
				sort.Strings(identities)
				for _, identity := range identities {
					refs := group.qualified[identity]
					if len(refs) < 2 {
						continue
					}
					sort.Strings(refs)
					firstRef := refs[0]
					for _, ref := range refs[1:] {
						ctx.addWarning(rule.Code, fmt.Sprintf("Element %q (Name: %q, also used by %q)", ref, name, firstRef))
					}
				}
			}
		},
	},
	{
		Code:        "ARC205",
		Name:        "Low Grounding",
		Description: "View has too few source-linked elements",
		Mediation:   "Link code-backed elements to a `file_path` (and `symbol` when possible). Deliberately abstract or external elements are exempt.",
		Level:       3,
		Check: func(ctx *warningContext, rule warningRule) {
			report := ctx.groundingReport()
			ctx.scores[rule.Code] = &report
			if report.Eligible == 0 {
				return
			}
			if report.Value < groundingMinScore {
				ctx.addWarning(rule.Code, fmt.Sprintf("Workspace is %d/%d grounded (score %d/10)", report.Grounded, report.Eligible, report.Value))
			}
			for _, view := range report.Views {
				if view.Eligible == 0 || view.Value >= groundingMinScore {
					continue
				}
				ctx.addWarning(rule.Code, fmt.Sprintf("View %q is %d/%d grounded (score %d/10)", view.ViewRef, view.Grounded, view.Eligible, view.Value))
			}
		},
	},
}

// externalTag marks an element whose source is documented externally, e.g. a
// repo the codeindex cannot access or a cloud resource. Such elements are
// exempt from the source grounding score.
const externalTag = "external"

// hasExternalTag reports whether the element carries the external marker tag.
func hasExternalTag(element *workspace.Element) bool {
	if element == nil {
		return false
	}
	for _, tag := range element.Tags {
		if strings.EqualFold(strings.TrimSpace(tag), externalTag) {
			return true
		}
	}
	return false
}

// elementHasSourceLink reports whether an element is linked to a source file or
// symbol. Repository-only links (repo/repository_id without a path) do not
// count as source links.
func elementHasSourceLink(element *workspace.Element) bool {
	if element == nil {
		return false
	}
	return strings.TrimSpace(element.FilePath) != "" || strings.TrimSpace(element.Symbol) != ""
}

// isGroundableElement reports whether an element should be counted toward the
// source grounding score. Every element counts: only codeindex-owned elements
// (excluded by the caller) and elements carrying the external tag are ignored.
func isGroundableElement(element *workspace.Element) bool {
	return element != nil
}

func groundingValue(grounded, eligible int) int {
	if eligible <= 0 {
		return 0
	}
	return int(math.Round(10 * float64(grounded) / float64(eligible)))
}

// Grounding computes the 0-10 source grounding score for the workspace and each
// view. It is exported so callers can render the full report on demand, even
// when no warning threshold was crossed.
func Grounding(ws *workspace.Workspace, opts ...Option) WarningScore {
	if ws == nil {
		return WarningScore{}
	}
	ctx := newWarningContext(ws, opts...)
	ctx.prepareData()
	return ctx.groundingReport()
}

// GroundingDetails computes the source grounding score together with a
// per-element breakdown of eligible elements. Elements are ordered by view
// depth (shallowest first), then view ref, then ref, so callers can surface the
// next element that still needs a source link. Elements linked externally are
// excluded (they are exempt), as are codeindex-owned elements.
func GroundingDetails(ws *workspace.Workspace, opts ...Option) (WarningScore, []GroundingElement) {
	if ws == nil {
		return WarningScore{}, nil
	}
	ctx := newWarningContext(ws, opts...)
	ctx.prepareData()
	report := ctx.groundingReport()
	viewDepths := ctx.viewDepthMap()

	elementViews := make(map[string]map[string]bool, len(ctx.ws.Elements))
	for viewRef, refs := range ctx.viewElements {
		for _, ref := range refs {
			if elementViews[ref] == nil {
				elementViews[ref] = make(map[string]bool)
			}
			elementViews[ref][viewRef] = true
		}
	}

	// Depth of the contents of a view: root is 0, a view owned by an element is
	// one level below the view that element sits in.
	depthOfView := func(viewRef string) int {
		if viewRef == syntheticRootViewRef {
			return 0
		}
		return viewDepths[viewRef] + 1
	}

	var details []GroundingElement
	for ref, element := range ctx.ws.Elements {
		if element == nil || ctx.isCodeindexElement(element) || hasExternalTag(element) {
			continue
		}
		views := make([]string, 0, len(elementViews[ref]))
		for viewRef := range elementViews[ref] {
			views = append(views, viewRef)
		}
		// Order views by depth so the first is the shallowest, matching Depth.
		sort.SliceStable(views, func(i, j int) bool {
			di, dj := depthOfView(views[i]), depthOfView(views[j])
			if di != dj {
				return di < dj
			}
			return views[i] < views[j]
		})
		depth := 0
		if len(views) > 0 {
			depth = depthOfView(views[0])
		}
		details = append(details, GroundingElement{
			Ref:      ref,
			Name:     element.Name,
			Kind:     element.Kind,
			Views:    views,
			Depth:    depth,
			Grounded: elementHasSourceLink(element),
			Eligible: true,
		})
	}

	sort.SliceStable(details, func(i, j int) bool {
		if details[i].Depth != details[j].Depth {
			return details[i].Depth < details[j].Depth
		}
		vi, vj := firstView(details[i]), firstView(details[j])
		if vi != vj {
			return vi < vj
		}
		return details[i].Ref < details[j].Ref
	})
	return report, details
}

func firstView(element GroundingElement) string {
	if len(element.Views) > 0 {
		return element.Views[0]
	}
	return ""
}

func (ctx *warningContext) groundingReport() WarningScore {
	var report WarningScore

	for _, element := range ctx.ws.Elements {
		if ctx.isCodeindexElement(element) {
			continue
		}
		if hasExternalTag(element) {
			report.External++
			continue
		}
		if !isGroundableElement(element) {
			continue
		}
		report.Eligible++
		if elementHasSourceLink(element) {
			report.Grounded++
		}
	}
	report.Value = groundingValue(report.Grounded, report.Eligible)

	viewRefs := make([]string, 0, len(ctx.viewElements))
	for viewRef := range ctx.viewElements {
		viewRefs = append(viewRefs, viewRef)
	}
	sort.Slice(viewRefs, func(i, j int) bool {
		if viewRefs[i] == syntheticRootViewRef {
			return true
		}
		if viewRefs[j] == syntheticRootViewRef {
			return false
		}
		return viewRefs[i] < viewRefs[j]
	})

	for _, viewRef := range viewRefs {
		seen := make(map[string]bool)
		var view ViewScore
		view.ViewRef = viewRef
		for _, ref := range ctx.viewElements[viewRef] {
			if seen[ref] {
				continue
			}
			seen[ref] = true
			element := ctx.ws.Elements[ref]
			if ctx.isCodeindexElement(element) {
				continue
			}
			if hasExternalTag(element) {
				continue
			}
			if !isGroundableElement(element) {
				continue
			}
			view.Eligible++
			if elementHasSourceLink(element) {
				view.Grounded++
			} else {
				view.Ungrounded = append(view.Ungrounded, ref)
			}
		}
		sort.Strings(view.Ungrounded)
		view.Value = groundingValue(view.Grounded, view.Eligible)
		report.Views = append(report.Views, view)
	}

	report.Reasoning = groundingReasoning(report)
	return report
}

func groundingReasoning(report WarningScore) []string {
	if report.Eligible == 0 {
		return []string{"No linkable elements found; source grounding does not apply."}
	}
	lines := []string{
		fmt.Sprintf("%d of %d linkable elements are source-linked (score %d/10).", report.Grounded, report.Eligible, report.Value),
	}
	if report.External > 0 {
		lines = append(lines, fmt.Sprintf("%d element(s) documented with an external link were exempt from the score.", report.External))
	}
	var weak []string
	for _, view := range report.Views {
		if view.Eligible == 0 || view.Value >= groundingMinScore {
			continue
		}
		weak = append(weak, fmt.Sprintf("%s (%d/10)", view.ViewRef, view.Value))
	}
	if len(weak) > 0 {
		lines = append(lines, "Views below the grounding floor of "+fmt.Sprintf("%d/10", groundingMinScore)+": "+strings.Join(weak, ", ")+".")
	}
	return lines
}

// elementCodeIdentity returns the stable source location of a code-backed
// element. The mapping pipeline names file elements by basename while keeping
// the full path on file_path, so repeated file names are intentional and only
// collide when they resolve to the same repository, branch and path.
// Hand-authored elements return "" and remain in the duplicate-name scope.
func elementCodeIdentity(element *workspace.Element) string {
	if element == nil {
		return ""
	}
	path := strings.TrimSpace(element.FilePath)
	if path == "" {
		return ""
	}
	repo := strings.TrimSpace(element.RepositoryID)
	if repo == "" {
		repo = strings.TrimSpace(element.Repo)
	}
	return repo + "\x00" + strings.TrimSpace(element.Branch) + "\x00" + path
}

func (ctx *warningContext) isSingleSystemRootContext() bool {
	if ctx == nil || ctx.ws == nil {
		return false
	}
	rootElements := make([]string, 0, len(ctx.ws.Elements))
	for ref, element := range ctx.ws.Elements {
		if element == nil {
			continue
		}
		for _, placement := range element.Placements {
			if normalizeWarningViewRef(placement.ParentRef) == syntheticRootViewRef {
				rootElements = append(rootElements, ref)
				break
			}
		}
	}
	if len(rootElements) != 1 {
		return false
	}
	rootViewConnectorCount := ctx.viewConnectors[syntheticRootViewRef]
	if rootViewConnectorCount != 0 {
		return false
	}
	rootRef := rootElements[0]
	rootElement := ctx.ws.Elements[rootRef]
	if rootElement == nil {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(rootElement.Kind))
	return kind == "system" || kind == "workspace"
}

type warningContext struct {
	ws                  *workspace.Workspace
	level               int
	allowLowInsight     bool
	activeRules         []warningRule
	violations          map[string][]string
	scores              map[string]*WarningScore
	viewElements        map[string][]string
	elementViews        map[string]map[string]int
	viewConnectors      map[string]int
	maxDepth            int
	codeindexClassifier func(*workspace.Element) bool
}

// isCodeindexElement reports whether the element was materialized from the
// codeindex, per the classifier supplied by the caller. With no classifier,
// nothing is classified as codeindex-owned.
func (ctx *warningContext) isCodeindexElement(element *workspace.Element) bool {
	if ctx == nil || ctx.codeindexClassifier == nil || element == nil {
		return false
	}
	return ctx.codeindexClassifier(element)
}

// Rule describes the static metadata for an architectural warning rule.
type Rule struct {
	Code        string
	Name        string
	Description string
	Mediation   string
	Level       int
}

// Rules returns the metadata for every architectural warning rule in
// declaration order.
func Rules() []Rule {
	rules := make([]Rule, 0, len(warningRules))
	for _, r := range warningRules {
		rules = append(rules, Rule{
			Code:        r.Code,
			Name:        r.Name,
			Description: r.Description,
			Mediation:   r.Mediation,
			Level:       r.Level,
		})
	}
	return rules
}

// RuleByCode returns the metadata for a rule code.
func RuleByCode(code string) (Rule, bool) {
	code = normalizeWarningRuleCode(code)
	for _, rule := range Rules() {
		if rule.Code == code {
			return rule, true
		}
	}
	return Rule{}, false
}

// Analyze evaluates the workspace against architectural best practices and
// returns grouped warnings based on the configured strictness level.
func Analyze(ws *workspace.Workspace, opts ...Option) []WarningGroup {
	if ws == nil {
		return nil
	}

	ctx := newWarningContext(ws, opts...)
	ctx.prepareData()
	ctx.checkAll()

	return ctx.toSlice()
}

func newWarningContext(ws *workspace.Workspace, opts ...Option) *warningContext {
	level := ws.Config.Validation.Level
	allowLowInsight := ws.Config.Validation.AllowLowInsight
	includeRules := ws.Config.Validation.IncludeRules
	excludeRules := ws.Config.Validation.ExcludeRules

	ctx := &warningContext{
		ws:              ws,
		level:           level,
		allowLowInsight: allowLowInsight,
		activeRules:     resolveConfiguredWarningRules(level, includeRules, excludeRules),
		violations:      make(map[string][]string),
		scores:          make(map[string]*WarningScore),
		viewElements:    make(map[string][]string),
		elementViews:    make(map[string]map[string]int),
		viewConnectors:  make(map[string]int),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(ctx)
		}
	}
	return ctx
}

func resolveConfiguredWarningRules(level int, includeRules, excludeRules []string) []warningRule {
	includeMap := make(map[string]bool)
	for _, code := range includeRules {
		includeMap[normalizeWarningRuleCode(code)] = true
	}
	excludeMap := make(map[string]bool)
	for _, code := range excludeRules {
		excludeMap[normalizeWarningRuleCode(code)] = true
	}

	var resolved []warningRule
	for _, rule := range warningRules {
		isEnabled := rule.Level <= level
		if includeMap[rule.Code] {
			isEnabled = true
		}
		if excludeMap[rule.Code] {
			isEnabled = false
		}
		if isEnabled {
			resolved = append(resolved, rule)
		}
	}
	return resolved
}

func normalizeWarningRuleCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func (ctx *warningContext) addWarning(code, violation string) {
	ctx.violations[code] = append(ctx.violations[code], violation)
}

func (ctx *warningContext) prepareData() {
	for elementRef, element := range ctx.ws.Elements {
		ctx.elementViews[elementRef] = make(map[string]int)
		for _, placement := range element.Placements {
			viewRef := normalizeWarningViewRef(placement.ParentRef)
			ctx.viewElements[viewRef] = append(ctx.viewElements[viewRef], elementRef)
		}
	}

	for _, connector := range ctx.ws.Connectors {
		viewRef := normalizeWarningViewRef(connector.View)
		ctx.viewConnectors[viewRef]++
		if elementViews, ok := ctx.elementViews[connector.Source]; ok {
			elementViews[viewRef]++
		}
		if elementViews, ok := ctx.elementViews[connector.Target]; ok {
			elementViews[viewRef]++
		}
	}

	ctx.calculateMaxDepth()
}

func (ctx *warningContext) calculateMaxDepth() {
	for _, depth := range ctx.viewDepthMap() {
		if depth > ctx.maxDepth {
			ctx.maxDepth = depth
		}
	}
}

// viewDepthMap returns the nesting depth of every view-owning element, using
// the same scheme as ARC004: a top-level view is 0 and each nested level adds 1.
// The synthetic root is implicit at 0.
func (ctx *warningContext) viewDepthMap() map[string]int {
	memo := make(map[string]int)
	visiting := make(map[string]bool)
	var viewDepth func(string) int
	viewDepth = func(ref string) int {
		if depth, ok := memo[ref]; ok {
			return depth
		}
		if visiting[ref] {
			return 0
		}
		visiting[ref] = true
		maxDepth := 0
		element := ctx.ws.Elements[ref]
		if element != nil {
			for _, placement := range element.Placements {
				parentRef := normalizeWarningViewRef(placement.ParentRef)
				if parentRef == syntheticRootViewRef {
					continue
				}
				depth := 1
				if parentElement, ok := ctx.ws.Elements[parentRef]; ok && parentElement.HasView {
					depth = viewDepth(parentRef) + 1
				}
				if depth > maxDepth {
					maxDepth = depth
				}
			}
		}
		visiting[ref] = false
		memo[ref] = maxDepth
		return maxDepth
	}

	for ref, element := range ctx.ws.Elements {
		if element == nil || !element.HasView {
			continue
		}
		viewDepth(ref)
	}
	return memo
}

func (ctx *warningContext) checkAll() {
	for _, rule := range ctx.activeRules {
		ctx.runWarningRule(rule)
	}
}

func (ctx *warningContext) runWarningRule(rule warningRule) {
	if rule.Check != nil {
		rule.Check(ctx, rule)
	}
}

func normalizeWarningViewRef(ref string) string {
	if ref == "" || ref == "root" {
		return syntheticRootViewRef
	}
	return ref
}

func (ctx *warningContext) toSlice() []WarningGroup {
	var result []WarningGroup
	for _, rule := range warningRules {
		if violations, ok := ctx.violations[rule.Code]; ok {
			result = append(result, WarningGroup{
				RuleCode:    rule.Code,
				RuleName:    rule.Name,
				Description: rule.Description,
				Mediation:   rule.Mediation,
				Violations:  violations,
				Score:       ctx.scores[rule.Code],
			})
		}
	}
	return result
}

func isGenericName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "module") || strings.Contains(lower, "stuff") || strings.Contains(lower, "thing")
}
