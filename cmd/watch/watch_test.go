package watch

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mertcikla/tld/internal/workspace"
)

func TestScanCommandFailsClearlyOutsideGitRepository(t *testing.T) {
	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"scan", t.TempDir(), "--data-dir", t.TempDir()})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
		t.Fatalf("expected outside-git error, got %v", err)
	}
}

func TestScanCommandPrintsCountsAndSkipsRepeatScan(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func main() {
	helper()
}

func helper() {}
`)
	dataDir := t.TempDir()

	first := runScanCommand(t, repo, dataDir)
	if !strings.Contains(first, "Files:") ||
		!strings.Contains(first, "1 seen, 1 parsed, 0 skipped") ||
		!strings.Contains(first, "Symbols:") ||
		!strings.Contains(first, "2") ||
		!strings.Contains(first, "References:") ||
		!strings.Contains(first, "1") {
		t.Fatalf("unexpected first scan output:\n%s", first)
	}

	second := runScanCommand(t, repo, dataDir)
	if !strings.Contains(second, "Files:") || !strings.Contains(second, "1 seen, 0 parsed, 1 skipped") {
		t.Fatalf("unexpected repeat scan output:\n%s", second)
	}
}

func TestRepresentCommandPrintsMaterializationCounts(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {
	helper()
}

func helper() {}
`)
	dataDir := t.TempDir()

	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"represent", repo, "--data-dir", dataDir, "--embedding-provider", "none"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("represent command: %v\n%s", err, out.String())
	}
	text := out.String()
	for _, expected := range []string{"Filter run:", "Represent run:", "Elements:", "Connectors:", "Representation:"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("represent output missing %q:\n%s", expected, text)
		}
	}
}

func TestScanCommandJSONRespectsLanguageFlag(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc Main() {}\n")
	writeFile(t, repo, "web/app.ts", "export function render() { return 1 }\n")
	dataDir := t.TempDir()

	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"scan", repo, "--data-dir", dataDir, "--language", "typescript", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scan command: %v\n%s", err, out.String())
	}
	var result struct {
		FilesSeen   int `json:"files_seen"`
		FilesParsed int `json:"files_parsed"`
		SymbolsSeen int `json:"symbols_seen"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON output %q: %v", out.String(), err)
	}
	if result.FilesSeen != 1 || result.FilesParsed != 1 || result.SymbolsSeen == 0 {
		t.Fatalf("expected only TypeScript file in JSON scan result, got %+v\n%s", result, out.String())
	}
}

func TestDiffCommandJSONAndFailOnDrift(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {}
`)
	dataDir := t.TempDir()

	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"diff", repo, "--data-dir", dataDir, "--embedding-provider", "none"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("diff command: %v\n%s", err, out.String())
	}
	var payload struct {
		Changed bool `json:"changed"`
		Scan    struct {
			FilesSeen int `json:"files_seen"`
		} `json:"scan"`
		Diffs []struct {
			ChangeType   string  `json:"change_type"`
			ResourceType *string `json:"resource_type"`
		} `json:"diffs"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON output %q: %v", out.String(), err)
	}
	if !payload.Changed || payload.Scan.FilesSeen != 1 || len(payload.Diffs) == 0 {
		t.Fatalf("unexpected diff payload: %+v\n%s", payload, out.String())
	}

	cmd = NewWatchCmd()
	out.Reset()
	var errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"diff", repo, "--data-dir", dataDir, "--embedding-provider", "none", "--fail-on-drift"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "drift detected") {
		t.Fatalf("expected fail-on-drift error, got %v\nstdout:\n%s\nstderr:\n%s", err, out.String(), errOut.String())
	}
	var driftPayload struct {
		Changed bool `json:"changed"`
	}
	if err := json.NewDecoder(strings.NewReader(out.String())).Decode(&driftPayload); err != nil || !driftPayload.Changed {
		t.Fatalf("fail-on-drift should print a JSON payload before usage text, payload=%+v err=%v output=%q", driftPayload, err, out.String())
	}
}

func TestResolveEmbeddingConfigPrecedence(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("TLD_EMBEDDING_PROVIDER", "local-deterministic-test")
	t.Setenv("TLD_EMBEDDING_MODEL", "env-model")
	t.Setenv("TLD_EMBEDDING_DIMENSION", "7")

	// Write a config file to test that env overrides it
	writeFile(t, configDir, "tld.yaml", "watch:\n  embedding:\n    provider: ollama\n    model: config-model\n")

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}

	resolved := resolveEmbeddingConfig(cfg, "none", "", "", 0)
	if resolved.Provider != "none" {
		t.Fatalf("flag provider should win over env/config, got %+v", resolved)
	}

	resolved = resolveEmbeddingConfig(cfg, "", "", "", 0)
	if resolved.Provider != "local-deterministic-test" || resolved.Model != "env-model" || resolved.Dimension != 7 {
		t.Fatalf("env should win over config, got %+v", resolved)
	}
}

func TestResolveWatchSettingsPrecedence(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	t.Setenv("TLD_WATCH_LANGUAGES", "python,typescript")
	t.Setenv("TLD_WATCH_WATCHER", "poll")
	t.Setenv("TLD_WATCH_POLL_INTERVAL", "3s")
	t.Setenv("TLD_WATCH_DEBOUNCE", "250ms")

	// Write a config file to test that env overrides it
	writeFile(t, configDir, "tld.yaml", "watch:\n  languages: [go]\n  watcher: fsnotify\n  poll_interval: 9s\n  debounce: 8s\n  thresholds:\n    max_elements_per_view: 11\n    max_connectors_per_view: 12\n")

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}

	envResolved := resolveWatchSettings(cfg, nil, "", "", "", 0, 0, 0, 0)
	if strings.Join(envResolved.Languages, ",") != "python,typescript" ||
		envResolved.Watcher != "poll" ||
		envResolved.PollInterval != 3*time.Second ||
		envResolved.Debounce != 250*time.Millisecond ||
		envResolved.Thresholds.MaxElementsPerView != 11 ||
		envResolved.Thresholds.MaxConnectorsPerView != 12 {
		t.Fatalf("env/config precedence resolved incorrectly: %+v", envResolved)
	}

	flagResolved := resolveWatchSettings(cfg, []string{"java"}, "fsnotify", "1s", "2s", 21, 22, 23, 24)
	if strings.Join(flagResolved.Languages, ",") != "java" ||
		flagResolved.Watcher != "fsnotify" ||
		flagResolved.PollInterval != time.Second ||
		flagResolved.Debounce != 2*time.Second ||
		flagResolved.Thresholds.MaxElementsPerView != 21 ||
		flagResolved.Thresholds.MaxConnectorsPerView != 22 ||
		flagResolved.Thresholds.MaxIncomingPerElement != 23 ||
		flagResolved.Thresholds.MaxOutgoingPerElement != 24 {
		t.Fatalf("flag precedence resolved incorrectly: %+v", flagResolved)
	}
}

func runScanCommand(t *testing.T, repo, dataDir string) string {
	t.Helper()
	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"scan", repo, "--data-dir", dataDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("scan command: %v\n%s", err, out.String())
	}
	return out.String()
}

func initGitRepoNoCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
