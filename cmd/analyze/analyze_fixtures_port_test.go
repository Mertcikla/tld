package analyze_test

import (
	"github.com/mertcikla/diag/tld/cmd"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func analyzeFixtureWorkspace(t *testing.T, fixture string) string {
	t.Helper()

	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", cmd.ReferenceFixturePath(t, fixture))
	if err != nil {
		t.Fatalf("analyze fixture %s: %v\nstdout: %s\nstderr: %s", fixture, err, stdout, stderr)
	}
	return dir
}

func TestAnalyzeCmd_ReferenceFixturesProduceValidWorkspaces(t *testing.T) {
	fixtures := []string{"go", "python", "typescript", "java-project", "cpp"}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			dir := analyzeFixtureWorkspace(t, fixture)

			elements, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
			if err != nil {
				t.Fatalf("read elements.yaml: %v", err)
			}
			connectors, err := os.ReadFile(filepath.Join(dir, "connectors.yaml"))
			if err != nil {
				t.Fatalf("read connectors.yaml: %v", err)
			}
			if strings.TrimSpace(string(elements)) == "" || strings.TrimSpace(string(elements)) == "{}" {
				t.Fatalf("elements.yaml should not be empty for %s:\n%s", fixture, elements)
			}
			if strings.TrimSpace(string(connectors)) == "" || strings.TrimSpace(string(connectors)) == "[]" {
				t.Fatalf("connectors.yaml should not be empty for %s:\n%s", fixture, connectors)
			}
			if !strings.Contains(string(elements), "has_view: true") {
				t.Fatalf("elements.yaml should include a view-owning element for %s:\n%s", fixture, elements)
			}

			stdout, stderr, err := cmd.RunCmd(t, dir, "validate")
			if err != nil {
				t.Fatalf("validate after analyze for %s: %v\nstdout: %s\nstderr: %s", fixture, err, stdout, stderr)
			}
		})
	}
}

func TestAnalyzeCmd_TypescriptFixtureIncludesCoreServiceSymbols(t *testing.T) {
	dir := analyzeFixtureWorkspace(t, "typescript")

	elements, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatalf("read elements.yaml: %v", err)
	}
	content := string(elements)
	for _, want := range []string{"OrderService", "placeOrder", "PaymentService", "ProductService"} {
		if !strings.Contains(content, "name: "+want) {
			t.Fatalf("typescript analyze output missing %s:\n%s", want, content)
		}
	}
}

func TestAnalyzeCmd_PythonFixturePreservesClassKinds(t *testing.T) {
	dir := analyzeFixtureWorkspace(t, "python")

	elements, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatalf("read elements.yaml: %v", err)
	}
	content := string(elements)
	if !strings.Contains(content, "kind: class") {
		t.Fatalf("python analyze output should preserve class declarations:\n%s", content)
	}
}

func TestAnalyzeCmd_NameCollisionFixtureAvoidsDuplicateElementNames(t *testing.T) {
	dir := analyzeFixtureWorkspace(t, "python-name-collision")

	elements, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatalf("read elements.yaml: %v", err)
	}

	seen := map[string]struct{}{}
	for _, line := range strings.Split(string(elements), "\n") {
		trimmed := strings.TrimSpace(line)
		name, ok := strings.CutPrefix(trimmed, "name: ")
		if !ok {
			continue
		}
		name = strings.Trim(name, "\"")
		if _, exists := seen[name]; exists {
			t.Fatalf("duplicate element name %q found in generated elements.yaml:\n%s", name, elements)
		}
		seen[name] = struct{}{}
	}
}
