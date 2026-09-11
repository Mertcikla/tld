package watch

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/localserver"
	storepkg "github.com/mertcikla/tld/v2/internal/store"
	watchpkg "github.com/mertcikla/tld/v2/internal/watch"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestWatchSubcommandsFailClearlyOutsideGitRepositoryWithoutRepositoryRows(t *testing.T) {
	for _, subcommand := range []string{"scan", "represent", "diff"} {
		t.Run(subcommand, func(t *testing.T) {
			dataDir := t.TempDir()
			cmd := NewWatchCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			args := []string{subcommand, t.TempDir(), "--data-dir", dataDir}
			if subcommand == "represent" || subcommand == "diff" {
				args = append(args, "--embedding-provider", "none")
			}
			cmd.SetArgs(args)

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "not inside a git repository") {
				t.Fatalf("expected outside-git error, got %v\n%s", err, out.String())
			}

			sqliteStore, err := storepkg.Open(localserver.DatabasePath(dataDir), assets.FS)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = sqliteStore.DB().Close() }()
			var repositories int
			if err := sqliteStore.DB().QueryRow(`SELECT COUNT(*) FROM watch_repositories`).Scan(&repositories); err != nil {
				t.Fatal(err)
			}
			if repositories != 0 {
				t.Fatalf("expected no watch repository rows after failed %s, found %d", subcommand, repositories)
			}
		})
	}
}

func TestConfirmLSPProceedWarnsAndContinuesForNonInteractiveInput(t *testing.T) {
	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetIn(strings.NewReader(""))

	status := watchpkg.LSPStatus{
		Enabled: true,
		Summary: watchpkg.LSPStatusSummary{Requested: 1, Unavailable: 1},
		Servers: []watchpkg.LSPServerStatus{{
			Language:  "go",
			Command:   "gopls",
			State:     "unavailable",
			LastError: "no installed LSP server found",
		}},
	}
	if err := confirmLSPProceed(cmd, status); err != nil {
		t.Fatalf("confirmLSPProceed: %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"tlDiagram is missing language servers and cannot analyze your code accurately",
		"Go (gopls): not found in PATH",
		"Error: no installed LSP server found",
		`Override: tld config set watch.lsp.commands.go "/path/to/gopls"`,
		"Remediation: install the missing language server(s) or ensure they are on your PATH",
		"Continuing without language servers; analysis quality will be degraded",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in output:\n%s", want, got)
		}
	}
}

func TestLogWatchEventUpdatesWorkspaceStatusInPlace(t *testing.T) {
	cmd := NewWatchCmd()
	var stdout bytes.Buffer
	var status bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	activity := newWatchActivityProgressWithOptions(&status, func() int { return 1 }, watchActivityOptions{
		ForceTerminal: true,
		DisableTicker: true,
		Width:         200,
		Now:           func() time.Time { return time.Unix(0, 0) },
	})
	activity.Start("Workspace status: current")

	event := watchpkg.Event{
		Type:         "source.changed",
		ChangedFiles: 2,
		Data: watchpkg.SourceFileChangeResult{
			Change:                watchpkg.SourceFileChange{Path: "cmd/service/main.go", ChangeType: "modified"},
			RepresentationChanged: true,
			Representation: watchpkg.RepresentResult{
				ElementsUpdated:   28,
				ConnectorsUpdated: 4,
			},
			GitTags: watchpkg.GitTagUpdateResult{TagsAdded: 13},
		},
	}
	if !logWatchEvent(cmd, event, activity) {
		t.Fatalf("expected source.changed to be handled")
	}
	if got := stdout.String(); got != "" {
		t.Fatalf("source.changed should not print verbose source output, got %q", got)
	}
	got := status.String()
	for _, want := range []string{"\r\033[K", "Workspace status: current", "2 files changed", "elements", "28", "connectors", "4", "tags", "13"} {
		if !strings.Contains(got, want) {
			t.Fatalf("status output missing %q:\n%q", want, got)
		}
	}
	for _, unwanted := range []string{"\n", "source modified", "cmd/service/main.go", "Watch elapsed"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("status output should not contain %q:\n%q", unwanted, got)
		}
	}
}

func TestWatchActivityProgressPrintsStopDurationOnce(t *testing.T) {
	started := time.Unix(100, 0)
	now := started
	var status bytes.Buffer
	activity := newWatchActivityProgressWithOptions(&status, nil, watchActivityOptions{
		ForceTerminal: true,
		DisableTicker: true,
		Width:         80,
		Now:           func() time.Time { return now },
	})
	activity.Start("Workspace status: current")
	now = now.Add(65 * time.Second)

	elapsed, stopped := activity.Stop()
	if !stopped {
		t.Fatalf("expected first stop to report elapsed time")
	}
	if got, want := formatWatchActivityDuration(elapsed), "1m05s"; got != want {
		t.Fatalf("elapsed = %q, want %q", got, want)
	}
	if _, stoppedAgain := activity.Stop(); stoppedAgain {
		t.Fatalf("second stop should be ignored")
	}
	got := status.String()
	if !strings.Contains(got, "\r\033[K- Workspace status: current") {
		t.Fatalf("expected spinner status line, got %q", got)
	}
	if strings.Contains(got, "Watch elapsed") || strings.Count(got, "\n") > 0 {
		t.Fatalf("status should update in place without source file chatter:\n%q", got)
	}
}

func TestLogWatchStoppedPrintsTotalDuration(t *testing.T) {
	cmd := NewWatchCmd()
	var stdout bytes.Buffer
	var status bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	started := time.Unix(100, 0)
	now := started
	activity := newWatchActivityProgressWithOptions(&status, nil, watchActivityOptions{
		ForceTerminal: true,
		DisableTicker: true,
		Width:         80,
		Now:           func() time.Time { return now },
	})
	activity.Start("Workspace status: current")
	now = now.Add(2*time.Minute + 3*time.Second)

	if !logWatchEvent(cmd, watchpkg.Event{Type: "watch.stopped"}, activity) {
		t.Fatalf("expected watch.stopped to be handled")
	}
	if got, want := stdout.String(), "watch stopped after 2m03s\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := status.String(); !strings.HasSuffix(got, "\r\033[K") {
		t.Fatalf("expected pinned status line to be cleared, got %q", got)
	}
}

func TestLogWatchEventSuppressesLSPStatusUpdates(t *testing.T) {
	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	event := watchpkg.Event{
		Type: "lsp.status",
		Data: watchpkg.LSPStatus{
			Enabled: true,
			Summary: watchpkg.LSPStatusSummary{
				Requested:   1,
				Available:   1,
				Unavailable: 1,
				Restarted:   1,
			},
		},
	}
	if !logWatchEvent(cmd, event, nil) {
		t.Fatalf("expected lsp.status to be handled")
	}
	if out.String() != "" {
		t.Fatalf("lsp.status should not print CLI updates, got %q", out.String())
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

func TestWatchDryRunGroupsSameDiffPayloadAsDiffCommand(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {}
`)
	dataDir := t.TempDir()

	dryRunCmd := NewWatchCmd()
	var dryRunOut bytes.Buffer
	dryRunCmd.SetOut(&dryRunOut)
	dryRunCmd.SetErr(&dryRunOut)
	dryRunCmd.SetArgs([]string{"--dry-run", repo, "--data-dir", dataDir, "--embedding-provider", "none"})
	if err := dryRunCmd.Execute(); err != nil {
		t.Fatalf("watch --dry-run command: %v\n%s", err, dryRunOut.String())
	}
	var dryRunPayload struct {
		Changed bool                                                `json:"changed"`
		Diffs   map[string]map[string][]watchpkg.RepresentationDiff `json:"diffs"`
	}
	if err := json.Unmarshal(dryRunOut.Bytes(), &dryRunPayload); err != nil {
		t.Fatalf("invalid dry-run JSON output %q: %v", dryRunOut.String(), err)
	}
	if !dryRunPayload.Changed || len(dryRunPayload.Diffs) == 0 {
		t.Fatalf("unexpected dry-run payload: %+v\n%s", dryRunPayload, dryRunOut.String())
	}
	if _, ok := dryRunPayload.Diffs["added"]["element"]; !ok {
		t.Fatalf("expected dry-run diffs to be grouped by change_type then resource_type, got %+v", dryRunPayload.Diffs)
	}
	if strings.Contains(dryRunOut.String(), "Watching") {
		t.Fatalf("dry-run should exit after printing JSON, got watch output:\n%s", dryRunOut.String())
	}
	dryRunLog, err := os.ReadFile(localserver.LogPath(dataDir))
	if err != nil {
		t.Fatalf("read dry-run watch log: %v", err)
	}
	if !strings.Contains(string(dryRunLog), "msg=watch.diff.started") || !strings.Contains(string(dryRunLog), "msg=watch.diff.completed") || !strings.Contains(string(dryRunLog), "msg=watch.scan.file") {
		t.Fatalf("watch dry-run should log pipeline details:\n%s", string(dryRunLog))
	}
	if strings.Contains(dryRunOut.String(), "msg=watch.") {
		t.Fatalf("dry-run JSON stdout should not contain log lines:\n%s", dryRunOut.String())
	}

	diffCmd := NewWatchCmd()
	var diffOut bytes.Buffer
	diffCmd.SetOut(&diffOut)
	diffCmd.SetErr(&diffOut)
	diffCmd.SetArgs([]string{"diff", repo, "--data-dir", dataDir, "--embedding-provider", "none"})
	if err := diffCmd.Execute(); err != nil {
		t.Fatalf("watch diff command: %v\n%s", err, diffOut.String())
	}
	var diffPayload struct {
		Diffs []watchpkg.RepresentationDiff `json:"diffs"`
	}
	if err := json.Unmarshal(diffOut.Bytes(), &diffPayload); err != nil {
		t.Fatalf("invalid diff JSON output %q: %v", diffOut.String(), err)
	}
	if !sameDiffPayload(flattenGroupedDiffPayload(dryRunPayload.Diffs), diffPayload.Diffs) {
		t.Fatalf("watch --dry-run diffs should match watch diff diffs\n dry-run: %+v\n diff: %+v", dryRunPayload.Diffs, diffPayload.Diffs)
	}
}

func TestWatchCommandWritesRuntimeLogWithoutBanner(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", "package main\nfunc Main() {}\n")
	dataDir := t.TempDir()

	ctx, cancel := context.WithTimeout(context.Background(), 750*time.Millisecond)
	defer cancel()
	cmd := NewWatchCmd()
	cmd.SetContext(ctx)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{repo, "--data-dir", dataDir, "--embedding-provider", "none", "--no-serve"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("watch command: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "░███████") {
		t.Fatalf("watch stdout should use compact ready output instead of ASCII logo:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "tld ") || !strings.Contains(out.String(), "Workspace\n") || !strings.Contains(out.String(), "Watching") {
		t.Fatalf("watch stdout should contain compact ready output:\n%s", out.String())
	}
	logData, err := os.ReadFile(localserver.LogPath(dataDir))
	if err != nil {
		t.Fatalf("read watch log: %v", err)
	}
	logText := string(logData)
	for _, want := range []string{
		"msg=watch.command.started",
		"msg=watch.server.skipped",
		"msg=watch.runner.started",
		"msg=watch.runner.ready",
		"msg=watch.event",
		"type=watch.started",
		"msg=watch.runner.stopped",
		"msg=watch.command.completed",
	} {
		if !strings.Contains(logText, want) {
			t.Fatalf("watch log missing %q:\n%s", want, logText)
		}
	}
	if strings.Contains(logText, "░███████") || strings.Contains(logText, "Version:") {
		t.Fatalf("watch log should not contain startup banner/logo:\n%s", logText)
	}
}

func TestWatchCommandDetectsAddedFileAndMaterializesSymbol(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {}
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")
	dataDir := t.TempDir()

	cancel, done, out := startPollingWatchCommand(t, repo, dataDir)
	defer cancel()
	waitForWatchLogContains(t, dataDir, "msg=watch.command.ready")

	sqliteStore := openWatchCommandTestStore(t, dataDir)
	defer func() { _ = sqliteStore.DB().Close() }()

	writeFile(t, repo, "pkg/added.go", `package pkg

func AddedLive() string {
	return "added"
}
`)

	waitForWatchLogContains(t, dataDir, "type=source.changed")
	waitForWatchLogContains(t, dataDir, "path=pkg/added.go")
	waitForWatchDBCondition(t, sqliteStore.DB(), "AddedLive element", func(db *sql.DB) bool {
		return watchElementNameExists(t, db, "AddedLive")
	})

	cancel()
	waitForWatchCommandDone(t, done, out)
}

func TestWatchCommandDetectsModifiedSymbolAndMaterializesReference(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {}

func Helper() string {
	return "helper"
}
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")
	dataDir := t.TempDir()

	cancel, done, out := startPollingWatchCommand(t, repo, dataDir)
	defer cancel()
	waitForWatchLogContains(t, dataDir, "msg=watch.command.ready")

	sqliteStore := openWatchCommandTestStore(t, dataDir)
	defer func() { _ = sqliteStore.DB().Close() }()

	writeFile(t, repo, "main.go", `package main

func Main() {}

func Helper() string {
	return "helper"
}

func NewCaller() string {
	return Helper()
}
`)

	waitForWatchLogContains(t, dataDir, "type=source.changed")
	waitForWatchLogContains(t, dataDir, "path=main.go")
	waitForWatchDBCondition(t, sqliteStore.DB(), "NewCaller materialization", func(db *sql.DB) bool {
		return watchElementNameExists(t, db, "NewCaller") && watchConnectorExistsBetween(t, db, "NewCaller", "Helper")
	})

	cancel()
	waitForWatchCommandDone(t, done, out)
}

func TestWatchDryRunCleanHeadInitializesWithoutDrift(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {}
`)
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "initial")
	dataDir := t.TempDir()

	cmd := NewWatchCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--dry-run", repo, "--data-dir", dataDir, "--embedding-provider", "none"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("watch --dry-run command: %v\n%s", err, out.String())
	}
	var payload struct {
		Changed bool `json:"changed"`
		Diffs   map[string]map[string][]struct {
			ChangeType   string  `json:"change_type"`
			ResourceType *string `json:"resource_type"`
		} `json:"diffs"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid dry-run JSON output %q: %v", out.String(), err)
	}
	if payload.Changed {
		t.Fatalf("clean HEAD dry-run should initialize without drift, got %+v", payload)
	}
	for _, byResource := range payload.Diffs {
		for _, diffs := range byResource {
			for _, diff := range diffs {
				if diff.ResourceType != nil && diff.ChangeType == "added" {
					t.Fatalf("clean HEAD dry-run should not include added resource diffs, got %+v", payload.Diffs)
				}
			}
		}
	}
}

func flattenGroupedDiffPayload(grouped map[string]map[string][]watchpkg.RepresentationDiff) []watchpkg.RepresentationDiff {
	var out []watchpkg.RepresentationDiff
	for _, byResource := range grouped {
		for _, diffs := range byResource {
			out = append(out, diffs...)
		}
	}
	return out
}

func sameDiffPayload(a, b []watchpkg.RepresentationDiff) bool {
	canonical := func(diffs []watchpkg.RepresentationDiff) []string {
		out := make([]string, 0, len(diffs))
		for _, diff := range diffs {
			data, _ := json.Marshal(diff)
			out = append(out, string(data))
		}
		sort.Strings(out)
		return out
	}
	left := canonical(a)
	right := canonical(b)
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func startPollingWatchCommand(t *testing.T, repo, dataDir string) (context.CancelFunc, <-chan error, *bytes.Buffer) {
	t.Helper()
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cmd := NewWatchCmd()
	cmd.SetContext(ctx)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		repo,
		"--data-dir", dataDir,
		"--embedding-provider", "none",
		"--no-serve",
		"--watcher", "poll",
		"--poll-interval", "50ms",
		"--debounce", "10ms",
	})
	done := make(chan error, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		done <- cmd.Execute()
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-stopped:
		case <-time.After(2 * time.Second):
		}
	})
	return cancel, done, &out
}

func waitForWatchCommandDone(t *testing.T, done <-chan error, out *bytes.Buffer) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("watch command failed: %v\n%s", err, out.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("watch command did not stop\n%s", out.String())
	}
}

func waitForWatchLogContains(t *testing.T, dataDir, needle string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	logPath := localserver.LogPath(dataDir)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(logPath)
		if err == nil && strings.Contains(string(data), needle) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	data, _ := os.ReadFile(logPath)
	t.Fatalf("watch log did not contain %q:\n%s", needle, string(data))
}

func openWatchCommandTestStore(t *testing.T, dataDir string) *storepkg.SQLiteStore {
	t.Helper()
	sqliteStore, err := storepkg.Open(localserver.DatabasePath(dataDir), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	return sqliteStore
}

func waitForWatchDBCondition(t *testing.T, db *sql.DB, label string, condition func(*sql.DB) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition(db) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("watch DB condition did not become true: %s", label)
}

func watchElementNameExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var id int64
	err := db.QueryRow(`SELECT id FROM elements WHERE name = ? ORDER BY id LIMIT 1`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
}

func watchConnectorExistsBetween(t *testing.T, db *sql.DB, sourceName, targetName string) bool {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		SELECT c.id
		FROM connectors c
		JOIN elements s ON s.id = c.source_element_id
		JOIN elements target ON target.id = c.target_element_id
		WHERE s.name = ? AND target.name = ?
		ORDER BY c.id
		LIMIT 1`, sourceName, targetName).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		t.Fatal(err)
	}
	return true
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

	resolved := resolveEmbeddingConfig(cfg, "none", "", "", 0, 0)
	if resolved.Provider != "none" {
		t.Fatalf("flag provider should win over env/config, got %+v", resolved)
	}

	resolved = resolveEmbeddingConfig(cfg, "", "", "", 0, 0)
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

	envResolved := resolveWatchSettings(cfg, nil, "", "", "", 0, 0, 0, 0, 0)
	if strings.Join(envResolved.Languages, ",") != "python,typescript" ||
		envResolved.Watcher != "poll" ||
		envResolved.PollInterval != 3*time.Second ||
		envResolved.Debounce != 250*time.Millisecond ||
		envResolved.Thresholds.MaxElementsPerView != 11 ||
		envResolved.Thresholds.MaxConnectorsPerView != 12 {
		t.Fatalf("env/config precedence resolved incorrectly: %+v", envResolved)
	}

	flagResolved := resolveWatchSettings(cfg, []string{"java"}, "fsnotify", "1s", "2s", 21, 22, 23, 24, 25)
	if strings.Join(flagResolved.Languages, ",") != "java" ||
		flagResolved.Watcher != "fsnotify" ||
		flagResolved.PollInterval != time.Second ||
		flagResolved.Debounce != 2*time.Second ||
		flagResolved.Thresholds.MaxElementsPerView != 21 ||
		flagResolved.Thresholds.MaxConnectorsPerView != 22 ||
		flagResolved.Thresholds.MaxIncomingPerElement != 23 ||
		flagResolved.Thresholds.MaxOutgoingPerElement != 24 ||
		flagResolved.Thresholds.MaxExpandedConnectorsPerGroup != 25 {
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

func TestIsInteractiveInputTreatsNullAndPipesAsNonInteractive(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err == nil {
		defer func() { _ = devNull.Close() }()
		if isInteractiveInput(devNull) {
			t.Fatalf("expected %s to be treated as non-interactive", os.DevNull)
		}
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()
	if isInteractiveInput(r) {
		t.Fatal("expected pipe to be treated as non-interactive")
	}

	if isInteractiveInput(strings.NewReader("yes\n")) {
		t.Fatal("expected non-file reader to be treated as non-interactive")
	}
}
