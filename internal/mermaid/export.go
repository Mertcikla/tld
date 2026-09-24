package mermaid

import (
	"fmt"
	"html"
	"math"
	"regexp"
	"slices"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

type metadataEntry struct {
	Key   string
	Value string
}

type exportGroup struct {
	layer   *diagv1.ViewLayer
	marker  string
	members []*diagv1.PlacedElement
}

var elementGroupMarkerPattern = regexp.MustCompile(`^group:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var mermaidGroupColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func ExportView(content *diagv1.ViewContent, viewID int32, includeTldMetadata bool) string {
	return ExportViewWithLayers(content, viewID, includeTldMetadata, nil)
}

func ExportViewWithLayers(content *diagv1.ViewContent, viewID int32, includeTldMetadata bool, layers []*diagv1.ViewLayer) string {
	if content == nil {
		content = &diagv1.ViewContent{}
	}
	placements := append([]*diagv1.PlacedElement(nil), content.GetPlacements()...)
	slices.SortFunc(placements, func(left, right *diagv1.PlacedElement) int {
		return int(left.GetElementId() - right.GetElementId())
	})

	elementIDs := map[int32]struct{}{}
	for _, placement := range placements {
		elementIDs[placement.GetElementId()] = struct{}{}
	}

	connectors := make([]*diagv1.Connector, 0, len(content.GetConnectors()))
	for _, connector := range content.GetConnectors() {
		if _, ok := elementIDs[connector.GetSourceElementId()]; !ok {
			continue
		}
		if _, ok := elementIDs[connector.GetTargetElementId()]; !ok {
			continue
		}
		connectors = append(connectors, connector)
	}
	slices.SortFunc(connectors, func(left, right *diagv1.Connector) int {
		return int(left.GetId() - right.GetId())
	})

	lines := []string{"flowchart LR"}
	if includeTldMetadata {
		if viewID > 0 {
			lines = append(lines, fmt.Sprintf("%%%% tld/v1 view=%d", viewID))
		} else {
			lines = append(lines, "%% tld/v1")
		}
	}

	groups, groupByElement := exportGroups(placements, layers)
	for _, group := range groups {
		groupID := fmt.Sprintf("group_%d", group.layer.GetId())
		lines = append(lines, "", fmt.Sprintf(`  subgraph %s["%s"]`, groupID, escapeMermaidLabel(group.layer.GetName())))
		for _, placement := range group.members {
			lines = appendPlacement(lines, placement, includeTldMetadata, "    ")
		}
		lines = append(lines, "  end")
		if color := strings.TrimSpace(group.layer.GetColor()); mermaidGroupColorPattern.MatchString(color) {
			lines = append(lines, fmt.Sprintf("  style %s fill:%s26,stroke:%s", groupID, color, color))
		}
	}
	for _, placement := range placements {
		if _, grouped := groupByElement[placement.GetElementId()]; grouped {
			continue
		}
		if len(groups) > 0 && len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = appendPlacement(lines, placement, includeTldMetadata, "  ")
	}
	if len(placements) > 0 && len(connectors) > 0 {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
	}
	for _, connector := range connectors {
		sourceRef := fmt.Sprintf("node_%d", connector.GetSourceElementId())
		targetRef := fmt.Sprintf("node_%d", connector.GetTargetElementId())
		sourceID := sanitizeMermaidID(sourceRef)
		targetID := sanitizeMermaidID(targetRef)
		label := strings.TrimSpace(connector.GetLabel())
		if label != "" {
			lines = append(lines, fmt.Sprintf(`  %s -- "%s" --> %s`, sourceID, escapeMermaidLabel(label), targetID))
		} else {
			lines = append(lines, fmt.Sprintf("  %s --> %s", sourceID, targetID))
		}
		if includeTldMetadata {
			lines = append(lines, metadataComment("tld-connector", "", connectorMetadataEntries(connector, fmt.Sprintf("%d", connector.GetId()), sourceRef, targetRef)))
		}
	}

	return strings.Join(lines, "\n") + "\n"
}

func appendPlacement(lines []string, placement *diagv1.PlacedElement, includeTldMetadata bool, indent string) []string {
	ref := fmt.Sprintf("node_%d", placement.GetElementId())
	nodeID := sanitizeMermaidID(ref)
	lines = append(lines, fmt.Sprintf(`%s%s["%s"]`, indent, nodeID, escapeMermaidLabel(placement.GetName())))
	if includeTldMetadata {
		lines = append(lines, indent+metadataComment("tld-element", "", elementMetadataEntries(placement, ref)))
	}
	return lines
}

func exportGroups(placements []*diagv1.PlacedElement, layers []*diagv1.ViewLayer) ([]exportGroup, map[int32]struct{}) {
	orderedLayers := make([]*diagv1.ViewLayer, 0, len(layers))
	for _, layer := range layers {
		if layer != nil {
			orderedLayers = append(orderedLayers, layer)
		}
	}
	slices.SortFunc(orderedLayers, func(left, right *diagv1.ViewLayer) int {
		return int(left.GetId() - right.GetId())
	})
	groupByElement := make(map[int32]struct{})
	groups := make([]exportGroup, 0, len(orderedLayers))
	for _, layer := range orderedLayers {
		if len(layer.GetTags()) != 1 || !elementGroupMarkerPattern.MatchString(layer.GetTags()[0]) {
			continue
		}
		group := exportGroup{layer: layer, marker: layer.GetTags()[0]}
		for _, placement := range placements {
			if _, alreadyGrouped := groupByElement[placement.GetElementId()]; alreadyGrouped {
				continue
			}
			if slices.Contains(placement.GetTags(), group.marker) {
				group.members = append(group.members, placement)
				groupByElement[placement.GetElementId()] = struct{}{}
			}
		}
		if len(group.members) > 0 {
			groups = append(groups, group)
		}
	}
	return groups, groupByElement
}

func ExportMarkdownBlock(content *diagv1.ViewContent, viewID int32, includeTldMetadata bool) string {
	return MermaidBlock(ExportView(content, viewID, includeTldMetadata))
}

func elementMetadataEntries(element *diagv1.PlacedElement, ref string) []metadataEntry {
	entries := []metadataEntry{
		{Key: "ref", Value: EscapeMetadataValue(ref)},
		{Key: "x", Value: compactNumber(element.GetPositionX())},
		{Key: "y", Value: compactNumber(element.GetPositionY())},
	}
	if kind := strings.TrimSpace(element.GetKind()); kind != "" && kind != "system" {
		entries = append(entries, metadataEntry{Key: "kind", Value: EscapeMetadataValue(kind)})
	}
	appendStringEntry := func(key string, value string) {
		if strings.TrimSpace(value) != "" {
			entries = append(entries, metadataEntry{Key: key, Value: EscapeMetadataValue(strings.TrimSpace(value))})
		}
	}
	appendStringEntry("desc", element.GetDescription())
	appendStringEntry("tech", element.GetTechnology())
	appendStringEntry("url", element.GetUrl())
	appendStringEntry("logo", element.GetLogoUrl())
	visibleTags := make([]string, 0, len(element.GetTags()))
	for _, tag := range element.GetTags() {
		if !elementGroupMarkerPattern.MatchString(tag) {
			visibleTags = append(visibleTags, tag)
		}
	}
	if encoded := EncodeStringList(visibleTags); encoded != "" {
		entries = append(entries, metadataEntry{Key: "tags", Value: encoded})
	}
	if encoded := EncodeTechnologyLinks(element.GetTechnologyLinks()); encoded != "" {
		entries = append(entries, metadataEntry{Key: "techLinks", Value: encoded})
	}
	appendStringEntry("repo", element.GetRepo())
	appendStringEntry("branch", element.GetBranch())
	appendStringEntry("file", element.GetFilePath())
	appendStringEntry("lang", element.GetLanguage())
	if element.GetBypassNoiseGate() {
		entries = append(entries, metadataEntry{Key: "bypass", Value: "1"})
	}
	if element.GetHasView() {
		entries = append(entries, metadataEntry{Key: "hasView", Value: "1"})
	}
	appendStringEntry("viewLabel", element.GetViewLabel())
	return entries
}

func connectorMetadataEntries(connector *diagv1.Connector, ref, sourceRef, targetRef string) []metadataEntry {
	entries := []metadataEntry{
		{Key: "ref", Value: EscapeMetadataValue(ref)},
		{Key: "source", Value: EscapeMetadataValue(sourceRef)},
		{Key: "target", Value: EscapeMetadataValue(targetRef)},
	}
	appendStringEntry := func(key string, value string) {
		if strings.TrimSpace(value) != "" {
			entries = append(entries, metadataEntry{Key: key, Value: EscapeMetadataValue(strings.TrimSpace(value))})
		}
	}
	appendStringEntry("label", connector.GetLabel())
	appendStringEntry("desc", connector.GetDescription())
	appendStringEntry("rel", connector.GetRelationship())
	if direction := strings.TrimSpace(connector.GetDirection()); direction != "" && direction != "forward" {
		appendStringEntry("dir", direction)
	}
	if style := strings.TrimSpace(connector.GetStyle()); style != "" && style != "bezier" {
		appendStringEntry("style", style)
	}
	appendStringEntry("url", connector.GetUrl())
	if sourceHandle := strings.TrimSpace(connector.GetSourceHandle()); sourceHandle != "" && sourceHandle != "right" {
		appendStringEntry("sourceHandle", sourceHandle)
	}
	if targetHandle := strings.TrimSpace(connector.GetTargetHandle()); targetHandle != "" && targetHandle != "left" {
		appendStringEntry("targetHandle", targetHandle)
	}
	return entries
}

func metadataComment(kind, subject string, entries []metadataEntry) string {
	parts := []string{"%% " + kind}
	if subject != "" {
		parts = append(parts, subject)
	}
	for _, entry := range entries {
		parts = append(parts, entry.Key+"="+entry.Value)
	}
	return strings.Join(parts, " ")
}

func sanitizeMermaidID(value string) string {
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	sanitized := builder.String()
	if sanitized == "" {
		return "node"
	}
	first := sanitized[0]
	if (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') || first == '_' {
		return sanitized
	}
	return "node_" + sanitized
}

func EscapeMermaidLabel(value string) string {
	return escapeMermaidLabel(value)
}

func DecodeMermaidLabel(value string) string {
	value = strings.ReplaceAll(value, `\"`, `"`)
	value = strings.ReplaceAll(value, `\\`, `\`)
	return strings.TrimSpace(html.UnescapeString(value))
}

func escapeMermaidLabel(value string) string {
	replacer := strings.NewReplacer(
		"\r\n", " ",
		"\n", " ",
		"\r", " ",
		"&", "&amp;",
		`\`, `\\`,
		`"`, "&quot;",
	)
	return replacer.Replace(value)
}

func compactNumber(value float64) string {
	if !isFinite(value) {
		return "0"
	}
	if math.Trunc(value) == value {
		return fmt.Sprintf("%.0f", value)
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", value), "0"), ".")
}

func isFinite(value float64) bool {
	return !math.IsInf(value, 0) && !math.IsNaN(value)
}
