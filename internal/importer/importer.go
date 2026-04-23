package importer

import (
	"strings"
)

type ParsedElement struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Shape       string `json:"shape"`
	Description string `json:"description"`
	Technology  string `json:"technology"`
}

type ParsedConnector struct {
	SourceID   string `json:"source_id"`
	TargetID   string `json:"target_id"`
	Label      string `json:"label"`
	Technology string `json:"technology"`
}

type ParsedWorkspace struct {
	Elements   []ParsedElement   `json:"elements"`
	Connectors []ParsedConnector `json:"connectors"`
	Warnings   []string          `json:"warnings"`
}

type Importer interface {
	Parse(input string) (*ParsedWorkspace, error)
}

func DetectFormat(input string) string {
	if strings.Contains(input, "architecture-beta") {
		return "mermaid"
	}
	if strings.Contains(input, "workspace") || strings.Contains(input, "model") {
		return "structurizr"
	}
	return "structurizr" // default to structurizr if it has model/workspace etc.
}

func Parse(input string) (*ParsedWorkspace, error) {
	format := DetectFormat(input)
	if format == "structurizr" {
		return ParseStructurizr(input)
	}
	return &ParsedWorkspace{
		Warnings: []string{"Format not fully supported. Using default Structurizr parser."},
	}, nil
}
