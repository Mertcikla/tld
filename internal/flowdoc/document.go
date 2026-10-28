// Package flowdoc implements the portable tld-flow v1 format shared with the UI.
package flowdoc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"unicode/utf8"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/api"
	"gopkg.in/yaml.v3"
)

const MaxBytes = 1 << 20
const SchemaURL = "https://raw.githubusercontent.com/Mertcikla/tld/refs/heads/main/schemas/flow.schema.json"

type Document struct {
	Format      string     `json:"format" yaml:"format"`
	Version     int        `json:"version" yaml:"version"`
	Name        string     `json:"name" yaml:"name"`
	Description string     `json:"description,omitempty" yaml:"description,omitempty"`
	Steps       []Step     `json:"steps" yaml:"steps"`
	Resources   *Resources `json:"resources,omitempty" yaml:"resources,omitempty"`
}
type Step struct {
	ID          string       `json:"id,omitempty" yaml:"id,omitempty"`
	Title       string       `json:"title,omitempty" yaml:"title,omitempty"`
	Description string       `json:"description,omitempty" yaml:"description,omitempty"`
	Elements    []string     `json:"elements,omitempty" yaml:"elements,omitempty"`
	Connectors  []Connection `json:"connectors,omitempty" yaml:"connectors,omitempty"`
	Branches    *Branches    `json:"branches,omitempty" yaml:"branches,omitempty"`
	GoToFlow    *Link        `json:"goToFlow,omitempty" yaml:"goToFlow,omitempty"`
}
type Connection struct {
	Source string  `json:"source" yaml:"source"`
	Target string  `json:"target" yaml:"target"`
	Label  *string `json:"label,omitempty" yaml:"label,omitempty"`
}
type Branches struct {
	Mode  string `json:"mode" yaml:"mode"`
	Paths []Path `json:"paths" yaml:"paths"`
}
type Path struct {
	ID    string `json:"id,omitempty" yaml:"id,omitempty"`
	Name  string `json:"name" yaml:"name"`
	Steps []Step `json:"steps" yaml:"steps"`
}
type Link struct {
	Name string `json:"name" yaml:"name"`
	View string `json:"view,omitempty" yaml:"view,omitempty"`
}
type Resources struct {
	Elements   []Element   `json:"elements" yaml:"elements"`
	Connectors []Connector `json:"connectors" yaml:"connectors"`
}
type Element struct {
	Name                 string       `json:"name" yaml:"name"`
	Kind                 string       `json:"kind,omitempty" yaml:"kind,omitempty"`
	Description          string       `json:"description,omitempty" yaml:"description,omitempty"`
	Technology           string       `json:"technology,omitempty" yaml:"technology,omitempty"`
	URL                  string       `json:"url,omitempty" yaml:"url,omitempty"`
	LogoURL              string       `json:"logo_url,omitempty" yaml:"logo_url,omitempty"`
	Tags                 []string     `json:"tags,omitempty" yaml:"tags,omitempty"`
	X                    *float64     `json:"position_x,omitempty" yaml:"position_x,omitempty"`
	Y                    *float64     `json:"position_y,omitempty" yaml:"position_y,omitempty"`
	TechnologyConnectors []Technology `json:"technology_connectors,omitempty" yaml:"technology_connectors,omitempty"`
}
type Technology struct {
	Type    string `json:"type" yaml:"type"`
	Label   string `json:"label" yaml:"label"`
	Slug    string `json:"slug,omitempty" yaml:"slug,omitempty"`
	Primary bool   `json:"is_primary_icon,omitempty" yaml:"is_primary_icon,omitempty"`
}
type Connector struct {
	Source       string   `json:"source" yaml:"source"`
	Target       string   `json:"target" yaml:"target"`
	Label        *string  `json:"label,omitempty" yaml:"label,omitempty"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	Relationship string   `json:"relationship,omitempty" yaml:"relationship,omitempty"`
	Direction    string   `json:"direction,omitempty" yaml:"direction,omitempty"`
	Style        string   `json:"style,omitempty" yaml:"style,omitempty"`
	URL          string   `json:"url,omitempty" yaml:"url,omitempty"`
	SourceHandle string   `json:"source_handle,omitempty" yaml:"source_handle,omitempty"`
	TargetHandle string   `json:"target_handle,omitempty" yaml:"target_handle,omitempty"`
	Tags         []string `json:"tags,omitempty" yaml:"tags,omitempty"`
}
type Catalog struct {
	Flows []*diagv1.Flow
	Views []*diagv1.View
}

// Parse rejects aliases, duplicate/unknown fields, multiple documents and loose
// scalar coercions before resolving any target references.
func Parse(source []byte) (*Document, error) {
	if len(source) > MaxBytes || !utf8.Valid(source) {
		return nil, fmt.Errorf("flow YAML must be valid UTF-8 and at most 1 MiB")
	}
	dec := yaml.NewDecoder(bytes.NewReader(source))
	var root yaml.Node
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	var trailing yaml.Node
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("flow requires exactly one YAML document")
	}
	value, err := nodeValue(&root, 0)
	if err != nil {
		return nil, err
	}
	schemaBytes, err := assets.FS.ReadFile("schemas/flow.schema.json")
	if err != nil {
		return nil, err
	}
	var schema map[string]any
	if err = json.Unmarshal(schemaBytes, &schema); err != nil {
		return nil, err
	}
	if err = validateSchema(value, schema, schema, "flow"); err != nil {
		return nil, err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var doc Document
	if err = json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if err = validateDocument(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}
func nodeValue(n *yaml.Node, depth int) (any, error) {
	if depth > 80 {
		return nil, fmt.Errorf("flow YAML is too deeply nested")
	}
	if n.Anchor != "" || n.Kind == yaml.AliasNode {
		return nil, fmt.Errorf("flow YAML anchors and aliases are unsupported")
	}
	switch n.Kind {
	case yaml.DocumentNode:
		if len(n.Content) != 1 {
			return nil, fmt.Errorf("expected a flow mapping")
		}
		return nodeValue(n.Content[0], depth+1)
	case yaml.MappingNode:
		result := map[string]any{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag != "!!str" {
				return nil, fmt.Errorf("mapping keys must be strings")
			}
			if _, ok := result[k.Value]; ok {
				return nil, fmt.Errorf("duplicate field %q", k.Value)
			}
			v, err := nodeValue(n.Content[i+1], depth+1)
			if err != nil {
				return nil, err
			}
			result[k.Value] = v
		}
		return result, nil
	case yaml.SequenceNode:
		result := []any{}
		for _, child := range n.Content {
			v, err := nodeValue(child, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, v)
		}
		return result, nil
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!str":
			return n.Value, nil
		case "!!bool":
			var v bool
			err := n.Decode(&v)
			return v, err
		case "!!int", "!!float":
			var v float64
			if err := n.Decode(&v); err != nil {
				return nil, err
			}
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, fmt.Errorf("expected a finite number")
			}
			return v, nil
		}
	}
	return nil, fmt.Errorf("unsupported YAML value on line %d", n.Line)
}
func validateSchema(v any, s, root map[string]any, path string) error {
	if ref, ok := s["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/$defs/")
		return validateSchema(v, root["$defs"].(map[string]any)[name].(map[string]any), root, path)
	}
	fail := func(message string) error { return fmt.Errorf("%s: %s", path, message) }
	if c, ok := s["const"]; ok {
		if fmt.Sprint(c) != fmt.Sprint(v) {
			return fail(fmt.Sprintf("expected %v", c))
		}
	}
	if enums, ok := s["enum"].([]any); ok {
		match := false
		for _, e := range enums {
			match = match || reflect.DeepEqual(e, v)
		}
		if !match {
			return fail("unsupported value")
		}
	}
	switch s["type"] {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			return fail("expected a mapping")
		}
		if required, ok := s["required"].([]any); ok {
			for _, key := range required {
				if _, ok := obj[key.(string)]; !ok {
					return fail("missing field " + key.(string))
				}
			}
		}
		props, _ := s["properties"].(map[string]any)
		for key, value := range obj {
			prop, ok := props[key]
			if !ok {
				return fail("unknown field " + key)
			}
			if err := validateSchema(value, prop.(map[string]any), root, path+"."+key); err != nil {
				return err
			}
		}
	case "array":
		items, ok := v.([]any)
		if !ok {
			return fail("expected a sequence")
		}
		if maxN, ok := s["maxItems"].(float64); ok && len(items) > int(maxN) {
			return fail("too many entries")
		}
		if minN, ok := s["minItems"].(float64); ok && len(items) < int(minN) {
			return fail("too few entries")
		}
		seen := map[string]bool{}
		for i, item := range items {
			data, _ := json.Marshal(item)
			key := string(data)
			if s["uniqueItems"] == true && seen[key] {
				return fail("duplicate entry")
			}
			seen[key] = true
			if err := validateSchema(item, s["items"].(map[string]any), root, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case "string":
		text, ok := v.(string)
		if !ok {
			return fail("expected a string")
		}
		if n, ok := s["maxLength"].(float64); ok && len(text) > int(n) {
			return fail("string exceeds UTF-8 byte limit")
		}
		if s["pattern"] != nil && strings.TrimSpace(text) == "" {
			return fail("expected a nonempty string")
		}
	case "number":
		if _, ok := v.(float64); !ok {
			return fail("expected a number")
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fail("expected a boolean")
		}
	}
	return nil
}
func validateDocument(doc *Document) error {
	ids := map[string]bool{}
	count := 0
	var visit func([]Step, int) error
	visit = func(steps []Step, depth int) error {
		if depth > 8 {
			return fmt.Errorf("branches may be nested at most eight levels")
		}
		claim := func(id string) error {
			if id != "" {
				if ids[id] {
					return fmt.Errorf("duplicate step or path id %q", id)
				}
				ids[id] = true
			}
			return nil
		}
		for _, s := range steps {
			count++
			if count > 1000 {
				return fmt.Errorf("at most 1000 total steps are allowed")
			}
			if err := claim(s.ID); err != nil {
				return err
			}
			if s.Branches != nil && s.GoToFlow != nil {
				return fmt.Errorf("choose branches or a linked flow")
			}
			seen := map[string]bool{}
			for _, c := range s.Connectors {
				key := connectionKey(c.Source, c.Target, c.Label)
				if seen[key] {
					return fmt.Errorf("duplicate connection highlight")
				}
				seen[key] = true
			}
			if s.Branches != nil {
				for _, p := range s.Branches.Paths {
					if err := claim(p.ID); err != nil {
						return err
					}
					if err := visit(p.Steps, depth+1); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := visit(doc.Steps, 0); err != nil {
		return err
	}
	if doc.Resources != nil {
		names := map[string]bool{}
		for _, e := range doc.Resources.Elements {
			if names[e.Name] {
				return fmt.Errorf("duplicate resource element %q", e.Name)
			}
			names[e.Name] = true
			if (e.X == nil) != (e.Y == nil) {
				return fmt.Errorf("supply both position coordinates")
			}
		}
		seen := map[string]bool{}
		for _, c := range doc.Resources.Connectors {
			if !names[c.Source] || !names[c.Target] {
				return fmt.Errorf("connector endpoints must be included in resources.elements")
			}
			key := connectionKey(c.Source, c.Target, c.Label)
			if seen[key] {
				return fmt.Errorf("duplicate resource connection")
			}
			seen[key] = true
		}
	}
	return nil
}
func connectionKey(source, target string, label *string) string {
	l := ""
	if label != nil {
		l = *label
	}
	data, _ := json.Marshal([]string{source, target, l})
	return string(data)
}
func fresh(id string) string {
	if id != "" {
		return id
	}
	return uuid.NewString()
}

func ValidateSave(flow *diagv1.Flow) error {
	return api.ValidateFlowRequest(&diagv1.SaveFlowRequest{Id: flow.Id, ViewId: flow.ViewId, ExpectedRevision: flow.Revision, Content: flow.Content})
}
