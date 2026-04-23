package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RemoveElement removes an element from elements.yaml.
func RemoveElement(dir, ref string) error {
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

// RemoveConnector removes connectors from connectors.yaml where view == view AND source == source AND target == target.
func RemoveConnector(dir, view, source, target string) (int, error) {
	keep := func(m map[string]any) bool {
		return strVal(m, "view") != view ||
			strVal(m, "source") != source ||
			strVal(m, "target") != target
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
		kept := make([]*Connector, 0, len(connectors))
		for _, connector := range connectors {
			if connector == nil {
				continue
			}
			if connector.View == view && connector.Source == source && connector.Target == target {
				removedKeys = append(removedKeys, ConnectorKey(connector))
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
				delete(items, key)
			}
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
