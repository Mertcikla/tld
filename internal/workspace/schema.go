package workspace

import (
	"fmt"
	"path/filepath"
)

// Schema URLs for the YAML files the CLI writes. Editor tooling such as the
// YAML language server (VS Code, Neovim) validates and autocompletes workspace
// files when a "# yaml-language-server: $schema=..." comment is present. The
// schemas are published in the open-source repository under schemas/.
const (
	SchemaBaseURL = "https://raw.githubusercontent.com/Mertcikla/tld/refs/heads/main/schemas"

	ElementsSchemaURL   = SchemaBaseURL + "/elements.schema.json"
	ConnectorsSchemaURL = SchemaBaseURL + "/connectors.schema.json"
)

// schemaDirective returns the yaml-language-server schema comment for a
// workspace file, or "" when the file has no published schema.
func schemaDirective(filename string) string {
	switch filename {
	case "elements.yaml":
		return fmt.Sprintf("# yaml-language-server: $schema=%s", ElementsSchemaURL)
	case "connectors.yaml":
		return fmt.Sprintf("# yaml-language-server: $schema=%s", ConnectorsSchemaURL)
	default:
		return ""
	}
}

// SchemaComment returns the yaml-language-server schema comment line (without a
// trailing newline) for a workspace YAML file, or "" when none applies.
func SchemaComment(filename string) string {
	return schemaDirective(filepath.Base(filename))
}
