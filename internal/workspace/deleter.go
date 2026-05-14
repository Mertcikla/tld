package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// RemoveElement removes an element from elements.yaml.
func RemoveElement(dir, ref string) error {
	if err := ValidateElementRef(ref); err != nil {
		return err
	}
	ws, err := Load(dir)
	if err != nil {
		return err
	}
	if _, ok := ws.Elements[ref]; !ok {
		return fmt.Errorf("element %q not found", ref)
	}
	if blockers := elementRemovalBlockers(ws, ref); len(blockers) > 0 {
		return fmt.Errorf("element %q is still referenced:\n  - %s\nRemove or update these references first", ref, strings.Join(blockers, "\n  - "))
	}
	removed, err := filterYAMLMap(filepath.Join(dir, "elements.yaml"), func(k string, _ any) bool { return k != ref })
	if err != nil {
		return err
	}
	if !removed {
		return nil
	}
	if err := DeleteCurrentElementMetadataEntries(dir, ref); err != nil {
		return err
	}
	return DeleteCurrentViewMetadataEntries(dir, ref)
}

func elementRemovalBlockers(ws *Workspace, ref string) []string {
	var blockers []string
	for elementRef, element := range ws.Elements {
		if element == nil {
			continue
		}
		for index, placement := range element.Placements {
			if placement.ParentRef == ref {
				blockers = append(blockers, fmt.Sprintf("elements.yaml[%s].placements[%d].parent", elementRef, index))
			}
		}
	}
	for key, connector := range ws.Connectors {
		if connector == nil {
			continue
		}
		if connector.View == ref {
			blockers = append(blockers, fmt.Sprintf("connectors.yaml[%s].view", key))
		}
		if connector.Source == ref {
			blockers = append(blockers, fmt.Sprintf("connectors.yaml[%s].source", key))
		}
		if connector.Target == ref {
			blockers = append(blockers, fmt.Sprintf("connectors.yaml[%s].target", key))
		}
	}
	sort.Strings(blockers)
	return blockers
}

// RemoveConnector removes connectors from connectors.yaml where view == view AND source == source AND target == target.
func RemoveConnector(dir, view, source, target string) (int, error) {
	return RemoveConnectorWithLabel(dir, view, source, target, "")
}

// RemoveConnectorWithLabel removes matching connectors. Without a label, an
// ambiguous multi-match is refused so users do not delete more than intended.
func RemoveConnectorWithLabel(dir, view, source, target, label string) (int, error) {
	if err := ValidateParentRef(view); err != nil {
		return 0, fmt.Errorf("invalid view ref: %w", err)
	}
	if err := ValidateElementRef(source); err != nil {
		return 0, fmt.Errorf("invalid source ref: %w", err)
	}
	if err := ValidateElementRef(target); err != nil {
		return 0, fmt.Errorf("invalid target ref: %w", err)
	}
	keep := func(m map[string]any) bool {
		return !connectorMatches(strVal(m, "view"), strVal(m, "source"), strVal(m, "target"), strVal(m, "label"), view, source, target, label)
	}
	path := filepath.Join(dir, "connectors.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, nil // file absent is fine
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(root.Content) == 0 {
		return 0, nil
	}

	removedKeys := make([]string, 0)
	switch root.Content[0].Kind {
	case yaml.SequenceNode:
		var connectors []*Connector
		if err := root.Content[0].Decode(&connectors); err != nil {
			return 0, fmt.Errorf("parse %s: %w", path, err)
		}
		for _, connector := range connectors {
			if connector == nil {
				continue
			}
			if connectorMatches(connector.View, connector.Source, connector.Target, connector.Label, view, source, target, label) {
				removedKeys = append(removedKeys, ConnectorKey(connector))
			}
		}
		if err := refuseAmbiguousConnectorRemoval(removedKeys, label); err != nil {
			return 0, err
		}
		kept := make([]*Connector, 0, len(connectors))
		for _, connector := range connectors {
			if connector == nil || connectorMatches(connector.View, connector.Source, connector.Target, connector.Label, view, source, target, label) {
				continue
			}
			kept = append(kept, connector)
		}
		if len(removedKeys) == 0 {
			return 0, nil
		}
		if err := WriteFullYAMLList(path, kept); err != nil {
			return 0, err
		}
	case yaml.MappingNode:
		var items map[string]any
		if err := root.Content[0].Decode(&items); err != nil {
			return 0, fmt.Errorf("parse %s: %w", path, err)
		}
		for key, value := range items {
			if key == "_meta" || key == "_meta_connectors" || key == "connectors" {
				continue
			}
			if mapped, ok := value.(map[string]any); ok && !keep(mapped) {
				removedKeys = append(removedKeys, key)
			}
		}
		if err := refuseAmbiguousConnectorRemoval(removedKeys, label); err != nil {
			return 0, err
		}
		for _, key := range removedKeys {
			delete(items, key)
		}
		if len(removedKeys) == 0 {
			return 0, nil
		}
		out, err := yaml.Marshal(items)
		if err != nil {
			return 0, fmt.Errorf("marshal %s: %w", path, err)
		}
		if err := os.WriteFile(path, out, 0600); err != nil {
			return 0, fmt.Errorf("write %s: %w", path, err)
		}
	default:
		return 0, fmt.Errorf("parse %s: expected list or mapping document", path)
	}
	if err := DeleteCurrentConnectorMetadataEntries(dir, removedKeys...); err != nil {
		return 0, err
	}
	return len(removedKeys), nil
}

func connectorMatches(view, source, target, connectorLabel, wantView, wantSource, wantTarget, wantLabel string) bool {
	if view != wantView || source != wantSource || target != wantTarget {
		return false
	}
	return wantLabel == "" || connectorLabel == wantLabel
}

func refuseAmbiguousConnectorRemoval(keys []string, label string) error {
	if label != "" || len(keys) <= 1 {
		return nil
	}
	sort.Strings(keys)
	return fmt.Errorf("multiple connectors match; specify --label. Matches:\n  - %s", strings.Join(keys, "\n  - "))
}

// filterYAMLMap reads path as map[string]any, keeps only items where keep(key, val)==true,
// writes back, and returns error if key not found or write fails.
func filterYAMLMap(path string, keep func(string, any) bool) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, nil // file absent is fine
	}

	var items map[string]any
	if err := yaml.Unmarshal(data, &items); err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}

	before := len(items)
	kept := make(map[string]any)
	for k, v := range items {
		if keep(k, v) {
			kept[k] = v
		}
	}

	if len(kept) == before {
		return false, nil
	}

	out, err := yaml.Marshal(kept)
	if err != nil {
		return false, fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
