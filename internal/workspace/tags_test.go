package workspace_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestParseTagListNormalizesAndDedupes(t *testing.T) {
	got := workspace.ParseTagList(" edge, ingress ,edge\ncore\tops ")
	want := []string{"edge", "ingress", "core", "ops"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseTagList = %v, want %v", got, want)
	}
	if got := workspace.ParseTagList("   "); len(got) != 0 {
		t.Fatalf("ParseTagList blank = %v, want empty", got)
	}
}

func TestUnionAndSubtractTags(t *testing.T) {
	base := []string{"a", "b"}
	if got := workspace.UnionTags(base, []string{"b", "c"}); !reflect.DeepEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("UnionTags = %v", got)
	}
	if got := workspace.SubtractTags(base, []string{"b"}); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("SubtractTags = %v", got)
	}
	if got := workspace.SubtractTags(base, []string{"a", "b"}); len(got) != 0 {
		t.Fatalf("SubtractTags all = %v, want empty", got)
	}
}

func TestUpdateElementTagsPreservesCommentsAndClears(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "elements.yaml")
	content := `# top comment
platform:
  name: Platform
  kind: workspace
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	if err := workspace.UpdateElementField(dir, "platform", "tags", "edge, ingress"); err != nil {
		t.Fatalf("update tags: %v", err)
	}
	el, err := loadElement(dir, "platform")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(el.Tags, []string{"edge", "ingress"}) {
		t.Fatalf("tags = %v, want [edge ingress]", el.Tags)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "# top comment") {
		t.Fatalf("comment was lost:\n%s", data)
	}

	if err := workspace.UpdateElementField(dir, "platform", "tags", ""); err != nil {
		t.Fatalf("clear tags: %v", err)
	}
	el, err = loadElement(dir, "platform")
	if err != nil {
		t.Fatal(err)
	}
	if len(el.Tags) != 0 {
		t.Fatalf("tags not cleared: %v", el.Tags)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "tags:") {
		t.Fatalf("tags field was not removed:\n%s", data)
	}
}

func TestUpdateConnectorFieldTagsReplacesSequence(t *testing.T) {
	dir := t.TempDir()
	writeConnectorWorkspace(t, dir, "")

	if err := workspace.UpdateConnectorField(dir, "system/web~api/calls", "tags", "critical, api"); err != nil {
		t.Fatalf("update connector tags: %v", err)
	}
	connector, err := loadConnector(dir, "system/web~api/calls")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(connector.Tags, []string{"critical", "api"}) {
		t.Fatalf("tags = %v, want [critical api]", connector.Tags)
	}

	if err := workspace.UpdateConnectorField(dir, "system/web~api/calls", "tags", ""); err != nil {
		t.Fatalf("clear connector tags: %v", err)
	}
	connector, err = loadConnector(dir, "system/web~api/calls")
	if err != nil {
		t.Fatal(err)
	}
	if len(connector.Tags) != 0 {
		t.Fatalf("connector tags not cleared: %v", connector.Tags)
	}
}

func TestUpdateConnectorFieldAppliesVisibilityDelta(t *testing.T) {
	dir := t.TempDir()
	writeConnectorWorkspace(t, dir, "")

	if err := workspace.UpdateConnectorField(dir, "system/web~api/calls", "visibility_delta", "2"); err != nil {
		t.Fatalf("update visibility_delta: %v", err)
	}
	connector, err := loadConnector(dir, "system/web~api/calls")
	if err != nil {
		t.Fatal(err)
	}
	if connector.VisibilityDelta != 2 {
		t.Fatalf("visibility_delta = %d, want 2", connector.VisibilityDelta)
	}
}

func TestApplyConnectorFieldVisibilityDelta(t *testing.T) {
	spec := &workspace.Connector{}
	workspace.ApplyConnectorField(spec, "visibility_delta", "-1")
	if spec.VisibilityDelta != -1 {
		t.Fatalf("visibility_delta = %d, want -1", spec.VisibilityDelta)
	}
	workspace.ApplyConnectorField(spec, "visibility_delta", "not-a-number")
	if spec.VisibilityDelta != -1 {
		t.Fatalf("invalid value should be ignored, got %d", spec.VisibilityDelta)
	}
}

func loadElement(dir, ref string) (*workspace.Element, error) {
	ws, err := workspace.Load(dir)
	if err != nil {
		return nil, err
	}
	return ws.Elements[ref], nil
}

func loadConnector(dir, ref string) (*workspace.Connector, error) {
	ws, err := workspace.Load(dir)
	if err != nil {
		return nil, err
	}
	return ws.Connectors[ref], nil
}

func writeConnectorWorkspace(t *testing.T, dir, tags string) {
	t.Helper()
	body := "system/web~api/calls:\n  view: system\n  source: web\n  target: api\n  label: calls\n"
	if tags != "" {
		body += "  tags:\n"
		for _, tag := range strings.Split(tags, ",") {
			body += "    - " + strings.TrimSpace(tag) + "\n"
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "connectors.yaml"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
