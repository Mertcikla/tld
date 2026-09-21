package tech

import (
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

// TechnologyLinksForElement maps a technology label + language to catalog links.
// Exported for use by synchronous single-resource commands.
func TechnologyLinksForElement(technology, language string) []*diagv1.TechnologyLink {
	return technologyLinksForElement(technology, language)
}

func technologyLinksForElement(technology, language string) []*diagv1.TechnologyLink {
	links := technologyLinksForLabel(technology)
	if len(links) > 0 {
		return links
	}
	if strings.TrimSpace(technology) != "" {
		return nil
	}
	return technologyLinksForLanguage(language)
}

func technologyLinksForLabel(label string) []*diagv1.TechnologyLink {
	var links []*diagv1.TechnologyLink
	seen := map[string]struct{}{}
	hasPrimary := false
	for _, part := range technologyLabelParts(label) {
		slug, displayLabel := technologyCatalogMatch(part)
		if slug == "" {
			links = append(links, customTechnologyLink(part))
			if len(links) == 3 {
				break
			}
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		links = append(links, catalogTechnologyLink(slug, displayLabel, !hasPrimary))
		hasPrimary = true
		if len(links) == 3 {
			break
		}
	}
	return links
}

func technologyLinksForLanguage(language string) []*diagv1.TechnologyLink {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "go":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("go", "Go", true)}
	case "typescript":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("typescript", "TypeScript", true)}
	case "javascript":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("javascript", "JavaScript", true)}
	case "python":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("python", "Python", true)}
	case "java":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("java", "Java", true)}
	case "cpp":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("cplusplus", "C++", true)}
	case "c":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("c", "C", true)}
	case "json":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("json", "JSON", true)}
	default:
		return nil
	}
}

func technologyLabelParts(label string) []string {
	parts := strings.FieldsFunc(label, func(r rune) bool {
		return r == ',' || r == '/' || r == ';' || r == '|'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 && strings.TrimSpace(label) != "" {
		return []string{strings.TrimSpace(label)}
	}
	return out
}

func technologyCatalogMatch(label string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "container":
		return "docker", "Container"
	default:
		slug, name, ok := LookupCatalogFuzzy(label)
		if !ok {
			return "", ""
		}
		return slug, name
	}
}

func catalogTechnologyLink(slug, label string, primary bool) *diagv1.TechnologyLink {
	return &diagv1.TechnologyLink{
		Type:          "catalog",
		Slug:          &slug,
		Label:         label,
		IsPrimaryIcon: primary,
	}
}

func customTechnologyLink(label string) *diagv1.TechnologyLink {
	return &diagv1.TechnologyLink{
		Type:  "custom",
		Label: label,
	}
}
