package semantic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/semantic/semanticpb"
)

func FlattenMarkdown(symbol *semanticpb.SymbolContext) string {
	if symbol == nil {
		return ""
	}
	var lines []string
	lines = append(lines,
		"# "+symbol.GetSymbolName(),
		"",
		"Domain: "+symbol.GetDomainBoundary(),
		"",
		"## Identity",
		"- URN: `"+symbol.GetUrn()+"`",
		"- Entity type: `"+symbol.GetEntityType().String()+"`",
		"- Source language: `"+symbol.GetSourceLanguage()+"`",
	)
	if symbol.GetRawSignature() != "" {
		lines = append(lines, "", "## Signature", "```", stripSignature(symbol.GetRawSignature()), "```")
	}
	if symbol.GetNaturalLanguageDocs() != "" {
		lines = append(lines, "", "## Documentation", cleanDoc(symbol.GetNaturalLanguageDocs()))
	}
	if len(symbol.GetProperties()) > 0 {
		lines = append(lines, "", "## Properties")
		for _, property := range symbol.GetProperties() {
			lines = append(lines, "- "+propertyLabel(property))
		}
	}
	if len(symbol.GetRelationships()) > 0 {
		lines = append(lines, "", "## Dependencies")
		for _, rel := range symbol.GetRelationships() {
			label := "Depends On:"
			if rel.GetRelationshipType() == "consumed_by" {
				label = "Consumed By:"
			}
			detail := strings.TrimSpace(rel.GetDescription())
			if detail != "" {
				detail = " - " + detail
			}
			lines = append(lines, fmt.Sprintf("- %s `%s`%s", label, rel.GetTargetUrn(), detail))
		}
	}
	if system := symbol.GetSystemContext(); system != nil {
		lines = appendSystemSection(lines, system)
	}
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}

func propertyLabel(property *semanticpb.Property) string {
	if property == nil {
		return ""
	}
	label := property.GetName()
	if property.GetValue() != "" {
		label += ": " + property.GetValue()
	}
	if property.GetKind() != "" {
		label += " (" + property.GetKind() + ")"
	}
	return label
}

func appendSystemSection(lines []string, system *semanticpb.SystemContext) []string {
	hasSystem := len(system.GetExecutionBoundaries()) > 0 || len(system.GetTriggers()) > 0 || len(system.GetExternalBindings()) > 0 || len(system.GetCustomAttributes()) > 0
	if !hasSystem {
		return lines
	}
	lines = append(lines, "", "## System Context")
	for _, value := range system.GetExecutionBoundaries() {
		lines = append(lines, "- Execution boundary: "+value)
	}
	for _, value := range system.GetTriggers() {
		lines = append(lines, "- Trigger: "+value)
	}
	for _, value := range system.GetExternalBindings() {
		lines = append(lines, "- External binding: "+value)
	}
	keys := make([]string, 0, len(system.GetCustomAttributes()))
	for key := range system.GetCustomAttributes() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		lines = append(lines, "- "+key+": "+system.GetCustomAttributes()[key])
	}
	return lines
}
