package workspace

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type metadataSection struct {
	name    string
	values  map[string]*ResourceMetadata
	persist bool
}

// WriteElement adds an element to elements.yaml. Errors if ref already exists.
func WriteElement(dir, ref string, spec *Element) error {
	path := filepath.Join(dir, "elements.yaml")
	return updateYAMLMap(path, ref, spec)
}

// UpdateElement overwrites an element in elements.yaml.
func UpdateElement(dir, ref string, spec *Element) error {
	path := filepath.Join(dir, "elements.yaml")
	existing := make(map[string]*Element)
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &existing)
	}
	existing[ref] = spec
	data, err := marshalPrettyYAML(existing)
	if err != nil {
		return fmt.Errorf("marshal elements: %w", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write elements.yaml: %w", err)
	}
	return nil
}

// UpsertElement adds an element to elements.yaml or updates placements on an existing one.
func UpsertElement(dir, ref string, spec *Element) error {
	return upsertYAMLNodeKey(filepath.Join(dir, "elements.yaml"), ref, spec)
}

// AppendConnector adds a connector to connectors.yaml.
func AppendConnector(dir string, spec *Connector) error {
	ws, err := Load(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		ws = &Workspace{Dir: dir, Elements: make(map[string]*Element), Connectors: make(map[string]*Connector)}
	}
	ws.Connectors[ConnectorKey(spec)] = spec
	return Save(ws)
}

// AppendConnectors adds multiple connectors to connectors.yaml in a single operation.
func AppendConnectors(dir string, specs []*Connector) error {
	if len(specs) == 0 {
		return nil
	}
	ws, err := Load(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		ws = &Workspace{Dir: dir, Elements: make(map[string]*Element), Connectors: make(map[string]*Connector)}
	}
	if ws.Connectors == nil {
		ws.Connectors = make(map[string]*Connector)
	}
	for _, spec := range specs {
		ws.Connectors[ConnectorKey(spec)] = spec
	}
	return Save(ws)
}

// Save writes the entire workspace state to YAML files in ws.Dir.
func Save(ws *Workspace) error {
	if useElementWorkspaceFiles(ws) {
		if ws.Elements == nil {
			ws.Elements = make(map[string]*Element)
		}
		if ws.Connectors == nil {
			ws.Connectors = make(map[string]*Connector)
		}

		var elementMeta map[string]*ResourceMetadata
		var viewMeta map[string]*ResourceMetadata
		var connectorMeta map[string]*ResourceMetadata
		if ws.Meta != nil {
			elementMeta = ws.Meta.Elements
			viewMeta = ws.Meta.Views
			connectorMeta = ws.Meta.Connectors
		}

		storedElementMeta, err := PersistCurrentElementMetadata(ws.Dir, elementMeta)
		if err != nil {
			return fmt.Errorf("persist current element metadata: %w", err)
		}
		storedViewMeta, err := PersistCurrentViewMetadata(ws.Dir, viewMeta)
		if err != nil {
			return fmt.Errorf("persist current view metadata: %w", err)
		}
		storedConnectorMeta, err := PersistCurrentConnectorMetadata(ws.Dir, connectorMeta)
		if err != nil {
			return fmt.Errorf("persist current connector metadata: %w", err)
		}

		elementSections := []metadataSection{}
		if !storedElementMeta {
			elementSections = append(elementSections, metadataSection{name: "_meta_elements", values: elementMeta, persist: true})
		}
		if !storedViewMeta {
			elementSections = append(elementSections, metadataSection{name: "_meta_views", values: viewMeta, persist: true})
		}

		if err := WriteFullYAMLMapSections(filepath.Join(ws.Dir, "elements.yaml"), ws.Elements, elementSections); err != nil {
			return fmt.Errorf("write elements: %w", err)
		}
		var connectorList []*Connector
		connectorRefs := make([]string, 0, len(ws.Connectors))
		for ref := range ws.Connectors {
			connectorRefs = append(connectorRefs, ref)
		}
		sort.Strings(connectorRefs)
		for _, ref := range connectorRefs {
			copyConnector := *ws.Connectors[ref]
			if connectorMeta != nil && connectorMeta[ref] != nil {
				copyConnector.ID = connectorMeta[ref].ID
				if storedConnectorMeta {
					copyConnector.UpdatedAt = time.Time{}
				} else {
					copyConnector.UpdatedAt = connectorMeta[ref].UpdatedAt
				}
			} else if storedConnectorMeta {
				copyConnector.UpdatedAt = time.Time{}
			}
			connectorList = append(connectorList, &copyConnector)
		}

		if err := WriteFullYAMLList(filepath.Join(ws.Dir, "connectors.yaml"), connectorList); err != nil {
			return fmt.Errorf("write connectors: %w", err)
		}
		if err := cleanupLegacyWorkspaceFiles(ws.Dir); err != nil {
			return fmt.Errorf("cleanup legacy workspace files: %w", err)
		}
		return nil
	}

	return nil
}

func upsertYAMLNodeKey(path, ref string, spec any) error {
	root, mapping, err := loadYAMLMappingNode(path)
	if err != nil {
		return err
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != ref {
			continue
		}

		mergedSpec, err := mergeExistingSpec(ref, mapping.Content[i+1], spec)
		if err != nil {
			return err
		}
		newValue, err := encodeYAMLValueNode(mergedSpec)
		if err != nil {
			return fmt.Errorf("encode %s: %w", ref, err)
		}
		mapping.Content[i+1] = newValue
		return writeYAMLNode(path, root)
	}

	newValue, err := encodeYAMLValueNode(spec)
	if err != nil {
		return fmt.Errorf("encode %s: %w", ref, err)
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: ref},
		newValue,
	)
	return writeYAMLNode(path, root)
}

func loadYAMLMappingNode(path string) (*yaml.Node, *yaml.Node, error) {
	var root yaml.Node
	data, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return nil, nil, fmt.Errorf("parse %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("read %s: %w", path, err)
	}

	if root.Kind == 0 {
		root = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%s must contain a YAML mapping", filepath.Base(path))
	}
	if filepath.Base(path) == "elements.yaml" {
		if err := migrateDeprecatedElementWorkspaceMetadata(filepath.Dir(path), root.Content[0]); err != nil {
			return nil, nil, err
		}
	}
	return &root, root.Content[0], nil
}

func migrateDeprecatedElementWorkspaceMetadata(dir string, mapping *yaml.Node) error {
	if err := migrateDeprecatedMetadataSection(dir, mapping, "_meta_elements", PersistCurrentElementMetadata); err != nil {
		return err
	}
	return migrateDeprecatedMetadataSection(dir, mapping, "_meta_views", PersistCurrentViewMetadata)
}

func migrateDeprecatedMetadataSection(dir string, mapping *yaml.Node, sectionName string, persist func(string, map[string]*ResourceMetadata) (bool, error)) error {
	keyIndex, valueNode := findMappingValueNode(mapping, sectionName)
	if valueNode == nil {
		return nil
	}

	meta, err := DecodeMetadataSectionNode(valueNode)
	if err != nil {
		return fmt.Errorf("decode %s: %w", sectionName, err)
	}

	stored, err := persist(dir, meta)
	if err != nil {
		return fmt.Errorf("persist %s to lock file: %w", sectionName, err)
	}
	if stored || len(meta) == 0 {
		removeMappingEntry(mapping, keyIndex)
	}
	return nil
}

func findMappingValueNode(mapping *yaml.Node, key string) (int, *yaml.Node) {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return -1, nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i, mapping.Content[i+1]
		}
	}
	return -1, nil
}

func removeMappingEntry(mapping *yaml.Node, keyIndex int) {
	if mapping == nil || mapping.Kind != yaml.MappingNode || keyIndex < 0 || keyIndex+1 >= len(mapping.Content) {
		return
	}
	mapping.Content = append(mapping.Content[:keyIndex], mapping.Content[keyIndex+2:]...)
}

func encodeYAMLValueNode(spec any) (*yaml.Node, error) {
	var value yaml.Node
	if err := value.Encode(spec); err != nil {
		return nil, err
	}
	if value.Kind == yaml.DocumentNode {
		if len(value.Content) == 0 {
			return &yaml.Node{}, nil
		}
		normalizeYAMLStyle(value.Content[0])
		return value.Content[0], nil
	}
	normalizeYAMLStyle(&value)
	return &value, nil
}

func normalizeYAMLStyle(node *yaml.Node) {
	normalizeYAMLStyleRecursive(node, 0)
}

func normalizeYAMLStyleRecursive(node *yaml.Node, depth int) {
	if node == nil {
		return
	}
	node.Style &^= yaml.FlowStyle

	if node.Kind == yaml.MappingNode {
		// Prettify field order within elements (depth 2)
		// We want 'name' and 'kind' to always be at the top.
		if depth == 2 {
			reorderMappingFields(node, []string{"name", "kind"})
		}
	}

	for _, child := range node.Content {
		normalizeYAMLStyleRecursive(child, depth+1)
	}
}

func reorderMappingFields(node *yaml.Node, priority []string) {
	if node.Kind != yaml.MappingNode {
		return
	}
	var newContent []*yaml.Node
	seen := make(map[int]bool)

	// Pull priority fields to the top
	for _, fieldName := range priority {
		for i := 0; i+1 < len(node.Content); i += 2 {
			if !seen[i] && node.Content[i].Value == fieldName {
				newContent = append(newContent, node.Content[i], node.Content[i+1])
				seen[i] = true
				break
			}
		}
	}

	// Append everything else in original order
	for i := 0; i+1 < len(node.Content); i += 2 {
		if !seen[i] {
			newContent = append(newContent, node.Content[i], node.Content[i+1])
		}
	}

	node.Content = newContent
}

func mergeExistingSpec(ref string, existingNode *yaml.Node, spec any) (any, error) {
	switch incoming := spec.(type) {
	case *Element:
		var existing Element
		if err := existingNode.Decode(&existing); err != nil {
			return nil, fmt.Errorf("decode existing element %q: %w", ref, err)
		}
		return mergeElementFields(ref, &existing, incoming)
	default:
		return spec, nil
	}
}

func mergeElementFields(ref string, existing, incoming *Element) (*Element, error) {
	if existing.Kind != incoming.Kind {
		return nil, fmt.Errorf("element %q already exists with kind %q (tried to reuse as %q)", ref, existing.Kind, incoming.Kind)
	}

	merged := *existing
	if merged.Name == "" {
		merged.Name = incoming.Name
	}
	if merged.Description == "" {
		merged.Description = incoming.Description
	}
	if merged.Technology == "" {
		merged.Technology = incoming.Technology
	}
	if merged.URL == "" {
		merged.URL = incoming.URL
	}
	if incoming.HasView {
		merged.HasView = true
		if merged.ViewLabel == "" {
			merged.ViewLabel = incoming.ViewLabel
		}
	}
	for _, newPlacement := range incoming.Placements {
		found := false
		for index, placement := range merged.Placements {
			if placement.ParentRef == newPlacement.ParentRef {
				merged.Placements[index].PositionX = newPlacement.PositionX
				merged.Placements[index].PositionY = newPlacement.PositionY
				found = true
				break
			}
		}
		if !found {
			merged.Placements = append(merged.Placements, newPlacement)
		}
	}
	return &merged, nil
}

func ConnectorKey(spec *Connector) string {
	return spec.View + ":" + spec.Source + ":" + spec.Target + ":" + spec.Label
}

func writeYAMLNode(path string, root *yaml.Node) error {
	normalizeYAMLStyle(root)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

func marshalPrettyYAML(v any) ([]byte, error) {
	var node yaml.Node
	if err := node.Encode(v); err != nil {
		return nil, err
	}
	normalizeYAMLStyle(&node)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		_ = enc.Close()
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteFullYAMLList writes a list of items to a YAML file.
func WriteFullYAMLList(path string, items any) error {
	var node yaml.Node
	if err := node.Encode(items); err != nil {
		return fmt.Errorf("encode items for %s: %w", path, err)
	}
	normalizeYAMLStyle(&node)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return f.Close()
}
func WriteFullYAMLMap(path string, items any, meta map[string]*ResourceMetadata) error {
	return WriteFullYAMLMapSections(path, items, []metadataSection{{name: "_meta", values: meta, persist: true}})
}

func WriteFullYAMLMapSections(path string, items any, sections []metadataSection) error {
	// 1. Marshal items to a node
	var node yaml.Node
	if err := node.Encode(items); err != nil {
		return fmt.Errorf("encode items for %s: %w", path, err)
	}
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}

	var mapping *yaml.Node
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		mapping = node.Content[0]
	} else if node.Kind == yaml.MappingNode {
		mapping = &node
	}
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		mapping = &yaml.Node{Kind: yaml.MappingNode}
		node = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping}}
	}

	for _, section := range sections {
		if !section.persist || len(section.values) == 0 {
			continue
		}
		metaNode, err := EncodeMeta(section.values)
		if err != nil {
			return fmt.Errorf("encode %s for %s: %w", section.name, path, err)
		}
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: section.name},
			metaNode,
		)
	}

	// 3. Write back to file with specific indentation
	normalizeYAMLStyle(&node)
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() {
		_ = f.Close()
	}()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(&node); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}

	return nil
}

func useElementWorkspaceFiles(ws *Workspace) bool {
	if len(ws.Elements) > 0 || len(ws.Connectors) > 0 {
		return true
	}
	for _, filename := range []string{"elements.yaml", "connectors.yaml"} {
		if _, err := os.Stat(filepath.Join(ws.Dir, filename)); err == nil {
			return true
		}
	}
	return false
}

func cleanupLegacyWorkspaceFiles(dir string) error {
	for _, filename := range []string{"diagrams.yaml", "objects.yaml", "edges.yaml", "links.yaml"} {
		err := os.Remove(filepath.Join(dir, filename))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// EncodeMeta converts ResourceMetadata map into a yaml.Node.
func EncodeMeta(meta map[string]*ResourceMetadata) (*yaml.Node, error) {
	metaNode := &yaml.Node{Kind: yaml.MappingNode}
	// Sort keys for determinism (yaml.v3 does this anyway for maps, but we are building node)
	// Actually, let's just use Encode and then move it into our node
	data, err := yaml.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("marshal meta: %w", err)
	}
	var n yaml.Node
	if err := yaml.Unmarshal(data, &n); err != nil {
		return nil, fmt.Errorf("unmarshal meta: %w", err)
	}
	if len(n.Content) > 0 {
		return n.Content[0], nil
	}
	return metaNode, nil
}

// RenameElement changes an element ref in elements.yaml and cascades to connectors.
func RenameElement(dir, oldRef, newRef string) error {
	if oldRef == newRef {
		return nil
	}

	path := filepath.Join(dir, "elements.yaml")
	root, mapping, err := loadYAMLMappingNode(path)
	if err != nil {
		return err
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		valNode := mapping.Content[i+1]
		if keyNode.Value == "_meta_elements" || keyNode.Value == "_meta_views" {
			continue
		}
		if keyNode.Value == oldRef {
			keyNode.Value = newRef
		}
		updatePlacementParentRefs(valNode, oldRef, newRef)
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		valNode := mapping.Content[i+1]
		if keyNode.Value == "_meta_elements" || keyNode.Value == "_meta_views" {
			renameMappingKeys(valNode, map[string]string{oldRef: newRef})
		}
	}

	if err := writeYAMLNode(path, root); err != nil {
		return err
	}
	if err := RenameCurrentElementMetadata(dir, oldRef, newRef); err != nil {
		return err
	}
	if err := RenameCurrentViewMetadata(dir, oldRef, newRef); err != nil {
		return err
	}
	return RenameConnector(dir, oldRef, newRef)
}

// UpdateElementField updates one scalar field on an element by ref.
// Special case: field "ref" performs a cascading element rename.
func UpdateElementField(dir, ref, field, value string) error {
	if field == "ref" {
		return RenameElement(dir, ref, value)
	}

	path := filepath.Join(dir, "elements.yaml")
	root, mapping, err := loadYAMLMappingNode(path)
	if err != nil {
		return err
	}

	found := false
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keyNode := mapping.Content[i]
		if keyNode.Value == "_meta_elements" || keyNode.Value == "_meta_views" {
			continue
		}
		if keyNode.Value != ref {
			continue
		}
		found = true
		if err := setMappingScalarField(mapping.Content[i+1], field, value); err != nil {
			return fmt.Errorf("update element %q field %q: %w", ref, field, err)
		}
		break
	}

	if !found {
		return fmt.Errorf("element %q not found", ref)
	}

	return writeYAMLNode(path, root)
}

func updatePlacementParentRefs(node *yaml.Node, oldRef, newRef string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != "placements" {
			continue
		}
		placements := node.Content[i+1]
		if placements.Kind != yaml.SequenceNode {
			return
		}
		for _, placement := range placements.Content {
			updateScalarField(placement, "parent", oldRef, newRef)
		}
	}
}

// RenameConnector updates connector refs in connectors.yaml.
func RenameConnector(dir, oldRef, newRef string) error {
	ws, err := Load(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	changed := false
	newConnectors := make(map[string]*Connector)
	for ref, c := range ws.Connectors {
		fieldChanged := false
		if c.View == oldRef {
			c.View = newRef
			fieldChanged = true
		}
		if c.Source == oldRef {
			c.Source = newRef
			fieldChanged = true
		}
		if c.Target == oldRef {
			c.Target = newRef
			fieldChanged = true
		}

		if fieldChanged {
			newConnectors[ConnectorKey(c)] = c
			changed = true
		} else {
			newConnectors[ref] = c
		}
	}

	if changed {
		if ws.Meta != nil && ws.Meta.Connectors != nil {
			updatedMeta := make(map[string]*ResourceMetadata, len(ws.Meta.Connectors))
			for ref, metadata := range ws.Meta.Connectors {
				if metadata == nil {
					updatedMeta[ref] = nil
					continue
				}
				copyMeta := *metadata
				updatedMeta[ref] = &copyMeta
			}
			for oldKey, connector := range ws.Connectors {
				newKey := ConnectorKey(connector)
				if oldKey == newKey {
					continue
				}
				if metadata, ok := updatedMeta[oldKey]; ok {
					updatedMeta[newKey] = metadata
					delete(updatedMeta, oldKey)
				}
			}
			ws.Meta.Connectors = updatedMeta
		}
		ws.Connectors = newConnectors
		return Save(ws)
	}
	return nil
}

// UpdateConnectorField updates one field on a connector by its key.
func UpdateConnectorField(dir, ref, field, value string) error {
	ws, err := Load(dir)
	if err != nil {
		return err
	}

	c, ok := ws.Connectors[ref]
	if !ok {
		return fmt.Errorf("connector %q not found", ref)
	}

	changed := true
	switch field {
	case "view":
		c.View = value
	case "source":
		c.Source = value
	case "target":
		c.Target = value
	case "label":
		c.Label = value
	case "description":
		c.Description = value
	case "relationship":
		c.Relationship = value
	case "direction":
		c.Direction = value
	case "style":
		c.Style = value
	case "url":
		c.URL = value
	case "source_handle":
		c.SourceHandle = value
	case "target_handle":
		c.TargetHandle = value
	default:
		changed = false
	}

	if changed {
		newKey := ConnectorKey(c)
		if newKey != ref {
			if ws.Meta != nil && ws.Meta.Connectors != nil {
				if metadata, ok := ws.Meta.Connectors[ref]; ok {
					ws.Meta.Connectors[newKey] = metadata
					delete(ws.Meta.Connectors, ref)
				}
			}
			delete(ws.Connectors, ref)
			ws.Connectors[newKey] = c
		}
		return Save(ws)
	}
	return nil
}

func updateScalarField(mapping *yaml.Node, fieldName, oldVal, newVal string) bool {
	if mapping.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == fieldName && mapping.Content[i+1].Value == oldVal {
			mapping.Content[i+1].Value = newVal
			return true
		}
	}
	return false
}

func setMappingScalarField(mapping *yaml.Node, fieldName, value string) error {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return fmt.Errorf("resource value must be a mapping")
	}

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != fieldName {
			continue
		}
		newNode, err := coerceScalarNode(value, mapping.Content[i+1])
		if err != nil {
			return err
		}
		mapping.Content[i+1] = newNode
		return nil
	}

	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: fieldName},
		inferScalarNode(value),
	)
	return nil
}

func coerceScalarNode(value string, existing *yaml.Node) (*yaml.Node, error) {
	if existing != nil && existing.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("field is not scalar and cannot be updated with a single value")
	}

	if existing == nil {
		return inferScalarNode(value), nil
	}

	switch existing.Tag {
	case "!!bool":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("invalid boolean value %q", value)
		}
		if parsed {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}, nil
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"}, nil
	case "!!int":
		if _, err := strconv.Atoi(value); err != nil {
			return nil, fmt.Errorf("invalid integer value %q", value)
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: value}, nil
	case "!!float":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return nil, fmt.Errorf("invalid float value %q", value)
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: value}, nil
	default:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}, nil
	}
}

func inferScalarNode(value string) *yaml.Node {
	if parsedBool, err := strconv.ParseBool(value); err == nil {
		if parsedBool {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"}
	}
	if _, err := strconv.Atoi(value); err == nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: value}
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil && strings.Contains(value, ".") {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: value}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func renameMappingKeys(mapping *yaml.Node, renames map[string]string) {
	if mapping == nil || mapping.Kind != yaml.MappingNode || len(renames) == 0 {
		return
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if newKey, ok := renames[mapping.Content[i].Value]; ok {
			mapping.Content[i].Value = newKey
		}
	}
}

// Slugify converts "API Service" -> "api-service" for use as a ref/filename.
func Slugify(s string) string {
	s = strings.ToLower(s)
	// Replace all non-alphanumeric characters with hyphens
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	s = result.String()
	// Clean up multiple hyphens
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return strings.Trim(s, "-")
}

func updateYAMLMap(path, ref string, item any) error {
	// Read existing map (if any)
	existing := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := yaml.Unmarshal(data, &existing); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
	}

	if _, ok := existing[ref]; ok {
		return fmt.Errorf("item %q already exists in %s", ref, filepath.Base(path))
	}

	// Marshal item to a map so we can insert it
	itemData, err := yaml.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal item: %w", err)
	}
	var itemMap map[string]any
	if err := yaml.Unmarshal(itemData, &itemMap); err != nil {
		return fmt.Errorf("unmarshal item: %w", err)
	}
	existing[ref] = itemMap

	data, err := marshalPrettyYAML(existing)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
