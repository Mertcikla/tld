package workspace

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mertcikla/diag/tld/internal/analyzer"
)

// ValidationError describes a single validation failure.
type ValidationError struct {
	Location string
	Message  string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Location, e.Message)
}

// ValidationOptions allows configuring the validation process.
type ValidationOptions struct {
	SkipSymbols bool
}

// Validate checks cross-reference integrity, required fields, and cycle
// detection in parent_diagram chains.
func (ws *Workspace) Validate() []ValidationError {
	// Default to checking symbols for backward compatibility and strictness by default.
	return ws.ValidateWithOpts(ValidationOptions{SkipSymbols: false})
}

// ValidateWithOpts checks cross-reference integrity with configurable options.
func (ws *Workspace) ValidateWithOpts(opts ValidationOptions) []ValidationError {
	var errs []ValidationError
	errs = append(errs, ws.validateConflictMarkers()...)

	// Elements: required fields + placement refs
	elementNames := make(map[string]string)
	for ref, element := range ws.Elements {
		loc := fmt.Sprintf("elements.yaml[%s]", ref)
		if element.Name == "" {
			errs = append(errs, ValidationError{loc, "name is required"})
		} else {
			if existingRef, ok := elementNames[element.Name]; ok {
				errs = append(errs, ValidationError{loc, fmt.Sprintf("duplicate element name %q (also used by %q)", element.Name, existingRef)})
			}
			elementNames[element.Name] = ref
		}
		if element.Kind == "" {
			errs = append(errs, ValidationError{loc, "kind is required"})
		}
		if element.Owner != "" && ws.WorkspaceConfig != nil {
			if _, ok := ws.WorkspaceConfig.Repositories[element.Owner]; !ok {
				errs = append(errs, ValidationError{loc, fmt.Sprintf("owner %q is not a registered repository", element.Owner)})
			}
		}
		for index, placement := range element.Placements {
			ploc := fmt.Sprintf("elements.yaml[%s][placements][%d]", ref, index)
			if placement.ParentRef == "" {
				errs = append(errs, ValidationError{ploc, "parent is required"})
				continue
			}
			if placement.ParentRef != "root" {
				if _, ok := ws.Elements[placement.ParentRef]; !ok {
					errs = append(errs, ValidationError{ploc, fmt.Sprintf("parent ref %q not found", placement.ParentRef)})
				}
			}
		}
	}

	// Connectors: required fields + ref integrity
	for ref, connector := range ws.Connectors {
		loc := fmt.Sprintf("connectors.yaml[%s]", ref)
		if connector.View == "" {
			errs = append(errs, ValidationError{loc, "view is required"})
		} else if connector.View != "root" {
			if _, ok := ws.Elements[connector.View]; !ok {
				errs = append(errs, ValidationError{loc, fmt.Sprintf("view ref %q not found", connector.View)})
			}
		}
		if connector.Source == "" {
			errs = append(errs, ValidationError{loc, "source is required"})
		} else if _, ok := ws.Elements[connector.Source]; !ok {
			errs = append(errs, ValidationError{loc, fmt.Sprintf("source ref %q not found", connector.Source)})
		}
		if connector.Target == "" {
			errs = append(errs, ValidationError{loc, "target is required"})
		} else if _, ok := ws.Elements[connector.Target]; !ok {
			errs = append(errs, ValidationError{loc, fmt.Sprintf("target ref %q not found", connector.Target)})
		}
	}

	// Symbol verification: for elements that declare both file_path and symbol,
	// confirm the named symbol actually exists in the file (skip if file not locally accessible).
	if !opts.SkipSymbols {
		errs = append(errs, ws.validateSymbols()...)
	}

	return errs
}

func (ws *Workspace) validateConflictMarkers() []ValidationError {
	var errs []ValidationError
	checkString := func(location, value string) {
		if strings.Contains(value, "<<< LOCAL") || strings.Contains(value, ">>> SERVER") {
			errs = append(errs, ValidationError{Location: location, Message: "unresolved merge conflict"})
		}
	}

	for ref, element := range ws.Elements {
		loc := fmt.Sprintf("elements.yaml[%s]", ref)
		checkString(loc, element.Name)
		checkString(loc, element.Description)
		checkString(loc, element.Technology)
		checkString(loc, element.URL)
	}
	for ref, connector := range ws.Connectors {
		loc := fmt.Sprintf("connectors.yaml[%s]", ref)
		checkString(loc, connector.Label)
		checkString(loc, connector.Description)
		checkString(loc, connector.Relationship)
	}
	return errs
}

// validateSymbols checks that elements with both file_path and symbol fields
// have a symbol that is actually present in the referenced file.
// Files that do not exist locally are silently skipped.
func (ws *Workspace) validateSymbols() []ValidationError {
	var errs []ValidationError
	ctx := context.Background()

	for ref, element := range ws.Elements {
		if element.FilePath == "" || element.Symbol == "" {
			continue
		}
		if _, err := os.Stat(element.FilePath); err != nil {
			continue // file not accessible locally skip
		}
		found, err := analyzer.HasSymbol(ctx, element.FilePath, element.Symbol)
		if err != nil {
			if analyzer.IsUnsupportedLanguage(err) {
				continue // language not supported skip silently
			}
			errs = append(errs, ValidationError{
				Location: fmt.Sprintf("elements.yaml[%s]", ref),
				Message:  fmt.Sprintf("symbol verification failed: %v", err),
			})
			continue
		}
		if !found {
			errs = append(errs, ValidationError{
				Location: fmt.Sprintf("elements.yaml[%s]", ref),
				Message:  fmt.Sprintf("symbol %q not found in %s", element.Symbol, element.FilePath),
			})
		}
	}
	return errs
}
