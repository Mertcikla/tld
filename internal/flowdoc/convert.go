package flowdoc

import (
	"fmt"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"gopkg.in/yaml.v3"
)

func Import(doc *Document, viewID int32, diagram *diagv1.ViewContent, catalog Catalog) (*diagv1.Flow, error) {
	element := func(name string) (int32, error) {
		var id int32
		count := 0
		for _, e := range diagram.Placements {
			if e.Name == name {
				id = e.ElementId
				count++
			}
		}
		if count != 1 {
			return 0, fmt.Errorf("element %q is missing or ambiguous in the target diagram", name)
		}
		return id, nil
	}
	var convert func([]Step) ([]*diagv1.FlowStep, error)
	convert = func(steps []Step) ([]*diagv1.FlowStep, error) {
		result := []*diagv1.FlowStep{}
		for _, s := range steps {
			item := &diagv1.FlowStep{Id: fresh(s.ID), Title: s.Title, Description: s.Description}
			for _, name := range s.Elements {
				id, err := element(name)
				if err != nil {
					return nil, err
				}
				item.ElementIds = append(item.ElementIds, id)
			}
			for _, c := range s.Connectors {
				source, err := element(c.Source)
				if err != nil {
					return nil, err
				}
				target, err := element(c.Target)
				if err != nil {
					return nil, err
				}
				var id int32
				count := 0
				for _, candidate := range diagram.Connectors {
					if candidate.SourceElementId == source && candidate.TargetElementId == target && (c.Label == nil || candidate.GetLabel() == *c.Label) {
						id = candidate.Id
						count++
					}
				}
				if count != 1 {
					return nil, fmt.Errorf("connection %q → %q is missing or ambiguous; specify a unique label", c.Source, c.Target)
				}
				item.ConnectorIds = append(item.ConnectorIds, id)
			}
			if s.GoToFlow != nil {
				targetView := viewID
				if s.GoToFlow.View != "" {
					count := 0
					for _, v := range catalog.Views {
						if v.Name == s.GoToFlow.View {
							targetView = v.Id
							count++
						}
					}
					if count != 1 {
						return nil, fmt.Errorf("linked diagram %q is missing or ambiguous", s.GoToFlow.View)
					}
				}
				matches := []*diagv1.Flow{}
				for _, f := range catalog.Flows {
					if f.ViewId == targetView && f.GetContent().GetName() == s.GoToFlow.Name {
						matches = append(matches, f)
					}
				}
				if len(matches) != 1 {
					return nil, fmt.Errorf("linked flow %q is missing or ambiguous; qualify its view when importing into a new view", s.GoToFlow.Name)
				}
				item.Control = &diagv1.FlowStep_GoToFlowId{GoToFlowId: matches[0].Id}
			}
			if s.Branches != nil {
				branches := &diagv1.FlowBranches{Mode: diagv1.FlowBranchMode_FLOW_BRANCH_MODE_ALTERNATE}
				if s.Branches.Mode == "parallel" {
					branches.Mode = diagv1.FlowBranchMode_FLOW_BRANCH_MODE_PARALLEL
				}
				for _, p := range s.Branches.Paths {
					children, err := convert(p.Steps)
					if err != nil {
						return nil, err
					}
					branches.Paths = append(branches.Paths, &diagv1.FlowBranch{Id: fresh(p.ID), Name: p.Name, Steps: children})
				}
				item.Control = &diagv1.FlowStep_Branches{Branches: branches}
			}
			result = append(result, item)
		}
		return result, nil
	}
	steps, err := convert(doc.Steps)
	if err != nil {
		return nil, err
	}
	result := &diagv1.Flow{ViewId: viewID, Content: &diagv1.FlowContent{Name: doc.Name, Description: doc.Description, Steps: steps}}
	if err = ValidateSave(result); err != nil {
		return nil, err
	}
	return result, nil
}

func Export(flow *diagv1.Flow, diagram *diagv1.ViewContent, catalog Catalog, complete bool) ([]byte, error) {
	doc := Document{Format: "tld-flow", Version: 1, Name: flow.GetContent().GetName(), Description: flow.GetContent().GetDescription()}
	elements := map[int32]*diagv1.PlacedElement{}
	names := map[string]int{}
	connectors := map[int32]*diagv1.Connector{}
	for _, e := range diagram.Placements {
		elements[e.ElementId] = e
		names[e.Name]++
	}
	for _, c := range diagram.Connectors {
		connectors[c.Id] = c
	}
	usedElements := map[int32]bool{}
	usedConnectors := map[int32]bool{}
	elementName := func(id int32) (string, error) {
		e := elements[id]
		if e == nil {
			return "", fmt.Errorf("element %d is unavailable; relink the step before exporting", id)
		}
		if names[e.Name] != 1 {
			return "", fmt.Errorf("element name %q is ambiguous; give it a unique name before exporting", e.Name)
		}
		usedElements[id] = true
		return e.Name, nil
	}
	var convert func([]*diagv1.FlowStep) ([]Step, error)
	convert = func(steps []*diagv1.FlowStep) ([]Step, error) {
		result := []Step{}
		for _, s := range steps {
			item := Step{ID: s.Id, Title: s.Title, Description: s.Description}
			for _, id := range s.ElementIds {
				name, err := elementName(id)
				if err != nil {
					return nil, err
				}
				item.Elements = append(item.Elements, name)
			}
			for _, id := range s.ConnectorIds {
				c := connectors[id]
				if c == nil {
					return nil, fmt.Errorf("connection %d is unavailable; relink the step before exporting", id)
				}
				source, err := elementName(c.SourceElementId)
				if err != nil {
					return nil, err
				}
				target, err := elementName(c.TargetElementId)
				if err != nil {
					return nil, err
				}
				count := 0
				for _, other := range diagram.Connectors {
					if other.SourceElementId == c.SourceElementId && other.TargetElementId == c.TargetElementId && other.GetLabel() == c.GetLabel() {
						count++
					}
				}
				if count != 1 {
					return nil, fmt.Errorf("connections with the same endpoints need distinct labels before exporting")
				}
				label := c.GetLabel()
				item.Connectors = append(item.Connectors, Connection{Source: source, Target: target, Label: &label})
				usedConnectors[id] = true
			}
			if id := s.GetGoToFlowId(); id != "" {
				var target *diagv1.Flow
				for _, f := range catalog.Flows {
					if f.Id == id {
						target = f
					}
				}
				if target == nil {
					return nil, fmt.Errorf("linked flow %q is unavailable", id)
				}
				count := 0
				for _, f := range catalog.Flows {
					if f.ViewId == target.ViewId && f.GetContent().GetName() == target.GetContent().GetName() {
						count++
					}
				}
				if count != 1 {
					return nil, fmt.Errorf("linked flow name is ambiguous")
				}
				item.GoToFlow = &Link{Name: target.Content.Name}
				if target.ViewId != flow.ViewId {
					name := ""
					for _, v := range catalog.Views {
						if v.Id == target.ViewId {
							name = v.Name
						}
					}
					count = 0
					for _, v := range catalog.Views {
						if v.Name == name {
							count++
						}
					}
					if name == "" || count != 1 {
						return nil, fmt.Errorf("linked diagram name is missing or ambiguous")
					}
					item.GoToFlow.View = name
				}
			}
			if b := s.GetBranches(); b != nil {
				mode := "alternate"
				if b.Mode == diagv1.FlowBranchMode_FLOW_BRANCH_MODE_PARALLEL {
					mode = "parallel"
				}
				item.Branches = &Branches{Mode: mode, Paths: []Path{}}
				for _, p := range b.Paths {
					children, err := convert(p.Steps)
					if err != nil {
						return nil, err
					}
					item.Branches.Paths = append(item.Branches.Paths, Path{ID: p.Id, Name: p.Name, Steps: children})
				}
			}
			result = append(result, item)
		}
		return result, nil
	}
	steps, err := convert(flow.GetContent().GetSteps())
	if err != nil {
		return nil, err
	}
	doc.Steps = steps
	if complete {
		doc.Resources = &Resources{Elements: []Element{}, Connectors: []Connector{}}
		for _, e := range diagram.Placements {
			if !usedElements[e.ElementId] {
				continue
			}
			x, y := e.PositionX, e.PositionY
			item := Element{Name: e.Name, Kind: e.GetKind(), Description: e.GetDescription(), Technology: e.GetTechnology(), URL: e.GetUrl(), LogoURL: e.GetLogoUrl(), Tags: e.Tags, X: &x, Y: &y}
			for _, link := range e.TechnologyLinks {
				item.TechnologyConnectors = append(item.TechnologyConnectors, Technology{Type: link.Type, Slug: link.GetSlug(), Label: link.Label, Primary: link.GetIsPrimaryIcon()})
			}
			doc.Resources.Elements = append(doc.Resources.Elements, item)
		}
		for _, c := range diagram.Connectors {
			if !usedConnectors[c.Id] {
				continue
			}
			label := c.GetLabel()
			doc.Resources.Connectors = append(doc.Resources.Connectors, Connector{Source: elements[c.SourceElementId].Name, Target: elements[c.TargetElementId].Name, Label: &label, Description: c.GetDescription(), Relationship: c.GetRelationship(), Direction: c.Direction, Style: c.Style, URL: c.GetUrl(), SourceHandle: c.GetSourceHandle(), TargetHandle: c.GetTargetHandle(), Tags: c.Tags})
		}
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return nil, err
	}
	data = append([]byte("# yaml-language-server: $schema="+SchemaURL+"\n"), data...)
	if _, err = Parse(data); err != nil {
		return nil, err
	}
	return data, nil
}

// PreviewResources resolves bundle references against temporary IDs before any writes.
func PreviewResources(resources *Resources, viewID int32) *diagv1.ViewContent {
	result := &diagv1.ViewContent{}
	ids := map[string]int32{}
	for i, e := range resources.Elements {
		id := int32(i + 1)
		ids[e.Name] = id
		result.Placements = append(result.Placements, &diagv1.PlacedElement{ElementId: id, Name: e.Name})
	}
	for i, c := range resources.Connectors {
		result.Connectors = append(result.Connectors, &diagv1.Connector{Id: int32(i + 1), ViewId: viewID, SourceElementId: ids[c.Source], TargetElementId: ids[c.Target], Label: c.Label})
	}
	return result
}
