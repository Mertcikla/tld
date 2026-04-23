package analyze_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/cmd"

	"github.com/mertcikla/tld/internal/workspace"
)

const (
	testAnalyzeDependencyLabelImport    = "imports"
	testAnalyzeDependencyLabelReference = "references"
	testAnalyzeDependencyLabelBoth      = "depends_on"
)

func TestAnalyzeCmd_DryRun_NoWrite(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := filepath.Join(dir, "service.go")
	if err := os.WriteFile(file, []byte("package main\nfunc Service() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", file, "--dry-run")
	if err != nil {
		t.Fatalf("analyze --dry-run: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	after, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("elements.yaml changed during dry-run")
	}
	if !strings.Contains(stdout, "[dry-run]   OK  3 elements written to elements.yaml") {
		t.Fatalf("unexpected stdout: %s", stdout)
	}
}

func TestAnalyzeCmd_WritesElements(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := filepath.Join(dir, "service.go")
	if err := os.WriteFile(file, []byte("package main\nfunc Foo() {}\nfunc Bar() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", file)
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Elements) != 4 {
		t.Fatalf("elements = %d, want 4", len(ws.Elements))
	}
	for ref, element := range ws.Elements {
		if element.Symbol == "" {
			continue
		}
		if len(element.Placements) == 0 {
			t.Fatalf("symbol %q (%s) has no placement", element.Name, ref)
		}
		if element.Placements[0].ParentRef == "root" {
			t.Fatalf("symbol %q (%s) was created at root", element.Name, ref)
		}
	}
}

func TestAnalyzeCmd_CreatesFolderHierarchy(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(filepath.Join(repoDir, "internal", "service"), 0750); err != nil {
		t.Fatal(err)
	}
	cmd.InitGitRepo(t, repoDir, filepath.Join("internal", "service", "service.go"), "package service\nfunc Run() {}\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir)
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	var folderRefs []string
	var fileRef string
	for ref, element := range ws.Elements {
		if element.Kind == "folder" {
			folderRefs = append(folderRefs, ref)
		}
		if element.Kind == "file" && element.FilePath == filepath.Join("internal", "service", "service.go") {
			fileRef = ref
		}
	}
	if len(folderRefs) != 2 {
		t.Fatalf("folder elements = %d, want 2: %+v", len(folderRefs), ws.Elements)
	}
	if fileRef == "" {
		t.Fatalf("expected nested file element, got %+v", ws.Elements)
	}
	fileElement := ws.Elements[fileRef]
	if len(fileElement.Placements) == 0 {
		t.Fatalf("file element has no placements: %+v", fileElement)
	}
	parentRef := fileElement.Placements[0].ParentRef
	parent := ws.Elements[parentRef]
	if parent == nil || parent.Kind != "folder" || parent.FilePath != filepath.Join("internal", "service") {
		t.Fatalf("file parent = %q (%+v), want folder internal/service", parentRef, parent)
	}
	grandparent := ws.Elements[parent.Placements[0].ParentRef]
	if grandparent == nil || grandparent.Kind != "folder" || grandparent.FilePath != "internal" {
		t.Fatalf("folder parent = %+v, want folder internal", grandparent)
	}
}

func TestAnalyzeCmd_ReusesExistingElements(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	repoDir := filepath.Join(dir, "backhaul_analysis")
	if err := os.MkdirAll(repoDir, 0750); err != nil {
		t.Fatal(err)
	}
	cmd.InitGitRepo(t, repoDir, "backhaul_analysis.py", "from collections import OrderedDict\n\n\ndef get_columns():\n    return []\n")

	if err := workspace.UpsertElement(dir, "backhaul-analysis", &workspace.Element{
		Name:       "backhaul_analysis",
		Kind:       "repository",
		Branch:     "main",
		HasView:    true,
		ViewLabel:  "backhaul_analysis",
		Placements: []workspace.ViewPlacement{{ParentRef: "root"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspace.UpsertElement(dir, "backhaul-analysis-py", &workspace.Element{
		Name:       "backhaul_analysis.py",
		Kind:       "file",
		Branch:     "main",
		FilePath:   "backhaul_analysis.py",
		HasView:    true,
		ViewLabel:  "backhaul_analysis.py",
		Placements: []workspace.ViewPlacement{{ParentRef: "backhaul-analysis"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspace.UpsertElement(dir, "existing-get-columns", &workspace.Element{
		Name:       "get_columns",
		Kind:       "function",
		Branch:     "main",
		FilePath:   "backhaul_analysis.py",
		Symbol:     "get_columns",
		Placements: []workspace.ViewPlacement{{ParentRef: "backhaul-analysis-py"}},
	}); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir)
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ws.Elements["existing-get-columns"]; !ok {
		t.Fatalf("expected existing symbol ref to be reused, got keys: %v", ws.Elements)
	}
	if len(ws.Elements) != 3 {
		t.Fatalf("elements = %d, want 3", len(ws.Elements))
	}
	element := ws.Elements["existing-get-columns"]
	if len(element.Placements) == 0 || element.Placements[0].ParentRef != "backhaul-analysis-py" {
		t.Fatalf("reused symbol placement = %+v, want parent backhaul-analysis-py", element.Placements)
	}
}

func TestAnalyzeCmd_WritesConnectors(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package main\nfunc Foo() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.go"), []byte("package main\nfunc Bar() { Foo() }\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", dir)
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ws.Connectors) == 0 {
		t.Fatalf("expected at least one connector, stdout=%s stderr=%s", stdout, stderr)
	}
}

func TestAnalyzeCmd_AddsCrossFileAndCrossFolderConnectors(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "go.mod", "module example.com/demo\n\ngo 1.23.0\n")
	if err := os.MkdirAll(filepath.Join(repoDir, "cmd", "app"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoDir, "internal", "service"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "internal", "service", "service.go"), []byte("package service\n\nfunc Run() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "cmd", "app", "main.go"), []byte("package main\n\nimport \"example.com/demo/internal/service\"\n\nfunc main() {\n\tservice.Run()\n}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir)
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	mainFileRef := findAnalyzeElementRefByKindAndPath(t, ws, "file", filepath.Join("cmd", "app", "main.go"))
	serviceFileRef := findAnalyzeElementRefByKindAndPath(t, ws, "file", filepath.Join("internal", "service", "service.go"))
	cmdFolderRef := findAnalyzeElementRefByKindAndPath(t, ws, "folder", filepath.Join("cmd", "app"))
	serviceFolderRef := findAnalyzeElementRefByKindAndPath(t, ws, "folder", filepath.Join("internal", "service"))

	assertAnalyzeConnectorExists(t, ws, mainFileRef, serviceFileRef, testAnalyzeDependencyLabelReference)
	assertAnalyzeConnectorExists(t, ws, mainFileRef, serviceFolderRef, testAnalyzeDependencyLabelImport)
	assertAnalyzeConnectorExists(t, ws, cmdFolderRef, serviceFolderRef, testAnalyzeDependencyLabelBoth)
	assertAnalyzeConnectorCount(t, ws, cmdFolderRef, serviceFolderRef, testAnalyzeDependencyLabelBoth, 1)
	if len(ws.Connectors) < 4 {
		t.Fatalf("expected at least 4 connectors, got %d: %+v", len(ws.Connectors), ws.Connectors)
	}
}

func TestAnalyzeCmd_MergesReverseConnectorsAsBidirectional(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package main\nfunc Foo() { Bar() }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.go"), []byte("package main\nfunc Bar() { Foo() }\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", dir)
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	fooRef := findAnalyzeElementRefBySymbol(t, ws, "Foo")
	barRef := findAnalyzeElementRefBySymbol(t, ws, "Bar")
	assertBidirectionalAnalyzeConnector(t, ws, fooRef, barRef, "calls")
	assertAnalyzeConnectorCountUnordered(t, ws, fooRef, barRef, "calls", 1)
	assertAnalyzeConnectorCountByLabel(t, ws, "calls", 1)
	assertAnalyzeConnectorCountByLabel(t, ws, testAnalyzeDependencyLabelReference, 1)
}

func TestAnalyzeCmd_DeepModeDoesNotDoubleConnectorCounts(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package main\nfunc Foo() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.go"), []byte("package main\nfunc Bar() { Foo() }\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", dir, "--dry-run", "--deep")
	if err != nil {
		t.Fatalf("analyze --deep: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "[dry-run]   OK  2 connectors written to connectors.yaml") {
		t.Fatalf("unexpected connector count in deep mode\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestAnalyzeCmd_WorkspaceRootWithoutConfiguredReposUsesWorkspaceFiles(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if err := os.WriteFile(filepath.Join(dir, "foo.go"), []byte("package main\nfunc Foo() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bar.go"), []byte("package main\nfunc Bar() { Foo() }\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", dir, "--dry-run")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, "[dry-run]   OK  5 elements written to elements.yaml") {
		t.Fatalf("unexpected element count\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stdout, "[dry-run]   OK  2 connectors written to connectors.yaml") {
		t.Fatalf("unexpected connector count\nstdout: %s\nstderr: %s", stdout, stderr)
	}
}

func TestAnalyzeCmd_PythonImports(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	repoDir := filepath.Join(dir, "pyapp")
	if err := os.MkdirAll(filepath.Join(repoDir, "myapp", "utils"), 0750); err != nil {
		t.Fatal(err)
	}
	cmd.InitGitRepo(t, repoDir, filepath.Join("myapp", "utils", "helper.py"), "def run():\n    pass\n")
	if err := os.WriteFile(filepath.Join(repoDir, "myapp", "main.py"), []byte("from .utils import helper\nfrom myapp.utils.helper import run\n\ndef main():\n    run()\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir)
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	mainFileRef := findAnalyzeElementRefByKindAndPath(t, ws, "file", filepath.Join("myapp", "main.py"))
	helperFileRef := findAnalyzeElementRefByKindAndPath(t, ws, "file", filepath.Join("myapp", "utils", "helper.py"))
	utilsFolderRef := findAnalyzeElementRefByKindAndPath(t, ws, "folder", filepath.Join("myapp", "utils"))

	assertAnalyzeConnectorExists(t, ws, mainFileRef, utilsFolderRef, testAnalyzeDependencyLabelImport)
	assertAnalyzeConnectorExists(t, ws, mainFileRef, helperFileRef, testAnalyzeDependencyLabelReference)
}

func TestAnalyzeCmd_ExcludeRules(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	config := "project_name: Demo\nexclude:\n  - '*_test.go'\n"
	if err := os.WriteFile(filepath.Join(dir, ".tld.yaml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prod.go"), []byte("package main\nfunc Prod() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "prod_test.go"), []byte("package main\nfunc TestOnly() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := cmd.RunCmd(t, dir, "analyze", dir); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, element := range ws.Elements {
		if element.Name == "TestOnly" {
			t.Fatalf("unexpected test symbol in elements.yaml: %+v", ws.Elements)
		}
	}
}

func TestAnalyzeCmd_GeneratedNamesAreGloballyUnique(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	repoDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(filepath.Join(repoDir, "cmd"), 0750); err != nil {
		t.Fatal(err)
	}
	cmd.InitGitRepo(t, repoDir, filepath.Join("cmd", "main.go"), "package main\nfunc main() {}\n")
	if err := os.MkdirAll(filepath.Join(repoDir, "tools"), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, "tools", "main.go"), []byte("package main\nfunc main() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir)
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, validationErr := range ws.ValidateWithOpts(workspace.ValidationOptions{SkipSymbols: true}) {
		if strings.Contains(validationErr.Message, "duplicate element name") {
			t.Fatalf("unexpected duplicate-name validation error: %v", validationErr)
		}
	}
}

func TestAnalyzeCmd_PathNotExist(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	_, _, err := cmd.RunCmd(t, dir, "analyze", filepath.Join(dir, "missing.go"))
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestAnalyzeCmd_SetsTechnology(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := filepath.Join(dir, "service.go")
	if err := os.WriteFile(file, []byte("package main\nfunc Foo() {}\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := cmd.RunCmd(t, dir, "analyze", file); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	fileRef := findAnalyzeElementRefByKindAndPath(t, ws, "file", "service.go")
	fileElement := ws.Elements[fileRef]
	if fileElement.Technology != "go" {
		t.Fatalf("file technology = %q, want go", fileElement.Technology)
	}

	fooRef := findAnalyzeElementRefBySymbol(t, ws, "Foo")
	fooElement := ws.Elements[fooRef]
	if fooElement.Technology != "go" {
		t.Fatalf("symbol technology = %q, want go", fooElement.Technology)
	}
}

func TestAnalyzeCmd_SetsTechnologyPython(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	file := filepath.Join(dir, "app.py")
	if err := os.WriteFile(file, []byte("def hello():\n    pass\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := cmd.RunCmd(t, dir, "analyze", file); err != nil {
		t.Fatalf("analyze: %v", err)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	fileRef := findAnalyzeElementRefByKindAndPath(t, ws, "file", "app.py")
	fileElement := ws.Elements[fileRef]
	if fileElement.Technology != "python" {
		t.Fatalf("file technology = %q, want python", fileElement.Technology)
	}

	helloRef := findAnalyzeElementRefBySymbol(t, ws, "hello")
	helloElement := ws.Elements[helloRef]
	if helloElement.Technology != "python" {
		t.Fatalf("symbol technology = %q, want python", helloElement.Technology)
	}
}

func findAnalyzeElementRefByKindAndPath(t *testing.T, ws *workspace.Workspace, kind, filePath string) string {
	t.Helper()
	for ref, element := range ws.Elements {
		if element.Kind == kind && element.FilePath == filePath {
			return ref
		}
	}
	t.Fatalf("expected %s element for %s, got %+v", kind, filePath, ws.Elements)
	return ""
}

func findAnalyzeElementRefBySymbol(t *testing.T, ws *workspace.Workspace, symbol string) string {
	t.Helper()
	for ref, element := range ws.Elements {
		if element.Symbol == symbol {
			return ref
		}
	}
	t.Fatalf("expected symbol element for %s, got %+v", symbol, ws.Elements)
	return ""
}

func assertAnalyzeConnectorExists(t *testing.T, ws *workspace.Workspace, source, target, label string) {
	t.Helper()
	for _, connector := range ws.Connectors {
		if connector.Source == source && connector.Target == target && connector.Label == label {
			return
		}
	}
	t.Fatalf("expected connector %s -> %s (%s), got %+v", source, target, label, ws.Connectors)
}

func assertAnalyzeConnectorCount(t *testing.T, ws *workspace.Workspace, source, target, label string, want int) {
	t.Helper()
	got := 0
	for _, connector := range ws.Connectors {
		if connector.Source == source && connector.Target == target && connector.Label == label {
			got++
		}
	}
	if got != want {
		t.Fatalf("connector count %s -> %s (%s) = %d, want %d: %+v", source, target, label, got, want, ws.Connectors)
	}
}

func assertAnalyzeConnectorCountUnordered(t *testing.T, ws *workspace.Workspace, left, right, label string, want int) {
	t.Helper()
	got := 0
	for _, connector := range ws.Connectors {
		if connector.Label != label {
			continue
		}
		if (connector.Source == left && connector.Target == right) || (connector.Source == right && connector.Target == left) {
			got++
		}
	}
	if got != want {
		t.Fatalf("unordered connector count %s <-> %s (%s) = %d, want %d: %+v", left, right, label, got, want, ws.Connectors)
	}
}

func assertAnalyzeConnectorCountByLabel(t *testing.T, ws *workspace.Workspace, label string, want int) {
	t.Helper()
	got := 0
	for _, connector := range ws.Connectors {
		if connector.Label == label {
			got++
		}
	}
	if got != want {
		t.Fatalf("connector count for label %s = %d, want %d: %+v", label, got, want, ws.Connectors)
	}
}

func assertBidirectionalAnalyzeConnector(t *testing.T, ws *workspace.Workspace, left, right, label string) {
	t.Helper()
	for _, connector := range ws.Connectors {
		if connector.Label != label {
			continue
		}
		if (connector.Source == left && connector.Target == right) || (connector.Source == right && connector.Target == left) {
			if connector.Direction != "both" {
				t.Fatalf("connector %s <-> %s (%s) direction = %s, want both: %+v", left, right, label, connector.Direction, connector)
			}
			return
		}
	}
	t.Fatalf("expected bidirectional connector %s <-> %s (%s), got %+v", left, right, label, ws.Connectors)
}
