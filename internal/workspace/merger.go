package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var positionKeys = map[string]bool{
	"position_x": true,
	"position_y": true,
}

// MergeWorkspace merges changes from a new workspace (from server) into the current on-disk state.
// It uses yaml.Node to preserve comments and formatting.
// lastSyncMeta is the metadata from the .tld.lock file (state at last pull/apply).
// currentMeta is the metadata loaded from local YAML files (current state on disk).
func MergeWorkspace(dir string, newWS *Workspace, lastSyncMeta *Meta, currentMeta *Meta) error {
	if useElementWorkspaceFiles(newWS) {
		var elementMeta map[string]*ResourceMetadata
		var viewMeta map[string]*ResourceMetadata
		var connectorMeta map[string]*ResourceMetadata
		if newWS.Meta != nil {
			elementMeta = newWS.Meta.Elements
			viewMeta = newWS.Meta.Views
			connectorMeta = newWS.Meta.Connectors
		}

		storedViewMeta, err := PersistCurrentViewMetadata(dir, viewMeta)
		if err != nil {
			return fmt.Errorf("persist current view metadata: %w", err)
		}
		storedConnectorMeta, err := PersistCurrentConnectorMetadata(dir, connectorMeta)
		if err != nil {
			return fmt.Errorf("persist current connector metadata: %w", err)
		}

		elementMetaSections := []metadataSection{{name: "_meta_elements", values: elementMeta, persist: false}, {name: "_meta_views", values: viewMeta, persist: !storedViewMeta}}
		if err := mergeYAMLMapWithMetadataSections(
			filepath.Join(dir, "elements.yaml"),
			newWS.Elements,
			combinedElementMetadata(newWS.Meta),
			combinedElementMetadata(lastSyncMeta),
			combinedElementMetadata(currentMeta),
			elementMetaSections,
		); err != nil {
			return fmt.Errorf("merge elements: %w", err)
		}

		if err := mergeYAMLMapWithMetadataSections(
			filepath.Join(dir, "connectors.yaml"),
			newWS.Connectors,
			connectorMeta,
			lastSyncMeta.Connectors,
			currentMeta.Connectors,
			[]metadataSection{{name: "_meta_connectors", values: connectorMeta, persist: !storedConnectorMeta}},
		); err != nil {
			return fmt.Errorf("merge connectors: %w", err)
		}

		if err := cleanupLegacyWorkspaceFiles(dir); err != nil {
			return fmt.Errorf("cleanup legacy workspace files: %w", err)
		}
		return nil
	}

	return nil
}

func mergeYAMLMapWithMetadataSections(path string, serverItems any, serverMeta map[string]*ResourceMetadata, lastSyncMeta map[string]*ResourceMetadata, currentMeta map[string]*ResourceMetadata, sections []metadataSection) error {
	// Load existing file into a Node
	var root yaml.Node
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WriteFullYAMLMapSections(path, serverItems, sections)
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("unmarshal %s: %w", path, err)
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return WriteFullYAMLMapSections(path, serverItems, sections)
	}

	mapping := root.Content[0]

	// Convert serverItems to a map for easy lookup
	serverItemsData, _ := yaml.Marshal(serverItems)
	var serverItemsMap map[string]any
	_ = yaml.Unmarshal(serverItemsData, &serverItemsMap)

	seenKeys := make(map[string]bool)
	var newContent []*yaml.Node

	// Iterate through existing mapping (local state)
	for i := 0; i < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		valNode := mapping.Content[i+1]
		key := keyNode.Value

		if isMetadataSectionKey(key, sections) {
			continue
		}

		serverItem, onServer := serverItemsMap[key]
		sMeta := serverMeta[key]
		lMeta := lastSyncMeta[key]
		cMeta := currentMeta[key]

		if onServer {
			// Resource exists locally and on server
			seenKeys[key] = true

			localChanged := lMeta != nil && cMeta != nil && cMeta.UpdatedAt.After(lMeta.UpdatedAt)
			serverChanged := lMeta != nil && sMeta != nil && sMeta.UpdatedAt.After(lMeta.UpdatedAt)

			if localChanged && serverChanged {
				mergedNode, hasConflict, mergeErr := mergeResourceValueNode(valNode, serverItem, key)
				if mergeErr != nil {
					return fmt.Errorf("merge %s[%s]: %w", filepath.Base(path), key, mergeErr)
				}
				if hasConflict {
					for _, section := range sections {
						if section.values[key] != nil {
							section.values[key].Conflict = true
							break
						}
					}
				} else if sMeta != nil {
					sMeta.Conflict = false
				}
				newContent = append(newContent, keyNode, mergedNode)
			} else if serverChanged {
				// Server changed, local did not: Update local with server content.
				var newValNode yaml.Node
				_ = newValNode.Encode(serverItem)
				newContent = append(newContent, keyNode, &newValNode)
			} else {
				// No changes or only local changes: Keep local.
				newContent = append(newContent, keyNode, valNode)
			}
		} else {
			// Resource exists locally but not on server.
			// Was it deleted on server or created locally?
			if lMeta == nil {
				// Was never on server, so it's a new local resource. Keep it.
				newContent = append(newContent, keyNode, valNode)
			}
			// Else: Was on server before (last sync), so it was deleted on server.
			// Remove locally too by NOT adding to newContent.
		}
	}

	// Add new keys from server that weren't in the local file
	for key, val := range serverItemsMap {
		if !seenKeys[key] {
			var keyNode yaml.Node
			_ = keyNode.Encode(key)
			var valNode yaml.Node
			_ = valNode.Encode(val)
			newContent = append(newContent, &keyNode, &valNode)
		}
	}

	for _, section := range sections {
		if !section.persist || len(section.values) == 0 {
			continue
		}
		var metaKeyNode yaml.Node
		_ = metaKeyNode.Encode(section.name)
		metaValNode, err := EncodeMeta(section.values)
		if err != nil {
			return err
		}
		newContent = append(newContent, &metaKeyNode, metaValNode)
	}

	mapping.Content = newContent

	// Write back
	normalizeYAMLStyle(&root)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return nil
}

func isMetadataSectionKey(key string, sections []metadataSection) bool {
	for _, section := range sections {
		if key == section.name {
			return true
		}
	}
	return false
}

func combinedElementMetadata(meta *Meta) map[string]*ResourceMetadata {
	combined := make(map[string]*ResourceMetadata)
	if meta == nil {
		return combined
	}
	for ref, resourceMeta := range meta.Elements {
		if resourceMeta == nil {
			continue
		}
		copyMeta := *resourceMeta
		combined[ref] = &copyMeta
	}
	for ref, resourceMeta := range meta.Views {
		if resourceMeta == nil {
			continue
		}
		existing := combined[ref]
		if existing == nil {
			copyMeta := *resourceMeta
			combined[ref] = &copyMeta
			continue
		}
		if resourceMeta.UpdatedAt.After(existing.UpdatedAt) {
			existing.UpdatedAt = resourceMeta.UpdatedAt
		}
		if existing.ID == 0 {
			existing.ID = resourceMeta.ID
		}
	}
	return combined
}

func mergeResourceValueNode(localNode *yaml.Node, serverItem any, ref string) (*yaml.Node, bool, error) {
	serverNode, err := encodeYAMLValueNode(serverItem)
	if err != nil {
		return nil, false, err
	}
	merged, hasConflict := mergeNodeValues(localNode, serverNode, "")
	if hasConflict && merged.HeadComment == "" {
		merged.HeadComment = fmt.Sprintf("CONFLICT: %q was modified both locally and on the server.\nResolve the marked values, then run `tld apply`.", ref)
	}
	return merged, hasConflict, nil
}

func mergeNodeValues(localNode, serverNode *yaml.Node, fieldName string) (*yaml.Node, bool) {
	if serverNode == nil {
		return cloneYAMLNode(localNode), false
	}
	if localNode == nil {
		return cloneYAMLNode(serverNode), false
	}
	if positionKeys[fieldName] {
		return cloneYAMLNode(serverNode), false
	}

	if localNode.Kind == yaml.MappingNode && serverNode.Kind == yaml.MappingNode {
		return mergeMappingNodes(localNode, serverNode)
	}
	if localNode.Kind == yaml.SequenceNode && serverNode.Kind == yaml.SequenceNode {
		if fieldName == "diagrams" || fieldName == "placements" {
			return mergePlacementSequences(localNode, serverNode, fieldName)
		}
		return cloneYAMLNode(serverNode), false
	}

	localValue := strings.TrimSpace(localNode.Value)
	serverValue := strings.TrimSpace(serverNode.Value)
	if localValue == serverValue {
		return cloneYAMLNode(localNode), false
	}
	if serverValue == "" && localValue != "" {
		return cloneYAMLNode(localNode), false
	}
	if localValue == "" && serverValue != "" {
		return cloneYAMLNode(serverNode), false
	}
	return conflictValueNode(fieldName, localValue, serverValue), true
}

func mergeMappingNodes(localNode, serverNode *yaml.Node) (*yaml.Node, bool) {
	result := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	hasConflict := false
	serverValues := make(map[string]*yaml.Node)
	serverOrder := make([]string, 0, len(serverNode.Content)/2)
	for i := 0; i+1 < len(serverNode.Content); i += 2 {
		serverValues[serverNode.Content[i].Value] = serverNode.Content[i+1]
		serverOrder = append(serverOrder, serverNode.Content[i].Value)
	}
	seen := make(map[string]bool)

	for i := 0; i+1 < len(localNode.Content); i += 2 {
		key := localNode.Content[i].Value
		localVal := localNode.Content[i+1]
		seen[key] = true
		mergedVal := cloneYAMLNode(localVal)
		fieldConflict := false
		if serverVal, ok := serverValues[key]; ok {
			mergedVal, fieldConflict = mergeNodeValues(localVal, serverVal, key)
		}
		result.Content = append(result.Content, cloneYAMLNode(localNode.Content[i]), mergedVal)
		hasConflict = hasConflict || fieldConflict
	}

	for _, key := range serverOrder {
		if seen[key] {
			continue
		}
		result.Content = append(result.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			cloneYAMLNode(serverValues[key]),
		)
	}
	return result, hasConflict
}

func mergePlacementSequences(localNode, serverNode *yaml.Node, fieldName string) (*yaml.Node, bool) {
	result := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	serverValues := make(map[string]*yaml.Node)
	serverOrder := make([]string, 0, len(serverNode.Content))
	for _, item := range serverNode.Content {
		if key := placementSequenceKey(item); key != "" {
			serverValues[key] = item
			serverOrder = append(serverOrder, key)
		}
	}
	seen := make(map[string]bool)
	hasConflict := false

	for _, localItem := range localNode.Content {
		key := placementSequenceKey(localItem)
		seen[key] = true
		if key != "" {
			if serverItem, ok := serverValues[key]; ok {
				mergedItem, conflict := mergeNodeValues(localItem, serverItem, fieldName)
				result.Content = append(result.Content, mergedItem)
				hasConflict = hasConflict || conflict
				continue
			}
		}
		result.Content = append(result.Content, cloneYAMLNode(localItem))
	}

	for _, key := range serverOrder {
		if seen[key] {
			continue
		}
		result.Content = append(result.Content, cloneYAMLNode(serverValues[key]))
	}
	return result, hasConflict
}

func placementSequenceKey(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "diagram", "parent":
			return node.Content[i+1].Value
		}
	}
	return ""
}

func conflictValueNode(fieldName, localValue, serverValue string) *yaml.Node {
	comment := fmt.Sprintf("CONFLICT: '%s' was modified both locally and on the server.\nLocal:  %s\nServer: %s\nResolve by keeping one value, then run `tld apply`.", fieldName, localValue, serverValue)
	return &yaml.Node{
		Kind:        yaml.ScalarNode,
		Tag:         "!!str",
		Style:       yaml.LiteralStyle,
		HeadComment: comment,
		Value:       fmt.Sprintf("<<< LOCAL\n%s\n===\n%s\n>>> SERVER", localValue, serverValue),
	}
}

func cloneYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	clone := *node
	if len(node.Content) > 0 {
		clone.Content = make([]*yaml.Node, len(node.Content))
		for i, child := range node.Content {
			clone.Content[i] = cloneYAMLNode(child)
		}
	}
	return &clone
}
