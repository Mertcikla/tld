package workspacesource

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	mermaidcore "github.com/mertcikla/tld/v2/internal/mermaid"
)

var (
	flowchartHeaderPattern = regexp.MustCompile(`^(?:flowchart|graph)\s+(?:TB|TD|BT|RL|LR)\b`)
	nodeLinePattern        = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*\["((?:\\.|[^"\\])*)"\]$`)
	edgeLinePattern        = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s+(?:--\s+"((?:\\.|[^"\\])*)"\s+-->|-->)\s+([A-Za-z_][A-Za-z0-9_]*)$`)
)

// ParseTree reads a workspace-source view tree rooted at .tld/views.
func ParseTree(root string) (*desiredWorkspace, error) {
	workspace := &desiredWorkspace{
		Views:          map[string]*desiredView{},
		Elements:       map[string]*desiredElement{},
		Connectors:     map[string]*desiredConnector{},
		RootPath:       root,
		SourceHash:     "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		ViewOrder:      []string{},
		ElementOrder:   []string{},
		ConnectorOrder: []string{},
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return workspace, nil
		}
		return nil, fmt.Errorf("stat workspace-source root: %w", err)
	}

	type docFile struct {
		path string
		dir  string
	}
	var docs []docFile
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}

		docPath := filepath.Join(path, ViewFileName)
		info, err := os.Stat(docPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("%s must be a file", docPath)
		}
		docs = append(docs, docFile{path: docPath, dir: path})
		return nil
	}); err != nil {
		return nil, err
	}

	sort.Slice(docs, func(i, j int) bool { return docs[i].path < docs[j].path })
	viewRefByDir := map[string]string{}
	for _, doc := range docs {
		parentDir := filepath.Dir(doc.dir)
		parentRef := "root"
		if ref, ok := viewRefByDir[parentDir]; ok {
			parentRef = ref
		}
		parsed, err := parseDocument(doc.path, parentRef)
		if err != nil {
			return nil, err
		}
		if _, exists := workspace.Views[parsed.view.Ref]; exists {
			return nil, fmt.Errorf("duplicate view ref %q", parsed.view.Ref)
		}
		workspace.Views[parsed.view.Ref] = parsed.view
		workspace.ViewOrder = append(workspace.ViewOrder, parsed.view.Ref)
		viewRefByDir[doc.dir] = parsed.view.Ref
		if parentRef != "root" && parsed.view.ParentRef != parentRef {
			workspace.Warnings = append(workspace.Warnings, fmt.Sprintf("%s parent metadata %q does not match folder parent %q", doc.path, parsed.view.ParentRef, parentRef))
		}

		for _, ref := range parsed.elementOrder {
			next := parsed.elements[ref]
			if existing, ok := workspace.Elements[ref]; ok {
				if !sameElementIdentity(existing, next) {
					workspace.Warnings = append(workspace.Warnings, fmt.Sprintf("element ref %q has conflicting metadata; first definition wins", ref))
				}
			} else {
				workspace.Elements[ref] = next
				workspace.ElementOrder = append(workspace.ElementOrder, ref)
			}
			workspace.Placements = append(workspace.Placements, parsed.placements[ref])
		}
		for _, ref := range parsed.connectorOrder {
			if _, exists := workspace.Connectors[ref]; exists {
				return nil, fmt.Errorf("duplicate connector ref %q", ref)
			}
			workspace.Connectors[ref] = parsed.connectors[ref]
			workspace.ConnectorOrder = append(workspace.ConnectorOrder, ref)
		}
	}

	hash, err := HashTree(root)
	if err != nil {
		return nil, err
	}
	workspace.SourceHash = hash
	return workspace, nil
}

type parsedDocument struct {
	view           *desiredView
	elements       map[string]*desiredElement
	placements     map[string]desiredPlacement
	connectors     map[string]*desiredConnector
	elementOrder   []string
	connectorOrder []string
}

func parseDocument(path string, derivedParentRef string) (*parsedDocument, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	source, err := extractWorkspaceSourceMermaidCode(path, string(data))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(source, "\n")
	out := &parsedDocument{
		elements:       map[string]*desiredElement{},
		placements:     map[string]desiredPlacement{},
		connectors:     map[string]*desiredConnector{},
		elementOrder:   []string{},
		connectorOrder: []string{},
	}
	aliasLabels := map[string]string{}
	aliasRefs := map[string]string{}
	var markerSeen bool
	var headerSeen bool
	var pendingNodeAlias string
	var pendingNodeLabel string
	var pendingEdge *pendingConnectorEdge
	for lineNumber, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "%%") {
			body := strings.TrimSpace(strings.TrimPrefix(trimmed, "%%"))
			if strings.HasPrefix(body, "tld/v1") {
				pairs := parseMetadataPairs(strings.TrimSpace(strings.TrimPrefix(body, "tld/v1")))
				view, err := parseViewMetadata(pairs, path, derivedParentRef)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: %w", path, lineNumber+1, err)
				}
				out.view = view
				markerSeen = true
				continue
			}
			if !markerSeen {
				continue
			}
			if strings.HasPrefix(body, "tld-element") {
				if pendingNodeAlias == "" {
					return nil, fmt.Errorf("%s:%d: tld-element metadata must follow a node line", path, lineNumber+1)
				}
				pairs := parseMetadataPairs(strings.TrimSpace(strings.TrimPrefix(body, "tld-element")))
				element, placement, err := parseElementMetadata(pairs, out.view.Ref, pendingNodeLabel)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: %w", path, lineNumber+1, err)
				}
				if _, exists := out.elements[element.Ref]; exists {
					return nil, fmt.Errorf("%s:%d: duplicate element ref %q in view %q", path, lineNumber+1, element.Ref, out.view.Ref)
				}
				out.elements[element.Ref] = element
				out.placements[element.Ref] = placement
				out.elementOrder = append(out.elementOrder, element.Ref)
				aliasRefs[pendingNodeAlias] = element.Ref
				pendingNodeAlias = ""
				pendingNodeLabel = ""
				continue
			}
			if strings.HasPrefix(body, "tld-connector") {
				if pendingEdge == nil {
					return nil, fmt.Errorf("%s:%d: tld-connector metadata must follow an edge line", path, lineNumber+1)
				}
				pairs := parseMetadataPairs(strings.TrimSpace(strings.TrimPrefix(body, "tld-connector")))
				connector, err := parseConnectorMetadata(pairs, out.view.Ref, pendingEdge)
				if err != nil {
					return nil, fmt.Errorf("%s:%d: %w", path, lineNumber+1, err)
				}
				if _, exists := out.connectors[connector.Ref]; exists {
					return nil, fmt.Errorf("%s:%d: duplicate connector ref %q in view %q", path, lineNumber+1, connector.Ref, out.view.Ref)
				}
				out.connectors[connector.Ref] = connector
				out.connectorOrder = append(out.connectorOrder, connector.Ref)
				pendingEdge = nil
				continue
			}
			continue
		}
		if !headerSeen {
			headerSeen = true
			if !flowchartHeaderPattern.MatchString(trimmed) {
				return nil, fmt.Errorf("%s:%d: expected flowchart Mermaid header", path, lineNumber+1)
			}
			continue
		}
		if !markerSeen {
			continue
		}
		if pendingNodeAlias != "" {
			return nil, fmt.Errorf("%s:%d: node %q is missing tld-element metadata", path, lineNumber, pendingNodeAlias)
		}
		if pendingEdge != nil {
			return nil, fmt.Errorf("%s:%d: edge %s -> %s is missing tld-connector metadata", path, lineNumber, pendingEdge.sourceAlias, pendingEdge.targetAlias)
		}
		if match := nodeLinePattern.FindStringSubmatch(trimmed); match != nil {
			pendingNodeAlias = match[1]
			pendingNodeLabel = mermaidLabel(match[2])
			aliasLabels[pendingNodeAlias] = pendingNodeLabel
			continue
		}
		if match := edgeLinePattern.FindStringSubmatch(trimmed); match != nil {
			pendingEdge = &pendingConnectorEdge{
				sourceAlias: match[1],
				targetAlias: match[3],
				label:       mermaidLabel(match[2]),
				aliasRefs:   aliasRefs,
				aliasLabels: aliasLabels,
			}
			continue
		}
		return nil, fmt.Errorf("%s:%d: unsupported Mermaid line %q", path, lineNumber+1, trimmed)
	}
	if !headerSeen {
		return nil, fmt.Errorf("%s: expected flowchart Mermaid header", path)
	}
	if out.view == nil {
		return nil, fmt.Errorf("%s: missing %% tld/v1 view metadata", path)
	}
	if pendingNodeAlias != "" {
		return nil, fmt.Errorf("%s: node %q is missing tld-element metadata", path, pendingNodeAlias)
	}
	if pendingEdge != nil {
		return nil, fmt.Errorf("%s: edge %s -> %s is missing tld-connector metadata", path, pendingEdge.sourceAlias, pendingEdge.targetAlias)
	}
	return out, nil
}

func extractWorkspaceSourceMermaidCode(path, markdown string) (string, error) {
	blocks := mermaidcore.FindMarkdownBlocks(markdown)
	if len(blocks) != 1 {
		return "", fmt.Errorf("%s: expected exactly one Markdown Mermaid block", path)
	}
	block := blocks[0]
	if strings.TrimSpace(markdown[:block.Start]) != "" || strings.TrimSpace(markdown[block.End:]) != "" {
		return "", fmt.Errorf("%s: workspace-source Markdown must contain only one Mermaid block", path)
	}
	if !strings.HasPrefix(block.Fence, "```") {
		return "", fmt.Errorf("%s: workspace-source Mermaid block must use backtick fences", path)
	}
	if strings.TrimSpace(block.Code) == "" {
		return "", fmt.Errorf("%s: empty Mermaid block", path)
	}
	return strings.TrimSpace(block.Code), nil
}

type pendingConnectorEdge struct {
	sourceAlias string
	targetAlias string
	label       string
	aliasRefs   map[string]string
	aliasLabels map[string]string
}

func parseViewMetadata(pairs map[string]string, path, derivedParentRef string) (*desiredView, error) {
	ref, err := metadataRequired(pairs, "ref", "view")
	if err != nil {
		return nil, err
	}
	parentRef := pairString(pairs, "parent")
	if parentRef == "" {
		parentRef = derivedParentRef
	}
	name := pairString(pairs, "name")
	if name == "" {
		name = ref
	}
	return &desiredView{
		Ref:       ref,
		ParentRef: parentRef,
		Name:      name,
		OwnerRef:  pairString(pairs, "owner"),
		Path:      path,
	}, nil
}

func parseElementMetadata(pairs map[string]string, viewRef, label string) (*desiredElement, desiredPlacement, error) {
	ref, err := metadataRequired(pairs, "ref", "element")
	if err != nil {
		return nil, desiredPlacement{}, err
	}
	x, _ := pairFloat(pairs, "x")
	y, _ := pairFloat(pairs, "y")
	element := &desiredElement{
		Ref:             ref,
		Name:            label,
		Kind:            pairString(pairs, "kind"),
		Description:     pairString(pairs, "desc"),
		Technology:      pairString(pairs, "tech"),
		URL:             pairString(pairs, "url"),
		LogoURL:         pairString(pairs, "logo"),
		Tags:            pairStringList(pairs, "tags"),
		Repo:            pairString(pairs, "repo"),
		Branch:          pairString(pairs, "branch"),
		Language:        pairString(pairs, "lang"),
		FilePath:        pairString(pairs, "file"),
		BypassNoiseGate: pairBool(pairs, "bypass"),
		HasView:         boolValue(pairBool(pairs, "hasView")),
		ViewLabel:       pairString(pairs, "viewLabel"),
	}
	if element.Name == "" {
		element.Name = ref
	}
	placement := desiredPlacement{ViewRef: viewRef, ElementRef: ref, X: x, Y: y}
	return element, placement, nil
}

func parseConnectorMetadata(pairs map[string]string, viewRef string, edge *pendingConnectorEdge) (*desiredConnector, error) {
	ref, err := metadataRequired(pairs, "ref", "connector")
	if err != nil {
		return nil, err
	}
	sourceRef := pairString(pairs, "source")
	if sourceRef == "" {
		sourceRef = edge.aliasRefs[edge.sourceAlias]
	}
	targetRef := pairString(pairs, "target")
	if targetRef == "" {
		targetRef = edge.aliasRefs[edge.targetAlias]
	}
	if sourceRef == "" || targetRef == "" {
		return nil, fmt.Errorf("connector %q missing source or target refs", ref)
	}
	label := pairString(pairs, "label")
	if label == "" {
		label = edge.label
	}
	return &desiredConnector{
		Ref:          ref,
		ViewRef:      viewRef,
		SourceRef:    sourceRef,
		TargetRef:    targetRef,
		Label:        label,
		Description:  pairString(pairs, "desc"),
		Relationship: pairString(pairs, "rel"),
		Direction:    pairString(pairs, "dir"),
		Style:        pairString(pairs, "style"),
		URL:          pairString(pairs, "url"),
		SourceHandle: pairString(pairs, "sourceHandle"),
		TargetHandle: pairString(pairs, "targetHandle"),
	}, nil
}

func boolValue(value *bool) bool {
	return value != nil && *value
}

func mermaidLabel(value string) string {
	return mermaidcore.DecodeMermaidLabel(value)
}

func sameElementIdentity(a, b *desiredElement) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.Kind == b.Kind &&
		a.Description == b.Description &&
		a.Technology == b.Technology &&
		a.URL == b.URL &&
		a.LogoURL == b.LogoURL &&
		a.Repo == b.Repo &&
		a.Branch == b.Branch &&
		a.Language == b.Language &&
		a.FilePath == b.FilePath
}

func HashTree(root string) (string, error) {
	hash := sha256.New()
	var files []string
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Base(path) != ViewFileName {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", nil
		}
		return "", err
	}
	sort.Strings(files)
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(filepath.ToSlash(rel)))
		hash.Write([]byte{0})
		hash.Write(data)
		hash.Write([]byte{0})
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil)), nil
}
