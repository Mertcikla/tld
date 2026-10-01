package parser

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"gopkg.in/yaml.v3"
)

func openAPIYAMLFacts(root *yaml.Node, rel, subject string) []Fact {
	if field(root, "openapi") == nil && field(root, "swagger") == nil {
		return nil
	}
	paths := field(root, "paths")
	var out []Fact
	if paths != nil && paths.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(paths.Content); i += 2 {
			key := paths.Content[i]
			out = append(out, nodeInfra(pb.FactKind_FACT_KIND_ROUTE, subject, normalizeRoute(key.Value), rel, key))
		}
	}
	components := field(root, "components")
	schemas := field(components, "schemas")
	if schemas != nil && schemas.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(schemas.Content); i += 2 {
			key := schemas.Content[i]
			out = append(out, nodeInfra(pb.FactKind_FACT_KIND_SCHEMA, subject, key.Value, rel, key))
		}
	}
	return out
}

func openAPIJSONFacts(path, rel, subject string) []Fact {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil
	}
	var doc struct {
		OpenAPI    string                     `json:"openapi"`
		Swagger    string                     `json:"swagger"`
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]json.RawMessage `json:"schemas"`
		} `json:"components"`
	}
	if json.Unmarshal(b, &doc) != nil || (doc.OpenAPI == "" && doc.Swagger == "") {
		return nil
	}
	var out []Fact
	for route := range doc.Paths {
		line := lineOfString(b, "\""+route+"\"")
		out = append(out, mkInfra(pb.FactKind_FACT_KIND_ROUTE, subject, normalizeRoute(route), rel, line, sourceLine(b, line), "structured"))
	}
	for schema := range doc.Components.Schemas {
		line := lineOfString(b, `"`+schema+`"`)
		out = append(out, mkInfra(pb.FactKind_FACT_KIND_SCHEMA, subject, schema, rel, line, sourceLine(b, line), "structured"))
	}
	return out
}

var protoService = regexp.MustCompile(`^service\s+([A-Za-z_][A-Za-z0-9_]*)`)
var protoRPC = regexp.MustCompile(`^rpc\s+([A-Za-z_][A-Za-z0-9_]*)`)

func protoFacts(path, rel, subject string) []Fact {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil
	}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	var out []Fact
	service := subject
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if m := protoService.FindStringSubmatch(raw); len(m) > 0 {
			service = m[1]
		}
		if m := protoRPC.FindStringSubmatch(raw); len(m) > 0 {
			out = append(out, mkInfra(pb.FactKind_FACT_KIND_RPC, service, m[1], rel, line, raw, "structured"))
		}
	}
	return out
}

var goWorkUse = regexp.MustCompile(`(?m)^\s*(?:use\s+)?(\./[^\s)]+)\s*$`)

func workspaceFacts(path, rel string) []Fact {
	base := filepath.Base(path)
	b, e := os.ReadFile(path)
	if e != nil {
		return nil
	}
	var out []Fact
	switch base {
	case "go.work":
		lines := bytes.Split(b, []byte("\n"))
		for i, line := range lines {
			m := goWorkUse.FindSubmatch(line)
			if len(m) > 0 {
				out = append(out, mkInfra(pb.FactKind_FACT_KIND_WORKSPACE_MEMBER, "root", string(m[1]), rel, i+1, strings.TrimSpace(string(line)), "local"))
			}
		}
	case "pnpm-workspace.yaml":
		var doc yaml.Node
		if yaml.Unmarshal(b, &doc) == nil && len(doc.Content) > 0 {
			for _, pkg := range values(field(doc.Content[0], "packages")) {
				line := lineOfString(b, pkg)
				out = append(out, mkInfra(pb.FactKind_FACT_KIND_WORKSPACE_MEMBER, "root", pkg, rel, line, sourceLine(b, line), "local"))
			}
		}
	case "package.json":
		var doc struct {
			Workspaces json.RawMessage `json:"workspaces"`
		}
		if json.Unmarshal(b, &doc) == nil && len(doc.Workspaces) > 0 {
			var direct []string
			if json.Unmarshal(doc.Workspaces, &direct) != nil {
				var nested struct {
					Packages []string `json:"packages"`
				}
				_ = json.Unmarshal(doc.Workspaces, &nested)
				direct = nested.Packages
			}
			for _, pkg := range direct {
				line := lineOfString(b, pkg)
				out = append(out, mkInfra(pb.FactKind_FACT_KIND_WORKSPACE_MEMBER, "root", pkg, rel, line, sourceLine(b, line), "local"))
			}
		}
	}
	return out
}

func lineOfString(b []byte, s string) int {
	for i, line := range bytes.Split(b, []byte("\n")) {
		if bytes.Contains(line, []byte(s)) {
			return i + 1
		}
	}
	return 1
}

func sourceLine(b []byte, n int) string {
	lines := bytes.Split(b, []byte("\n"))
	if n < 1 || n > len(lines) {
		return ""
	}
	return strings.TrimSpace(string(lines[n-1]))
}
