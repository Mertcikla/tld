package mermaid

import (
	"regexp"
	"strconv"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

type elementMetadata struct {
	Ref             string
	X               *float64
	Y               *float64
	Kind            string
	Description     string
	Technology      string
	URL             string
	LogoURL         string
	Tags            []string
	TechnologyLinks []*diagv1.TechnologyLink
	Repo            string
	Branch          string
	FilePath        string
	Language        string
	BypassNoiseGate *bool
	HasView         *bool
	ViewLabel       string
}

type connectorMetadata struct {
	Ref          string
	SourceRef    string
	TargetRef    string
	Label        string
	Description  string
	Relationship string
	Direction    string
	Style        string
	URL          string
	SourceHandle string
	TargetHandle string
}

type tldMetadata struct {
	Elements   map[string]elementMetadata
	Connectors []connectorMetadata
}

var (
	metadataPairPattern               = regexp.MustCompile(`(^|\s+)([A-Za-z][A-Za-z0-9_]*)=`)
	exportedFlowchartConnectorLineRe  = regexp.MustCompile(`^` + mermaidFlowchartIDPattern + `\s+(?:--\s+"(?:\\.|[^"\\])*"\s+-->|-->)\s+` + mermaidFlowchartIDPattern + `$`)
	exportedFlowchartElementRefLineRe = regexp.MustCompile(`^(` + mermaidFlowchartIDPattern + `)\s*\["(?:\\.|[^"\\])*"\]$`)
)

func ParseTldMetadata(source string) *tldMetadata {
	lines := strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n")
	metadata := &tldMetadata{
		Elements: map[string]elementMetadata{},
	}
	markerSeen := false
	lastElementRef := ""
	lastConnectorIndex := -1

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "%%") {
			if markerSeen {
				lastElementRef = exportedFlowchartElementRef(trimmed)
				if exportedFlowchartConnectorLineRe.MatchString(trimmed) {
					metadata.Connectors = append(metadata.Connectors, connectorMetadata{})
					lastConnectorIndex = len(metadata.Connectors) - 1
					lastElementRef = ""
				}
			}
			continue
		}

		body := strings.TrimSpace(strings.TrimPrefix(trimmed, "%%"))
		if regexp.MustCompile(`^tld/v1(?:\s|$)`).MatchString(body) {
			markerSeen = true
			lastElementRef = ""
			lastConnectorIndex = -1
			continue
		}
		if !markerSeen {
			continue
		}

		kind, text, ok := strings.Cut(body, " ")
		if !ok {
			kind = body
			text = ""
		}
		if kind != "tld-element" && kind != "tld-connector" {
			continue
		}
		pairs := parseMetadataPairs(text)
		if len(pairs) == 0 {
			continue
		}
		if kind == "tld-element" {
			if lastElementRef == "" {
				continue
			}
			metadata.Elements[lastElementRef] = parseElementMetadata(pairs)
			continue
		}
		item := parseConnectorMetadata(pairs)
		if lastConnectorIndex >= 0 {
			metadata.Connectors[lastConnectorIndex] = mergeConnectorMetadata(metadata.Connectors[lastConnectorIndex], item)
		} else {
			metadata.Connectors = append(metadata.Connectors, item)
		}
	}

	if !markerSeen {
		return nil
	}
	return metadata
}

func ApplyTldMetadata(parsed *ParsedDiagram, metadata *tldMetadata) {
	if metadata == nil || parsed == nil {
		return
	}

	explicitRefs := map[string]string{}
	for _, element := range parsed.Elements {
		item, ok := metadata.Elements[element.GetRef()]
		if !ok {
			continue
		}
		if item.Ref != "" && item.Ref != element.GetRef() {
			explicitRefs[element.GetRef()] = item.Ref
			element.Ref = item.Ref
		}
		if item.Kind != "" {
			element.Kind = &item.Kind
		}
		if item.Description != "" {
			element.Description = &item.Description
		}
		if item.Technology != "" {
			element.Technology = &item.Technology
		}
		if item.URL != "" {
			element.Url = &item.URL
		}
		if item.LogoURL != "" {
			element.LogoUrl = &item.LogoURL
		}
		if item.Tags != nil {
			element.Tags = item.Tags
		}
		if item.TechnologyLinks != nil {
			element.TechnologyLinks = item.TechnologyLinks
		}
		if item.Repo != "" {
			element.Repo = &item.Repo
		}
		if item.Branch != "" {
			element.Branch = &item.Branch
		}
		if item.FilePath != "" {
			element.FilePath = &item.FilePath
		}
		if item.Language != "" {
			element.Language = &item.Language
		}
		if item.BypassNoiseGate != nil {
			element.BypassNoiseGate = item.BypassNoiseGate
		}
		if item.HasView != nil {
			element.HasView = *item.HasView
		}
		if item.ViewLabel != "" {
			element.ViewLabel = &item.ViewLabel
		}
		if item.X != nil && item.Y != nil {
			placement := &diagv1.PlanViewPlacement{ParentRef: "root", PositionX: item.X, PositionY: item.Y}
			if len(element.GetPlacements()) > 0 {
				placement = element.GetPlacements()[0]
				if placement.ParentRef == "" {
					placement.ParentRef = "root"
				}
				placement.PositionX = item.X
				placement.PositionY = item.Y
			}
			element.Placements = []*diagv1.PlanViewPlacement{placement}
		}
	}

	if len(explicitRefs) > 0 {
		for _, connector := range parsed.Connectors {
			if next := explicitRefs[connector.GetSourceElementRef()]; next != "" {
				connector.SourceElementRef = next
			}
			if next := explicitRefs[connector.GetTargetElementRef()]; next != "" {
				connector.TargetElementRef = next
			}
			if next := explicitRefs[connector.GetViewRef()]; next != "" {
				connector.ViewRef = next
			}
		}
	}

	for index, connector := range parsed.Connectors {
		if index >= len(metadata.Connectors) {
			return
		}
		item := metadata.Connectors[index]
		if item.Ref != "" {
			connector.Ref = item.Ref
		}
		if item.SourceRef != "" {
			connector.SourceElementRef = item.SourceRef
		}
		if item.TargetRef != "" {
			connector.TargetElementRef = item.TargetRef
		}
		if item.Label != "" {
			connector.Label = &item.Label
		}
		if item.Description != "" {
			connector.Description = &item.Description
		}
		if item.Relationship != "" {
			connector.Relationship = &item.Relationship
		}
		if item.Direction != "" {
			connector.Direction = &item.Direction
		}
		if item.Style != "" {
			connector.Style = &item.Style
		}
		if item.URL != "" {
			connector.Url = &item.URL
		}
		if item.SourceHandle != "" {
			connector.SourceHandle = &item.SourceHandle
		}
		if item.TargetHandle != "" {
			connector.TargetHandle = &item.TargetHandle
		}
	}
}

func EscapeMetadataValue(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
		"=", `\=`,
		",", `\,`,
		"|", `\|`,
		":", `\:`,
	)
	return replacer.Replace(value)
}

func UnescapeMetadataValue(value string) (string, bool) {
	return unescapeMetadataValue(value)
}

func EncodeStringList(values []string) string {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		encoded = append(encoded, EscapeMetadataValue(value))
	}
	return strings.Join(encoded, ",")
}

func DecodeStringList(value string) ([]string, bool) {
	return decodeStringList(value)
}

func EncodeTechnologyLinks(values []*diagv1.TechnologyLink) string {
	encoded := make([]string, 0, len(values))
	for _, link := range values {
		if strings.TrimSpace(link.GetLabel()) == "" {
			continue
		}
		primary := "0"
		if link.GetIsPrimaryIcon() {
			primary = "1"
		}
		encoded = append(encoded, strings.Join([]string{
			EscapeMetadataValue(link.GetType()),
			EscapeMetadataValue(link.GetSlug()),
			EscapeMetadataValue(link.GetLabel()),
			primary,
		}, ":"))
	}
	return strings.Join(encoded, "|")
}

func parseMetadataPairs(text string) map[string]string {
	matches := metadataPairPattern.FindAllStringSubmatchIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}
	pairs := map[string]string{}
	for index, match := range matches {
		keyStart := match[4]
		keyEnd := match[5]
		valueStart := keyEnd + 1
		valueEnd := len(text)
		if index+1 < len(matches) {
			valueEnd = matches[index+1][0]
		}
		pairs[text[keyStart:keyEnd]] = text[valueStart:valueEnd]
	}
	return pairs
}

func parseElementMetadata(pairs map[string]string) elementMetadata {
	item := elementMetadata{}
	stringKeys := map[string]*string{
		"ref":       &item.Ref,
		"kind":      &item.Kind,
		"desc":      &item.Description,
		"tech":      &item.Technology,
		"url":       &item.URL,
		"logo":      &item.LogoURL,
		"repo":      &item.Repo,
		"branch":    &item.Branch,
		"file":      &item.FilePath,
		"lang":      &item.Language,
		"viewLabel": &item.ViewLabel,
	}
	for key, target := range stringKeys {
		if raw, ok := pairs[key]; ok {
			if decoded, ok := unescapeMetadataValue(raw); ok && decoded != "" {
				*target = decoded
			}
		}
	}
	if raw, ok := pairs["x"]; ok {
		if decoded, ok := decodeNumber(raw); ok {
			item.X = &decoded
		}
	}
	if raw, ok := pairs["y"]; ok {
		if decoded, ok := decodeNumber(raw); ok {
			item.Y = &decoded
		}
	}
	if raw, ok := pairs["tags"]; ok {
		if decoded, ok := decodeStringList(raw); ok && len(decoded) > 0 {
			item.Tags = decoded
		}
	}
	if raw, ok := pairs["techLinks"]; ok {
		if decoded, ok := decodeTechnologyLinks(raw); ok && len(decoded) > 0 {
			item.TechnologyLinks = decoded
		}
	}
	if raw, ok := pairs["bypass"]; ok {
		if decoded, ok := decodeBool(raw); ok {
			item.BypassNoiseGate = &decoded
		}
	}
	if raw, ok := pairs["hasView"]; ok {
		if decoded, ok := decodeBool(raw); ok {
			item.HasView = &decoded
		}
	}
	return item
}

func parseConnectorMetadata(pairs map[string]string) connectorMetadata {
	item := connectorMetadata{}
	stringKeys := map[string]*string{
		"ref":          &item.Ref,
		"source":       &item.SourceRef,
		"target":       &item.TargetRef,
		"label":        &item.Label,
		"desc":         &item.Description,
		"rel":          &item.Relationship,
		"dir":          &item.Direction,
		"style":        &item.Style,
		"url":          &item.URL,
		"sourceHandle": &item.SourceHandle,
		"targetHandle": &item.TargetHandle,
	}
	for key, target := range stringKeys {
		if raw, ok := pairs[key]; ok {
			if decoded, ok := unescapeMetadataValue(raw); ok && decoded != "" {
				*target = decoded
			}
		}
	}
	return item
}

func mergeConnectorMetadata(base, override connectorMetadata) connectorMetadata {
	if override.Ref != "" {
		base.Ref = override.Ref
	}
	if override.SourceRef != "" {
		base.SourceRef = override.SourceRef
	}
	if override.TargetRef != "" {
		base.TargetRef = override.TargetRef
	}
	if override.Label != "" {
		base.Label = override.Label
	}
	if override.Description != "" {
		base.Description = override.Description
	}
	if override.Relationship != "" {
		base.Relationship = override.Relationship
	}
	if override.Direction != "" {
		base.Direction = override.Direction
	}
	if override.Style != "" {
		base.Style = override.Style
	}
	if override.URL != "" {
		base.URL = override.URL
	}
	if override.SourceHandle != "" {
		base.SourceHandle = override.SourceHandle
	}
	if override.TargetHandle != "" {
		base.TargetHandle = override.TargetHandle
	}
	return base
}

func unescapeMetadataValue(value string) (string, bool) {
	var out strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' {
			out.WriteByte(value[index])
			continue
		}
		index++
		if index >= len(value) {
			return "", false
		}
		switch value[index] {
		case '\\', '=', ',', '|', ':':
			out.WriteByte(value[index])
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		default:
			return "", false
		}
	}
	return out.String(), true
}

func splitEscaped(value string, separator byte) []string {
	parts := []string{}
	var current strings.Builder
	for index := 0; index < len(value); index++ {
		char := value[index]
		if char == '\\' && index+1 < len(value) {
			current.WriteByte(char)
			current.WriteByte(value[index+1])
			index++
			continue
		}
		if char == separator {
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteByte(char)
	}
	parts = append(parts, current.String())
	return parts
}

func decodeStringList(value string) ([]string, bool) {
	if value == "" {
		return []string{}, true
	}
	out := []string{}
	for _, raw := range splitEscaped(value, ',') {
		item, ok := unescapeMetadataValue(raw)
		if !ok {
			return nil, false
		}
		out = append(out, item)
	}
	return out, true
}

func decodeTechnologyLinks(value string) ([]*diagv1.TechnologyLink, bool) {
	if value == "" {
		return []*diagv1.TechnologyLink{}, true
	}
	out := []*diagv1.TechnologyLink{}
	for _, raw := range splitEscaped(value, '|') {
		fields := splitEscaped(raw, ':')
		if len(fields) < 3 || len(fields) > 4 {
			return nil, false
		}
		linkType, ok := unescapeMetadataValue(fields[0])
		if !ok {
			return nil, false
		}
		slug, ok := unescapeMetadataValue(fields[1])
		if !ok {
			return nil, false
		}
		label, ok := unescapeMetadataValue(fields[2])
		if !ok || strings.TrimSpace(label) == "" {
			return nil, false
		}
		primary := "0"
		if len(fields) == 4 {
			primary, ok = unescapeMetadataValue(fields[3])
			if !ok {
				return nil, false
			}
		}
		if linkType != "catalog" && linkType != "custom" {
			return nil, false
		}
		out = append(out, &diagv1.TechnologyLink{
			Type:          linkType,
			Slug:          ptrString(slug),
			Label:         label,
			IsPrimaryIcon: primary == "1" || primary == "true",
		})
	}
	return out, true
}

func decodeNumber(value string) (float64, bool) {
	raw, ok := unescapeMetadataValue(value)
	if !ok {
		return 0, false
	}
	number, err := strconv.ParseFloat(raw, 64)
	return number, err == nil
}

func decodeBool(value string) (bool, bool) {
	raw, ok := unescapeMetadataValue(value)
	if !ok {
		return false, false
	}
	switch raw {
	case "1", "true":
		return true, true
	case "0", "false":
		return false, true
	default:
		return false, false
	}
}

func exportedFlowchartElementRef(line string) string {
	match := exportedFlowchartElementRefLineRe.FindStringSubmatch(line)
	if match == nil {
		return ""
	}
	return match[1]
}
