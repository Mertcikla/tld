package analyze_test

import (
	"encoding/json"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestAnalyzeCmd_WatchPipelineWritesYAML(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "service.go", "package main\nfunc Foo() {}\nfunc Bar() {}\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if countKind(ws, "repository") != 1 || countKind(ws, "file") != 1 || countKind(ws, "function") != 2 {
		t.Fatalf("unexpected analyzed elements: %+v", ws.Elements)
	}
	for ref, element := range ws.Elements {
		if element.Kind == "function" && len(element.Placements) == 0 {
			t.Fatalf("symbol %q (%s) has no placement", element.Name, ref)
		}
	}

	stdout, stderr, err = cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("second analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, element := range ws.Elements {
		if element.Kind == "file" && strings.HasPrefix(element.FilePath, ".tld/") {
			t.Fatalf("generated workspace YAML should not be scanned as source: %+v", element)
		}
	}
}

func TestAnalyzeCmd_QualifiesCollidingGeneratedSymbolNames(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "alpha/handler.go", "package alpha\nfunc Shared() {}\n")
	writeAnalyzeTestFile(t, repoDir, "beta/handler.go", "package beta\nfunc Shared() {}\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if errs := ws.ValidateWithOpts(workspace.ValidationOptions{SkipSymbols: true}); len(errs) > 0 {
		t.Fatalf("analyze generated invalid workspace: %v", errs)
	}
	if countElementName(ws, "alpha.handler.Shared") != 1 || countElementName(ws, "beta.handler.Shared") != 1 {
		t.Fatalf("expected colliding generated symbol names to include file context: %+v", ws.Elements)
	}
	if countElementName(ws, "Shared") != 0 {
		t.Fatalf("expected unqualified duplicate generated symbol name to be removed: %+v", ws.Elements)
	}
}

func TestAnalyzeCmd_SkipsDependencyImportsByDefault(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "alpha.go", "package app\n\nimport \"fmt\"\n\nfunc Alpha() { fmt.Println(\"alpha\") }\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if countElementName(ws, "fmt") != 0 || countElementName(ws, "Go standard library") != 0 || countKind(ws, "dependency-group") != 0 {
		t.Fatalf("expected dependency imports to be disabled by default, got %+v", ws.Elements)
	}
}

func TestAnalyzeCmd_GroupsDependencyImportsWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "config", "set", "watch.dependencies.enabled", "true")
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "alpha.go", "package app\n\nimport \"fmt\"\n\nfunc Alpha() { fmt.Println(\"alpha\") }\n")
	writeAnalyzeTestFile(t, repoDir, "beta.go", "package app\n\nimport \"fmt\"\n\nfunc Beta() { fmt.Println(\"beta\") }\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if countElementName(ws, "fmt") != 0 {
		t.Fatalf("expected exact fmt dependency module to stay hidden by default, got %+v", ws.Elements)
	}
	if countElementName(ws, "Go standard library") != 1 {
		t.Fatalf("expected one grouped Go stdlib dependency element, got %+v", ws.Elements)
	}
	if countKind(ws, "dependency-group") != 1 {
		t.Fatalf("expected dependency imports to materialize as one dependency group, got %+v", ws.Elements)
	}
	if countConnectorsToName(ws, "Go standard library", "1 import") != 2 {
		t.Fatalf("expected both files to connect to grouped fmt imports, got %+v", ws.Connectors)
	}
}

func TestAnalyzeCmd_RemovedWeakORMEnrichersDoNotMaterializeMetadata(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "model.py", "import sqlalchemy\n\ndef load():\n    return sqlalchemy.text('select 1')\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if countElementName(ws, "Python SQLAlchemy") != 0 {
		t.Fatalf("expected removed SQLAlchemy enricher not to materialize a standalone element: %+v", ws.Elements)
	}
	if anyElementTechnologyContains(ws, "sqlalchemy") {
		t.Fatalf("expected removed SQLAlchemy enricher not to attach element technology: %+v", ws.Elements)
	}
}

func TestAnalyzeCmd_MergesDuplicateComposeRuntimeComponents(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "docker-compose.yaml", "services:\n  redis:\n    image: redis:7\n")
	writeAnalyzeTestFile(t, repoDir, "docker-compose.prod.yml", "services:\n  redis:\n    image: redis:7\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if countElementName(ws, "redis") != 1 {
		t.Fatalf("expected duplicate runtime components to merge, got %+v", ws.Elements)
	}
}

func TestAnalyzeCmd_RuntimeArtifactsUseArchitectureView(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "runtime-app")
	cmd.InitGitRepo(t, repoDir, "generated/client.go", "package generated\n\n// Code generated by protoc. DO NOT EDIT.\nfunc NoisyStub() {}\n")
	writeAnalyzeTestFile(t, repoDir, "deploy/topology.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: alpha
spec:
  template:
    spec:
      containers:
      - name: app
        image: example/alpha:latest
        ports:
        - containerPort: 8080
        env:
        - name: PEER
          value: "beta:9090"
        - name: CACHE
          value: "cache:6379"
---
apiVersion: v1
kind: Service
metadata:
  name: alpha
spec:
  type: LoadBalancer
  ports:
  - port: 80
    targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: beta
spec:
  template:
    spec:
      containers:
      - name: app
        image: example/beta:latest
        ports:
        - containerPort: 9090
---
apiVersion: v1
kind: Service
metadata:
  name: beta
spec:
  ports:
  - port: 9090
---
apiVersion: v1
kind: Service
metadata:
  name: cache
spec:
  ports:
  - port: 6379
`)

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	architectureRef := refByElementName(ws, "Architecture")
	structuralRef := refByElementName(ws, "Structural")
	repositoryRef := refByKind(ws, "repository")
	if architectureRef == "" || structuralRef == "" || repositoryRef == "" {
		t.Fatalf("missing repository sections: %+v", ws.Elements)
	}
	if !hasPlacementParent(ws, architectureRef, structuralRef) || !hasPlacementParent(ws, structuralRef, repositoryRef) {
		t.Fatalf("architecture section should be nested under structural, and structural under repository: %+v", ws.Elements)
	}
	for _, name := range []string{"alpha", "beta", "cache", "External traffic"} {
		ref := refByElementNameWithParent(ws, name, architectureRef)
		if ref == "" {
			t.Fatalf("missing architecture element %q in %+v", name, ws.Elements)
		}
	}
	deployRef := refByElementName(ws, "deploy")
	if deployRef == "" || !hasPlacementParent(ws, deployRef, structuralRef) {
		t.Fatalf("top-level structural folder should be under Structural, ref=%q elements=%+v", deployRef, ws.Elements)
	}
	if ref := refByElementName(ws, "topology.yaml"); ref == "" || !hasPlacementParent(ws, ref, deployRef) {
		t.Fatalf("structural file should be under its folder view, ref=%q elements=%+v", ref, ws.Elements)
	}

	if !connectorByElementNamesInParent(ws, "alpha", "beta", architectureRef) || !connectorByElementNamesInParent(ws, "alpha", "cache", architectureRef) || !connectorByElementNamesInParent(ws, "External traffic", "alpha", architectureRef) {
		t.Fatalf("missing expected architecture connectors: %+v", ws.Connectors)
	}
}

func TestAnalyzeCmd_ComposeInferenceDoesNotDependOnFolderNames(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "odd-layout")
	cmd.InitGitRepo(t, repoDir, "main.go", "package main\nfunc Main() {}\n")
	writeAnalyzeTestFile(t, repoDir, "ops/runtime.yml", `
services:
  worker:
    image: example/worker
    environment:
      TARGET_URL: "http://api:8080"
  api:
    image: example/api
    ports:
    - "8080:8080"
  datastore:
    image: redis:7
`)

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	architectureRef := refByElementName(ws, "Architecture")
	structuralRef := refByElementName(ws, "Structural")
	if architectureRef == "" || structuralRef == "" {
		t.Fatalf("missing repository sections: %+v", ws.Elements)
	}
	if ref := refByElementName(ws, "Main"); ref == "" {
		t.Fatalf("structural symbol should still be materialized: %+v", ws.Elements)
	} else if !hasPlacementParent(ws, ref, refByElementName(ws, "main.go")) {
		t.Fatalf("symbol should remain nested under its file view: %+v", ws.Elements[ref].Placements)
	}
	if ref := refByElementName(ws, "main.go"); ref == "" || !hasPlacementParent(ws, ref, structuralRef) {
		t.Fatalf("top-level structural file should be under Structural, ref=%q elements=%+v", ref, ws.Elements)
	}
}

func TestAnalyzeCmd_CrossRepositoryArchitectureLinksReuseStructuralElements(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	sourceRepo := filepath.Join(dir, "source-repo")
	cmd.InitGitRepo(t, sourceRepo, "modules/cart/main.go", "package main\nfunc ServeCart() {}\n")
	runtimeRepo := filepath.Join(dir, "runtime-repo")
	cmd.InitGitRepo(t, runtimeRepo, "deploy/cart.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cart
spec:
  template:
    spec:
      containers:
      - name: app
        image: example/cart
        env:
        - name: PEER
          value: "cache:6379"
---
apiVersion: v1
kind: Service
metadata:
  name: cache
spec:
  ports:
  - port: 6379
`)

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", sourceRepo, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze source: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	stdout, stderr, err = cmd.RunCmd(t, dir, "analyze", runtimeRepo, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze runtime: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	runtimeRef := refByElementName(ws, "runtime-repo")
	structuralRef := refByElementNameWithParent(ws, "Structural", runtimeRef)
	architectureRef := refByElementNameWithParent(ws, "Architecture", structuralRef)
	cartArchRef := refByElementNameWithParent(ws, "cart", architectureRef)
	cartFolderRef := refByKindAndFilePath(ws, "folder", "modules/cart")
	if runtimeRef == "" || structuralRef == "" || architectureRef == "" || cartArchRef == "" || cartFolderRef == "" {
		t.Fatalf("missing cross-repo test elements: runtime=%q structural=%q architecture=%q cartArch=%q cartFolder=%q elements=%+v", runtimeRef, structuralRef, architectureRef, cartArchRef, cartFolderRef, ws.Elements)
	}
	if placementCount(ws, cartFolderRef) < 1 {
		t.Fatalf("source structural folder should remain in its original structural view: %+v", ws.Elements[cartFolderRef])
	}
	if !hasPlacementParent(ws, cartFolderRef, "folder-folder-modules") {
		t.Fatalf("source structural folder should be placed under modules folder, got: %+v", ws.Elements[cartFolderRef].Placements)
	}
}

func TestAnalyzeCmd_PrunesDisconnectedArchitectureComponents(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "runtime-app")
	cmd.InitGitRepo(t, repoDir, "main.go", "package main\nfunc Main() {}\n")
	writeAnalyzeTestFile(t, repoDir, "deploy/topology.yaml", `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: connected
spec:
  template:
    spec:
      containers:
      - name: app
        image: example/connected:latest
        env:
        - name: PEER
          value: "target:9090"
---
apiVersion: v1
kind: Service
metadata:
  name: target
spec:
  ports:
  - port: 9090
---
apiVersion: v1
kind: Service
metadata:
  name: isolated
spec:
  ports:
  - port: 9999
`)

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	architectureRef := refByElementName(ws, "Architecture")
	for ref, element := range ws.Elements {
		if element.Name == "isolated" && hasPlacementParent(ws, ref, architectureRef) {
			t.Fatalf("disconnected architecture element should be pruned from Architecture: %+v", ws.Elements)
		}
	}
	if !connectorByElementNamesInParent(ws, "connected", "target", architectureRef) {
		t.Fatalf("expected connected architecture edge, got %+v", ws.Connectors)
	}
}

func TestAnalyzeCmd_PreservesManualYAML(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Manual API", "--ref", "manual-api", "--kind", "service")
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "main.go", "package main\nfunc Main() {}\n")

	if stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none"); err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ws.Elements["manual-api"] == nil {
		t.Fatalf("manual element was not preserved: %+v", ws.Elements)
	}
}

func TestAnalyzeCmd_DryRunDoesNotWriteYAML(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "service.go", "package main\nfunc Service() {}\n")
	before, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--dry-run", "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze --dry-run: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	after, err := os.ReadFile(filepath.Join(dir, "elements.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("elements.yaml changed during dry-run")
	}
}

func TestAnalyzeCmd_WarnsWhenLimitedScanModeIsActive(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	t.Setenv("TLD_WATCH_SCALE_STRATEGY", "limited")
	t.Setenv("TLD_WATCH_SCALE_MAX_TRACKED_FILES", "1")
	t.Setenv("TLD_WATCH_SCALE_MAX_LIMITED_FILES", "10")
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "build.gradle", "plugins { id 'java' }\n")
	writeAnalyzeTestFile(t, repoDir, "src/main/java/example/App.java", "package example;\nclass App { void run() {} }\n")
	commitAnalyzeTestFiles(t, repoDir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	for _, want := range []string{
		"Limited scan mode active: limited scan requested.",
		"Scanned recent files plus bounded reference/caller context; source symbols and connectors may still be omitted.",
		"Limited expansion: recent=",
		"Use `tld config set watch.scale.strategy full` or raise `watch.scale.max_tracked_files` for a full scan.",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("limited scan warning missing %q:\n%s", want, stdout)
		}
	}
}

func TestAnalyzeCmd_WritesPipelineLogWithoutBanner(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "service.go", "package main\nfunc Service() {}\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	for _, want := range []string{"tld analyze", "Workspace\n", "Data directory", "Runtime\n", "Results\n", "Duration"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("text stdout should include compact analyze output %q, got:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "Pipeline\n") {
		t.Fatalf("text stdout should hide pipeline history without verbose, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "░███████") || strings.Contains(stdout, "Version:") || strings.Contains(stdout, "✓") {
		t.Fatalf("text stdout should use compact analyze output without logo/check glyphs, got:\n%s", stdout)
	}
	logData, err := os.ReadFile(localserver.LogPath(dataDir))
	if err != nil {
		t.Fatalf("read analyze log: %v", err)
	}
	logText := string(logData)
	for _, want := range []string{
		"msg=analyze.started",
		"msg=watch.scan.started",
		"msg=watch.scan.file",
		"path=service.go",
		"msg=watch.representation.completed",
		"msg=analyze.export.completed",
		"msg=analyze.workspace_save.completed",
		"msg=analyze.completed",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("log missing %q:\n%s", want, logText)
		}
	}
	if strings.Contains(logText, "░███████") || strings.Contains(logText, "Version:") {
		t.Fatalf("log should not contain startup banner/logo:\n%s", logText)
	}
}

func TestAnalyzeCmd_RemovedFlagsFail(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	if _, _, err := cmd.RunCmd(t, dir, "analyze", dir, "--deep"); err == nil {
		t.Fatal("expected --deep to fail")
	}
	if _, _, err := cmd.RunCmd(t, dir, "analyze", dir, "--changed-since", "HEAD"); err == nil {
		t.Fatal("expected --changed-since to fail")
	}
}

func TestAnalyzeCmd_JSONDryRunUsesWatchDiffShape(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "main.go", "package main\nfunc Main() {}\n")

	stdout, stderr, err := cmd.RunCmd(t, dir, "--format", "json", "analyze", repoDir, "--dry-run", "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze --format json --dry-run: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var payload struct {
		Changed        bool             `json:"changed"`
		Scan           map[string]any   `json:"scan"`
		LSP            map[string]any   `json:"lsp"`
		Representation map[string]any   `json:"representation"`
		Export         map[string]any   `json:"export"`
		Diffs          []map[string]any `json:"diffs"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout)
	}
	if payload.Scan["repository_id"] == nil || payload.Representation["representation_hash"] == nil || payload.Export["elements_written"] == nil {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.LSP["summary"] == nil {
		t.Fatalf("expected top-level lsp summary in payload: %+v", payload)
	}
	logData, err := os.ReadFile(localserver.LogPath(dataDir))
	if err != nil {
		t.Fatalf("read analyze log: %v", err)
	}
	if !strings.Contains(string(logData), "msg=analyze.completed") {
		t.Fatalf("json-mode analyze should still write logs:\n%s", string(logData))
	}
	if strings.Contains(stdout, "msg=analyze.") {
		t.Fatalf("json stdout should not contain log lines:\n%s", stdout)
	}
}

func TestAnalyzeCmd_LogsWatchPipelineError(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", dir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err == nil {
		t.Fatalf("expected analyze to fail for non-git path\nstdout: %s\nstderr: %s", stdout, stderr)
	}
	logData, readErr := os.ReadFile(localserver.LogPath(dataDir))
	if readErr != nil {
		t.Fatalf("read analyze log: %v", readErr)
	}
	logText := string(logData)
	if !strings.Contains(logText, "msg=watch.prepare.failed") || !strings.Contains(logText, "msg=analyze.failed") {
		t.Fatalf("log should contain failed phase and final failure:\n%s", logText)
	}
}

func countKind(ws *workspace.Workspace, kind string) int {
	count := 0
	for _, element := range ws.Elements {
		if element.Kind == kind {
			count++
		}
	}
	return count
}

func writeAnalyzeTestFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func commitAnalyzeTestFiles(t *testing.T, repoDir string) {
	t.Helper()
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "update"}} {
		command := osexec.Command("git", args...)
		command.Dir = repoDir
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
}

func refByElementName(ws *workspace.Workspace, name string) string {
	for ref, element := range ws.Elements {
		if element.Name == name {
			return ref
		}
	}
	return ""
}

func refByElementNameWithParent(ws *workspace.Workspace, name, parentRef string) string {
	for ref, element := range ws.Elements {
		if element.Name == name && hasPlacementParent(ws, ref, parentRef) {
			return ref
		}
	}
	return ""
}

func refByKind(ws *workspace.Workspace, kind string) string {
	for ref, element := range ws.Elements {
		if element.Kind == kind {
			return ref
		}
	}
	return ""
}

func refByKindAndFilePath(ws *workspace.Workspace, kind, filePath string) string {
	for ref, element := range ws.Elements {
		if element.Kind == kind && element.FilePath == filePath {
			return ref
		}
	}
	return ""
}

func countElementName(ws *workspace.Workspace, name string) int {
	count := 0
	for _, element := range ws.Elements {
		if element.Name == name {
			count++
		}
	}
	return count
}

func countConnectorsToName(ws *workspace.Workspace, targetName, label string) int {
	targetRef := refByElementName(ws, targetName)
	count := 0
	for _, connector := range ws.Connectors {
		if connector.Target == targetRef && connector.Label == label {
			count++
		}
	}
	return count
}

func anyElementTechnologyContains(ws *workspace.Workspace, value string) bool {
	value = strings.ToLower(value)
	for _, element := range ws.Elements {
		if strings.Contains(strings.ToLower(element.Technology), value) {
			return true
		}
	}
	return false
}

func hasPlacementParent(ws *workspace.Workspace, ref, parentRef string) bool {
	element := ws.Elements[ref]
	if element == nil {
		return false
	}
	for _, placement := range element.Placements {
		if placement.ParentRef == parentRef {
			return true
		}
	}
	return false
}

func placementCount(ws *workspace.Workspace, ref string) int {
	element := ws.Elements[ref]
	if element == nil {
		return 0
	}
	return len(element.Placements)
}

func connectorByElementNamesInParent(ws *workspace.Workspace, sourceName, targetName, parentRef string) bool {
	sourceRef := refByElementNameWithParent(ws, sourceName, parentRef)
	targetRef := refByElementNameWithParent(ws, targetName, parentRef)
	if sourceRef == "" || targetRef == "" {
		return false
	}
	for _, connector := range ws.Connectors {
		if connector.Source == sourceRef && connector.Target == targetRef {
			return true
		}
	}
	return false
}

func TestAnalyzeCmd_RespectsWorkspaceConfigIgnoreList(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	// Write .tld.yaml config file
	configContent := `exclude:
  - "global_ignored.go"
repositories:
  app:
    localDir: app
    exclude:
      - "repo_ignored.go"
`
	if err := os.WriteFile(filepath.Join(dir, ".tld.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "service.go", "package main\nfunc Foo() {}\n")
	writeAnalyzeTestFile(t, repoDir, "global_ignored.go", "package main\nfunc GlobalIgnored() {}\n")
	writeAnalyzeTestFile(t, repoDir, "repo_ignored.go", "package main\nfunc RepoIgnored() {}\n")

	// Commit files so git knows about them
	commitAnalyzeTestFiles(t, repoDir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if countElementName(ws, "Foo") == 0 {
		t.Fatal("expected Foo function to be analyzed and present in workspace elements")
	}
	if countElementName(ws, "GlobalIgnored") != 0 {
		t.Fatal("expected GlobalIgnored function to be ignored, but it was found in workspace elements")
	}
	if countElementName(ws, "RepoIgnored") != 0 {
		t.Fatal("expected RepoIgnored function to be ignored, but it was found in workspace elements")
	}
}

func TestAnalyzeCmd_PreservesManualEditsOnGeneratedElements(t *testing.T) {
	dir := t.TempDir()
	dataDir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	repoDir := filepath.Join(dir, "app")
	cmd.InitGitRepo(t, repoDir, "service.go", "package main\nfunc Foo() {}\n")

	// 1. First analyze to generate elements
	stdout, stderr, err := cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("first analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Find the element ref for the generated function Foo
	var fooRef string
	for ref, element := range ws.Elements {
		if element.Name == "Foo" {
			fooRef = ref
			break
		}
	}
	if fooRef == "" {
		t.Fatalf("expected Foo to be generated, got elements: %+v", ws.Elements)
	}

	// 2. Manually edit the generated element in elements.yaml
	ws.Elements[fooRef].Description = "This is my manual description"
	ws.Elements[fooRef].Technology = "MyTech"
	if err := workspace.Save(ws); err != nil {
		t.Fatalf("save manual edit: %v", err)
	}

	// 3. Re-run tld analyze
	stdout, stderr, err = cmd.RunCmd(t, dir, "analyze", repoDir, "--data-dir", dataDir, "--embedding-provider", "none")
	if err != nil {
		t.Fatalf("second analyze: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// 4. Verify that the edits were not overwritten
	ws, err = workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	fooElement := ws.Elements[fooRef]
	if fooElement == nil {
		t.Fatalf("fooElement not found after second analyze: %+v", ws.Elements)
	}
	if fooElement.Description != "This is my manual description" {
		t.Fatalf("expected description 'This is my manual description', got %q", fooElement.Description)
	}
	if fooElement.Technology != "MyTech" {
		t.Fatalf("expected technology 'MyTech', got %q", fooElement.Technology)
	}
}
