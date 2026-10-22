package plans

import (
	"fmt"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

func Project(base Graph, s *State, radius uint32, full bool) []*pb.PlanProjectedResource {
	all := Graph{}
	for k, v := range s.Historical {
		all[k] = v
	}
	for k, v := range base {
		all[k] = v
	}
	for k, v := range s.Graph {
		all[k] = v
	}
	distances := map[string]uint32{}
	reasons := map[string]string{}
	for k := range s.Members {
		distances[k] = 0
		reasons[k] = "member"
	}
	for k, v := range all {
		if (s.Changes != nil && s.Changes[k] != "") || (s.Changes == nil && Hash(base[k]) != Hash(s.Graph[k])) {
			distances[k] = 0
			reasons[k] = "changed"
		}
		if full {
			if _, ok := distances[k]; !ok {
				distances[k] = 0
				reasons[k] = "workspace"
			}
		}
		_ = v
	}
	// Expand only graph relationships, never ownership/placement containment.
	for step := uint32(1); step <= radius; step++ {
		add := map[string]bool{}
		for _, g := range []Graph{base, s.Graph} {
			for k, v := range g {
				c := v.GetConnector()
				if c == nil {
					continue
				}
				a, b := g.Find("element", c.SourceElementId), g.Find("element", c.TargetElementId)
				da, oa := distances[a]
				db, ob := distances[b]
				if (oa && da < step) || (ob && db < step) {
					add[a] = true
					add[b] = true
					add[k] = true
				}
			}
		}
		for k := range add {
			if _, ok := distances[k]; !ok {
				distances[k] = step
				reasons[k] = "neighbor"
			}
		}
	}
	// Endpoint/ancestor closure provides navigation without expanding scope.
	for changed := true; changed; {
		changed = false
		for k, d := range distances {
			r := all[k]
			if r == nil {
				continue
			}
			for _, dep := range Dependencies(all, r) {
				if _, ok := distances[dep]; !ok {
					distances[dep] = d
					reasons[dep] = "reference"
					changed = true
				}
			}
			if r.GetElement() != nil {
				for pk, p := range all {
					if pl := p.GetPlacement(); pl != nil && pl.ElementId == ID(r) {
						if _, ok := distances[pk]; !ok {
							distances[pk] = d
							reasons[pk] = "placement"
							changed = true
						}
					}
				}
			}
		}
	}
	keys := []string{}
	for k := range distances {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := []*pb.PlanProjectedResource{}
	for _, k := range keys {
		r := s.Graph[k]
		before := base[k]
		if s.Changes != nil {
			if original, ok := s.Before[k]; ok {
				before = original
			}
		}
		if before == nil && s.Graph[k] == nil {
			before = s.Historical[k]
		}
		if r == nil {
			r = before
		}
		if r == nil {
			continue
		}
		change := ""
		switch {
		case before == nil:
			change = "create"
		case s.Graph[k] == nil:
			change = "remove"
		case Hash(before) != Hash(r):
			change = "modify"
		}
		if s.Changes != nil {
			change = s.Changes[k]
		}
		v := &pb.PlanProjectedResource{Resource: r, Before: before, Change: change, Distance: distances[k], InclusionReason: reasons[k]}
		if change != "" {
			v.Tags = append(v.Tags, "plan:"+change)
		}
		if m := s.Members[k]; m != nil {
			v.Role = m.Role
			v.Explicit = m.Explicit
			if m.Explicit {
				v.Tags = append(v.Tags, "plan:include")
			}
			if m.Role == "preserve" || m.Role == "context" {
				v.Tags = append(v.Tags, "plan:"+m.Role)
			}
		}
		out = append(out, v)
	}
	return out
}
func Label(r *pb.PlanResource) string {
	switch {
	case r == nil:
		return "missing"
	case r.GetElement() != nil:
		return r.GetElement().Name
	case r.GetView() != nil:
		return r.GetView().Name
	case r.GetConnector() != nil:
		return r.GetConnector().GetLabel()
	case r.GetLayer() != nil:
		return r.GetLayer().Name
	}
	return r.Key
}

// Describe resolves display names without exposing storage keys.
func Describe(g Graph, r *pb.PlanResource) string {
	if r == nil {
		return "Missing object"
	}
	name := func(kind string, id int32) string {
		v := g[g.Find(kind, id)]
		if v == nil {
			return "Missing " + kind
		}
		return Label(v)
	}
	if c := r.GetConnector(); c != nil {
		arrow := " → "
		switch c.Direction {
		case "backward":
			arrow = " ← "
		case "both":
			arrow = " ↔ "
		case "none":
			arrow = " — "
		}
		text := name("element", c.SourceElementId) + arrow + name("element", c.TargetElementId)
		if label := strings.TrimSpace(c.GetLabel()); label != "" {
			text += " (" + label + ")"
		}
		return text
	}
	if p := r.GetPlacement(); p != nil {
		return name("element", p.ElementId) + " in " + name("view", p.ViewId)
	}
	if label := strings.TrimSpace(Label(r)); label != "" && label != r.Key {
		return label
	}
	return "Unnamed " + Kind(r)
}
func ShellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }
func Prompt(p *pb.Plan, base Graph, s *State) string {
	var b strings.Builder
	graph := Graph{}
	for k, v := range s.Historical {
		graph[k] = v
	}
	for k, v := range base {
		graph[k] = v
	}
	for k, v := range s.Graph {
		graph[k] = v
	}
	selector := ShellQuote(p.Name)
	fmt.Fprintf(&b, "Implement the code changes described by plan %s, revision %d, in its associated repository. Deliver working source changes and tests.\n\n", p.Name, p.Revision)
	if objective := strings.TrimSpace(p.Objective); objective != "" {
		fmt.Fprintf(&b, "Objective: %s\n\n", objective)
	}
	b.WriteString("The diagram is the specification already authored by the user. Scope entries describe recorded diagram changes and the software intent they represent; they are not commands to recreate nodes or connectors. Diagram edits alone do not implement this plan.\n\n")
	fmt.Fprintf(&b, "Read the plan with `tld plan show %s` and inspect context with `tld plan show %s --radius 1`. Use the plan’s configured workspace and repository.\n\n", selector, selector)
	b.WriteString("Scope (translate into code behavior and relationships):\n")
	projection := Project(base, s, 0, false)
	// Fold matching lifecycle placements into their element, retaining view context.
	elements := map[int32]*pb.PlanProjectedResource{}
	for _, r := range projection {
		if r.Resource.GetElement() != nil && (r.Change == "create" || r.Change == "remove") {
			elements[ID(r.Resource)] = r
		}
	}
	views := map[string][]string{}
	folded := map[string]bool{}
	for _, r := range projection {
		if placement := r.Resource.GetPlacement(); placement != nil {
			e := elements[placement.ElementId]
			if e != nil && e.Change == r.Change && e.Role == r.Role {
				view := Describe(graph, graph[graph.Find("view", placement.ViewId)])
				views[e.Resource.Key] = append(views[e.Resource.Key], view)
				folded[r.Resource.Key] = true
			}
		}
	}
	for _, r := range projection {
		if r.InclusionReason != "member" && r.InclusionReason != "changed" {
			continue
		}
		if folded[r.Resource.Key] {
			continue
		}
		action := r.Change
		if action == "" {
			action = r.Role
		}
		label := Describe(graph, r.Resource)
		if names := views[r.Resource.Key]; len(names) > 0 {
			sort.Strings(names)
			label += " in " + strings.Join(names, ", ")
		}
		// Change is implied by create/modify/remove; retain differing constraints.
		if r.Role != "" && r.Role != "change" && r.Role != action {
			label += " (" + r.Role + ")"
		}
		fmt.Fprintf(&b, "- %s: %s\n", action, label)
		if e := r.Resource.GetElement(); e != nil {
			if e.GetTechnology() != "" {
				fmt.Fprintf(&b, "  Technology: %s\n", e.GetTechnology())
			}
			if e.GetDescription() != "" {
				fmt.Fprintf(&b, "  Details: %s\n", e.GetDescription())
			}
			if e.GetRepo() != "" || e.GetFilePath() != "" || e.GetRepositoryId() != "" {
				fmt.Fprintf(&b, "  Source: %s %s @ %s\n", e.GetRepo(), e.GetFilePath(), e.GetBranch())
			}
		}
		if c := r.Resource.GetConnector(); c != nil {
			if c.GetDescription() != "" {
				fmt.Fprintf(&b, "  Details: %s\n", c.GetDescription())
			}
			if c.GetRelationship() != "" {
				fmt.Fprintf(&b, "  Relationship: %s\n", c.GetRelationship())
			}
			if c.GetUrl() != "" {
				fmt.Fprintf(&b, "  Link: %s\n", c.GetUrl())
			}
		}
	}
	if len(s.Checklist) > 0 {
		b.WriteString("\nRequirements:\n")
		keys := []string{}
		for k := range s.Checklist {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			item := s.Checklist[k]
			fmt.Fprintf(&b, "- %s (%s)\n", item.Text, item.VerificationKind)
		}
	}
	notes := map[string]string{}
	for _, r := range Project(base, s, 0, false) {
		if v := r.Resource.GetView(); v != nil && v.Markdown != nil {
			notes[r.Resource.Key] = "linked note"
		}
	}
	for k := range s.Notes {
		notes[k] = "linked note"
		if original := s.Notes[k].OriginalPath; original != "" {
			notes[k] = "linked note"
		}
	}
	if len(notes) > 0 {
		b.WriteString("\nView notes:\n")
		ks := []string{}
		for k := range notes {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		for _, k := range ks {
			name := Describe(graph, graph[k])
			fmt.Fprintf(&b, "- %s (%s): `tld plan note %s %s`\n", name, notes[k], selector, ShellQuote(name))
		}
	}
	b.WriteString("\nImplementation workflow:\n")
	b.WriteString("1. Read the scoped plan, linked notes, and relevant repository source and tests. Use tld for missing architectural context, then work in the codebase. A diagram-authoring skill is only relevant for later diagram maintenance.\n")
	b.WriteString("2. Translate the scope into concrete code behavior and acceptance checks. Follow existing repository conventions for routine details and state assumptions. Ask only when a missing decision materially changes behavior; do not substitute diagram work for unresolved implementation. Preserve listed constraints; context objects are supporting information, not extra scope. Shared elements retain one identity across views; placement does not imply runtime ownership.\n")
	b.WriteString("3. Implement the feature in source files, add or update relevant tests, and run the repository's required checks. Continue through implementation and validation rather than stopping after inspecting or updating tld.\n")
	b.WriteString("4. After code and tests, update the diagram only where the implementation adds useful detail or differs from the specification. Reuse existing objects and relationships. When implementation is locked, ordinary tld mutations automatically stage these supporting changes for user review in Watch. Staged diagram changes are supplementary proof-of-work, not evidence that code works.\n")
	b.WriteString("5. Report changed source files, implemented behavior, test commands and results, and remaining gaps against the scope and requirements. Do not claim completion based on diagram updates or claim unverifiable requirements are proven.\n")

	return b.String()
}
