package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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
			nil,
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
			connectorKeyFromNode,
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

func mergeYAMLMapWithMetadataSections(path string, serverItems any, serverMeta map[string]*ResourceMetadata, lastSyncMeta map[string]*ResourceMetadata, currentMeta map[string]*ResourceMetadata, normalizeKey func(string, *yaml.Node) string, sections []metadataSection) error {
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

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return WriteFullYAMLMapSections(path, serverItems, sections)
	}

	// Convert serverItems to a map for easy lookup
	serverItemsData, _ := yaml.Marshal(serverItems)
	var serverItemsMap map[string]any
	_ = yaml.Unmarshal(serverItemsData, &serverItemsMap)

	switch root.Content[0].Kind {
	case yaml.MappingNode:
		return mergeYAMLMappingNode(path, &root, root.Content[0], serverItemsMap, serverMeta, lastSyncMeta, currentMeta, normalizeKey, sections)
	case yaml.SequenceNode:
		if normalizeKey == nil {
			return WriteFullYAMLMapSections(path, serverItems, sections)
		}
		return mergeYAMLSequenceNode(path, &root, root.Content[0], serverItems, serverItemsMap, serverMeta, lastSyncMeta, currentMeta, normalizeKey, sections)
	default:
		return WriteFullYAMLMapSections(path, serverItems, sections)
	}
}

// mergeYAMLMappingNode merges a mapping-rooted workspace document, where each
// resource is keyed by its ref and metadata lives in _meta_* sections.
func mergeYAMLMappingNode(path string, root, mapping *yaml.Node, serverItemsMap map[string]any, serverMeta, lastSyncMeta, currentMeta map[string]*ResourceMetadata, normalizeKey func(string, *yaml.Node) string, sections []metadataSection) error {
	seenKeys := make(map[string]bool)
	var newContent []*yaml.Node

	// Iterate through existing mapping (local state)
	for i := 0; i < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		valNode := mapping.Content[i+1]
		key := keyNode.Value
		if normalizeKey != nil {
			if normalized := normalizeKey(key, valNode); normalized != key {
				keyNode.Value = normalized
				key = normalized
			}
		}

		if isMetadataSectionKey(key, sections) {
			continue
		}

		serverItem, onServer := serverItemsMap[key]
		if onServer {
			seenKeys[key] = true
		}

		entry, err := mergeLocalYAMLValue(path, key, valNode, serverItem, onServer, serverMeta[key], lastSyncMeta[key], currentMeta[key])
		if err != nil {
			return err
		}
		applyEntryConflict(entry, sections, serverMeta, key)
		if !entry.keep {
			continue
		}
		newContent = append(newContent, keyNode, entry.node)
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

	// Write back, preserving/adding the yaml-language-server schema directive.
	return encodeYAMLWithSchemaHeader(path, root)
}

// mergeYAMLSequenceNode merges a sequence-rooted document (the flat list format
// Save/WriteFullYAMLList produce for connectors.yaml). The sequence shape is
// preserved; since a list has nowhere to attach a _meta_* section, metadata that
// would otherwise be persisted there is written inline on each entry instead.
func mergeYAMLSequenceNode(path string, root, sequence *yaml.Node, serverItems any, serverItemsMap map[string]any, serverMeta, lastSyncMeta, currentMeta map[string]*ResourceMetadata, normalizeKey func(string, *yaml.Node) string, sections []metadataSection) error {
	inlineUpdatedAt := sectionsPersistInlineMetadata(sections)

	seenKeys := make(map[string]bool)
	newContent := make([]*yaml.Node, 0, len(sequence.Content))

	for _, item := range sequence.Content {
		key := normalizeKey("", item)
		serverItem, onServer := serverItemsMap[key]
		if onServer {
			seenKeys[key] = true
		}

		entry, err := mergeLocalYAMLValue(path, key, item, serverItem, onServer, serverMeta[key], lastSyncMeta[key], currentMeta[key])
		if err != nil {
			return err
		}
		applyEntryConflict(entry, nil, serverMeta, key)
		if !entry.keep {
			continue
		}
		newContent = append(newContent, entry.node)
	}

	// Append server connectors that were not present locally. Sorting matches
	// the deterministic ordering Save uses for the flat list.
	serverKeys := make([]string, 0, len(serverItemsMap))
	for key := range serverItemsMap {
		if !seenKeys[key] {
			serverKeys = append(serverKeys, key)
		}
	}
	sort.Strings(serverKeys)
	for _, key := range serverKeys {
		node, err := encodeSequenceServerEntry(serverItems, key, serverItemsMap[key], serverMeta[key], inlineUpdatedAt)
		if err != nil {
			return err
		}
		newContent = append(newContent, node)
	}

	sequence.Content = newContent

	return encodeYAMLWithSchemaHeader(path, root)
}

// mergedYAMLEntry is the outcome of merging one on-disk entry against the
// server state.
type mergedYAMLEntry struct {
	node        *yaml.Node
	keep        bool
	bothChanged bool
	conflict    bool
}

// mergeLocalYAMLValue applies the pull merge policy to a single local entry:
// keep local-only resources that were never synced, drop ones deleted on the
// server, and reconcile when both sides changed.
func mergeLocalYAMLValue(path, key string, localNode *yaml.Node, serverItem any, onServer bool, serverMeta, lastSyncMeta, currentMeta *ResourceMetadata) (mergedYAMLEntry, error) {
	if !onServer {
		// Resource exists locally but not on server.
		if lastSyncMeta == nil {
			// Was never on the server, so it is a new local resource. Keep it.
			return mergedYAMLEntry{node: localNode, keep: true}, nil
		}
		// Was on the server before (last sync), so it was deleted on the
		// server. Remove it locally too.
		return mergedYAMLEntry{}, nil
	}

	localChanged := lastSyncMeta != nil && currentMeta != nil && currentMeta.UpdatedAt.After(lastSyncMeta.UpdatedAt)
	serverChanged := lastSyncMeta != nil && serverMeta != nil && serverMeta.UpdatedAt.After(lastSyncMeta.UpdatedAt)

	switch {
	case localChanged && serverChanged:
		mergedNode, hasConflict, err := mergeResourceValueNode(localNode, serverItem, key)
		if err != nil {
			return mergedYAMLEntry{}, fmt.Errorf("merge %s[%s]: %w", filepath.Base(path), key, err)
		}
		return mergedYAMLEntry{node: mergedNode, keep: true, bothChanged: true, conflict: hasConflict}, nil
	case serverChanged:
		// Server changed, local did not: update local with server content.
		var newValNode yaml.Node
		_ = newValNode.Encode(serverItem)
		return mergedYAMLEntry{node: &newValNode, keep: true}, nil
	default:
		// No changes or only local changes: keep local.
		return mergedYAMLEntry{node: localNode, keep: true}, nil
	}
}

// applyEntryConflict records conflict state for an entry that changed on both
// sides, preferring the persisted metadata section when present.
func applyEntryConflict(entry mergedYAMLEntry, sections []metadataSection, serverMeta map[string]*ResourceMetadata, key string) {
	if !entry.bothChanged {
		return
	}
	if entry.conflict {
		for _, section := range sections {
			if section.values[key] != nil {
				section.values[key].Conflict = true
				return
			}
		}
		if meta := serverMeta[key]; meta != nil {
			meta.Conflict = true
		}
		return
	}
	if meta := serverMeta[key]; meta != nil {
		meta.Conflict = false
	}
}

// sectionsPersistInlineMetadata reports whether any metadata section is meant to
// be persisted on disk rather than in the lockfile.
func sectionsPersistInlineMetadata(sections []metadataSection) bool {
	for _, section := range sections {
		if section.persist && len(section.values) > 0 {
			return true
		}
	}
	return false
}

// encodeSequenceServerEntry builds the YAML node for a server connector that is
// being added to a flat list, mirroring Save by writing id/updated_at inline
// when the metadata is not persisted elsewhere.
func encodeSequenceServerEntry(serverItems any, key string, fallback any, meta *ResourceMetadata, includeUpdatedAt bool) (*yaml.Node, error) {
	if connectors, ok := serverItems.(map[string]*Connector); ok {
		if connector := connectors[key]; connector != nil {
			copyConnector := *connector
			if meta != nil {
				copyConnector.ID = meta.ID
				if includeUpdatedAt {
					copyConnector.UpdatedAt = meta.UpdatedAt
				} else {
					copyConnector.UpdatedAt = time.Time{}
				}
			}
			return encodeYAMLValueNode(&copyConnector)
		}
	}
	return encodeYAMLValueNode(fallback)
}

// connectorKeyFromNode returns the canonical key for an on-disk connector
// entry, preferring the connector's own fields so hand-written or legacy keys
// migrate to the same key the loader would compute.
func connectorKeyFromNode(key string, value *yaml.Node) string {
	if value != nil && value.Kind == yaml.MappingNode {
		var connector Connector
		if err := value.Decode(&connector); err == nil && (connector.View != "" || connector.Source != "" || connector.Target != "") {
			return ConnectorKey(&connector)
		}
	}
	return NormalizeConnectorKey(key)
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
		merged.HeadComment = fmt.Sprintf("CONFLICT: %q was modified both locally and on the server.\nResolve the marked values, then run the matching add/update command to sync.", ref)
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
	comment := fmt.Sprintf("CONFLICT: '%s' was modified both locally and on the server.\nLocal:  %s\nServer: %s\nResolve by keeping one value, then run the matching add/update command to sync.", fieldName, localValue, serverValue)
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
