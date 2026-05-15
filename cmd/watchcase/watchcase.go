package watchcase

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	assets "github.com/mertcikla/tld/v2"
	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/localserver"
	storepkg "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/watch"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	reviewCorrect    = "correct"
	reviewIncorrect  = "incorrect"
	reviewUnreviewed = "unreviewed"
)

type CaseManifest struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description,omitempty"`
	Baseline    string   `yaml:"baseline"`
	Patch       string   `yaml:"patch"`
	Languages   []string `yaml:"languages,omitempty"`
}

type ExpectedFile struct {
	Objects []ExpectedObject `yaml:"objects"`
}

type ExpectedObject struct {
	Kind     string `yaml:"kind"`
	Identity string `yaml:"identity"`
	Change   string `yaml:"change"`
	Review   string `yaml:"review"`
	Summary  string `yaml:"summary,omitempty"`
	Comment  string `yaml:"comment,omitempty"`
}

type ObjectDiff struct {
	Kind         string `json:"kind"`
	Identity     string `json:"identity"`
	Change       string `json:"change"`
	OwnerType    string `json:"owner_type"`
	OwnerKey     string `json:"owner_key"`
	ResourceType string `json:"resource_type"`
	Summary      string `json:"summary,omitempty"`
	AddedLines   int    `json:"added_lines,omitempty"`
	RemovedLines int    `json:"removed_lines,omitempty"`
	Review       string `json:"review"`
	Comment      string `json:"comment,omitempty"`
}

type CaseResult struct {
	Dir     string
	Name    string
	Objects []ObjectDiff
	TempDir string
}

type runOptions struct {
	KeepTemp bool
	Config   *workspace.Config
}

func NewWatchcaseCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "watchcase",
		Short: "Review and verify watch pipeline fixture cases",
	}
	c.AddCommand(newReviewCmd(), newRunCmd())
	return c
}

func newReviewCmd() *cobra.Command {
	var keepTemp bool
	c := &cobra.Command{
		Use:   "review <cases-dir>",
		Short: "Run watch cases and quickly annotate object diffs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cases, err := discoverCases(args[0])
			if err != nil {
				return err
			}
			if len(cases) == 0 {
				return fmt.Errorf("no watch cases found under %s", args[0])
			}
			cfg, err := workspace.LoadGlobalConfig()
			if err != nil {
				return err
			}
			return reviewCases(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cases, runOptions{KeepTemp: keepTemp, Config: cfg})
		},
	}
	c.Flags().BoolVar(&keepTemp, "keep-temp", false, "keep temporary repositories after each run")
	return c
}

func newRunCmd() *cobra.Command {
	var keepTemp bool
	c := &cobra.Command{
		Use:   "run <cases-dir>",
		Short: "Verify watch cases against saved annotations",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cases, err := discoverCases(args[0])
			if err != nil {
				return err
			}
			if len(cases) == 0 {
				return fmt.Errorf("no watch cases found under %s", args[0])
			}
			cfg, err := workspace.LoadGlobalConfig()
			if err != nil {
				return err
			}
			return runCases(cmd.Context(), cmd.OutOrStdout(), cases, runOptions{KeepTemp: keepTemp, Config: cfg})
		},
	}
	c.Flags().BoolVar(&keepTemp, "keep-temp", false, "keep temporary repositories after each run")
	return c
}

func discoverCases(root string) ([]string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var cases []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root {
			switch filepath.Base(path) {
			case "baseline", ".git":
				return filepath.SkipDir
			}
		}
		if _, err := os.Stat(filepath.Join(path, "fixture.yaml")); err == nil {
			cases = append(cases, path)
			if path != root {
				return filepath.SkipDir
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(cases)
	return cases, nil
}

func reviewCases(ctx context.Context, in io.Reader, out io.Writer, cases []string, opts runOptions) error {
	reader := bufio.NewReader(in)
	for i := 0; i < len(cases); i++ {
		caseDir := cases[i]
	rerun:
		result, expected, err := runCaseWithExpected(ctx, caseDir, opts)
		if err != nil {
			return err
		}
		for {
			printCase(out, i+1, len(cases), result)
			if result.TempDir != "" {
				_, _ = fmt.Fprintf(out, "Temp repo: %s\n", result.TempDir)
			}
			_, _ = fmt.Fprintln(out, "\nCommands: a=accept all, c N=correct, x N=incorrect, u N=unreviewed, e=edit patch, j=json-ish list, r=rerun, n=next, q=quit")
			_, _ = fmt.Fprint(out, "> ")
			line, err := reader.ReadString('\n')
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			line = strings.TrimSpace(line)
			if errors.Is(err, io.EOF) && line == "" {
				return nil
			}
			switch {
			case line == "", line == "n":
				if err := saveExpected(caseDir, expectedFromObjects(result.Objects, expected)); err != nil {
					return err
				}
				goto nextCase
			case line == "q":
				if err := saveExpected(caseDir, expectedFromObjects(result.Objects, expected)); err != nil {
					return err
				}
				return nil
			case line == "r":
				goto rerun
			case line == "a":
				for idx := range result.Objects {
					result.Objects[idx].Review = reviewCorrect
				}
				if err := saveExpected(caseDir, expectedFromObjects(result.Objects, expected)); err != nil {
					return err
				}
				continue
			case line == "e":
				if err := openPatchInEditor(caseDir); err != nil {
					_, _ = fmt.Fprintf(out, "editor failed: %v\n", err)
				}
				goto rerun
			case line == "j":
				printJSONishObjects(out, result.Objects)
				continue
			case strings.HasPrefix(line, "c "), strings.HasPrefix(line, "x "), strings.HasPrefix(line, "u "):
				if err := markObjects(result.Objects, line); err != nil {
					_, _ = fmt.Fprintf(out, "%v\n", err)
					continue
				}
				if err := saveExpected(caseDir, expectedFromObjects(result.Objects, expected)); err != nil {
					return err
				}
				continue
			default:
				_, _ = fmt.Fprintf(out, "unknown command %q\n", line)
			}
		}
	nextCase:
	}
	return nil
}

func runCases(ctx context.Context, out io.Writer, cases []string, opts runOptions) error {
	var failures []string
	for i, caseDir := range cases {
		result, expected, err := runCaseWithExpected(ctx, caseDir, opts)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", filepath.Base(caseDir), err))
			continue
		}
		errs := verifyObjects(result.Objects, expected)
		status := "ok"
		if len(errs) > 0 {
			status = "fail"
			for _, item := range errs {
				failures = append(failures, fmt.Sprintf("%s: %s", result.Name, item))
			}
		}
		_, _ = fmt.Fprintf(out, "[%d/%d] %s %s (%d objects)\n", i+1, len(cases), status, result.Name, len(result.Objects))
	}
	if len(failures) > 0 {
		_, _ = fmt.Fprintln(out, "\nFailures:")
		for _, failure := range failures {
			_, _ = fmt.Fprintf(out, "  - %s\n", failure)
		}
		return fmt.Errorf("%d watchcase failure(s)", len(failures))
	}
	return nil
}

func runCaseWithExpected(ctx context.Context, caseDir string, opts runOptions) (CaseResult, ExpectedFile, error) {
	expected, err := loadExpected(caseDir)
	if err != nil {
		return CaseResult{}, ExpectedFile{}, err
	}
	result, err := RunCase(ctx, caseDir, expected, opts)
	if err != nil {
		return CaseResult{}, ExpectedFile{}, err
	}
	return result, expected, nil
}

func RunCase(ctx context.Context, caseDir string, expected ExpectedFile, opts runOptions) (CaseResult, error) {
	manifest, err := loadManifest(caseDir)
	if err != nil {
		return CaseResult{}, err
	}
	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		name = filepath.Base(caseDir)
	}
	baseline := filepath.Join(caseDir, firstNonEmpty(manifest.Baseline, "baseline"))
	patch := filepath.Join(caseDir, firstNonEmpty(manifest.Patch, "change.patch"))
	if _, err := os.Stat(baseline); err != nil {
		return CaseResult{}, fmt.Errorf("baseline: %w", err)
	}
	if _, err := os.Stat(patch); err != nil {
		return CaseResult{}, fmt.Errorf("patch: %w", err)
	}
	tempRoot, err := os.MkdirTemp("", "tld-watchcase-*")
	if err != nil {
		return CaseResult{}, err
	}
	if !opts.KeepTemp {
		defer func() { _ = os.RemoveAll(tempRoot) }()
	}
	repo := filepath.Join(tempRoot, "repo")
	dataDir := filepath.Join(tempRoot, "data")
	if err := copyDir(baseline, repo); err != nil {
		return CaseResult{}, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return CaseResult{}, err
	}
	if err := initBaselineGit(repo); err != nil {
		return CaseResult{}, err
	}
	sqliteStore, err := storepkg.Open(localserver.DatabasePath(dataDir), assets.FS)
	if err != nil {
		return CaseResult{}, err
	}
	defer func(db *sql.DB) { _ = db.Close() }(sqliteStore.DB())
	watchStore := watch.NewStore(sqliteStore.DB())
	runner := watch.NewRunner(watchStore)
	cfg := opts.Config
	if cfg == nil {
		cfg = workspace.DefaultConfig()
	}
	settings := watch.ResolveSettings(cfg, manifest.Languages, "", "", "", 0, 0, 0, 0, 0)
	embedding := watch.ResolveEmbeddingConfig(cfg, "", "", "", 0)
	if hasEmbeddingHealthCheck(embedding) {
		checked, _, err := watch.CheckEmbeddingHealth(ctx, embedding)
		if err != nil {
			return CaseResult{}, fmt.Errorf("embedding healthcheck failed: %w", err)
		}
		embedding = checked
	}
	baselineRun, err := runner.RunOnce(ctx, watch.OneShotOptions{
		Path:      repo,
		Rescan:    true,
		Embedding: embedding,
		Settings:  settings,
		DataDir:   dataDir,
	})
	if err != nil {
		return CaseResult{}, fmt.Errorf("baseline watch run: %w", err)
	}
	head, _ := tldgit.DetectHeadCommit(repo)
	branch, _ := tldgit.DetectBranch(repo)
	if _, err := watchStore.CreateWatchVersion(ctx, baselineRun.Repository.ID, head, "watchcase baseline", "", branch, baselineRun.Representation.RepresentationHash, nil, nil); err != nil {
		return CaseResult{}, fmt.Errorf("record baseline watch version: %w", err)
	}
	if err := runGitApply(repo, patch); err != nil {
		return CaseResult{}, err
	}
	nextRun, err := runner.RunOnce(ctx, watch.OneShotOptions{
		Path:      repo,
		Rescan:    true,
		Embedding: embedding,
		Settings:  settings,
		DataDir:   dataDir,
	})
	if err != nil {
		return CaseResult{}, fmt.Errorf("patched watch run: %w", err)
	}
	objects := objectDiffs(nextRun.Diffs, expected)
	return CaseResult{Dir: caseDir, Name: name, Objects: objects, TempDir: keepTempPath(tempRoot, opts)}, nil
}

func loadManifest(caseDir string) (CaseManifest, error) {
	data, err := os.ReadFile(filepath.Join(caseDir, "fixture.yaml"))
	if err != nil {
		return CaseManifest{}, err
	}
	var manifest CaseManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return CaseManifest{}, err
	}
	return manifest, nil
}

func loadExpected(caseDir string) (ExpectedFile, error) {
	data, err := os.ReadFile(filepath.Join(caseDir, "expected.yaml"))
	if errors.Is(err, os.ErrNotExist) {
		return ExpectedFile{}, nil
	}
	if err != nil {
		return ExpectedFile{}, err
	}
	var expected ExpectedFile
	if err := yaml.Unmarshal(data, &expected); err != nil {
		return ExpectedFile{}, err
	}
	return expected, nil
}

func saveExpected(caseDir string, expected ExpectedFile) error {
	data, err := yaml.Marshal(expected)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(caseDir, "expected.yaml"), data, 0o644)
}

func objectDiffs(diffs []watch.RepresentationDiff, expected ExpectedFile) []ObjectDiff {
	reviews := map[string]ExpectedObject{}
	for _, item := range expected.Objects {
		reviews[objectKey(item.Kind, item.Identity, item.Change)] = item
	}
	var out []ObjectDiff
	for _, diff := range diffs {
		kind := objectKind(diff)
		if kind == "" {
			continue
		}
		identity := objectIdentity(kind, diff)
		item := ObjectDiff{
			Kind:         kind,
			Identity:     identity,
			Change:       normalizedChange(diff.ChangeType),
			OwnerType:    diff.OwnerType,
			OwnerKey:     diff.OwnerKey,
			ResourceType: strings.TrimSpace(ptrString(diff.ResourceType)),
			Summary:      strings.TrimSpace(ptrString(diff.Summary)),
			AddedLines:   diff.AddedLines,
			RemovedLines: diff.RemovedLines,
			Review:       reviewUnreviewed,
		}
		if saved, ok := reviews[objectKey(item.Kind, item.Identity, item.Change)]; ok {
			item.Review = normalizeReview(saved.Review)
			item.Comment = strings.TrimSpace(saved.Comment)
		}
		out = append(out, item)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Change != out[j].Change {
			return out[i].Change < out[j].Change
		}
		return out[i].Identity < out[j].Identity
	})
	return out
}

func objectKind(diff watch.RepresentationDiff) string {
	resourceType := strings.TrimSpace(ptrString(diff.ResourceType))
	switch resourceType {
	case "element", "connector", "view":
		return resourceType
	default:
		return ""
	}
}

func objectIdentity(kind string, diff watch.RepresentationDiff) string {
	key := strings.TrimSpace(diff.OwnerKey)
	if key == "" && diff.ResourceID != nil {
		key = strconv.FormatInt(*diff.ResourceID, 10)
	}
	return kind + ":" + strings.TrimSpace(diff.OwnerType) + ":" + key
}

func expectedFromObjects(objects []ObjectDiff, previous ExpectedFile) ExpectedFile {
	expected := ExpectedFile{Objects: make([]ExpectedObject, 0, len(objects))}
	seen := map[string]struct{}{}
	for _, object := range objects {
		key := objectKey(object.Kind, object.Identity, object.Change)
		seen[key] = struct{}{}
		expected.Objects = append(expected.Objects, ExpectedObject{
			Kind:     object.Kind,
			Identity: object.Identity,
			Change:   object.Change,
			Review:   normalizeReview(object.Review),
			Summary:  object.Summary,
			Comment:  strings.TrimSpace(object.Comment),
		})
	}
	for _, item := range previous.Objects {
		if normalizeReview(item.Review) != reviewCorrect {
			continue
		}
		if _, ok := seen[objectKey(item.Kind, item.Identity, item.Change)]; ok {
			continue
		}
		expected.Objects = append(expected.Objects, item)
	}
	return expected
}

func verifyObjects(objects []ObjectDiff, expected ExpectedFile) []string {
	current := map[string]ObjectDiff{}
	for _, object := range objects {
		current[objectKey(object.Kind, object.Identity, object.Change)] = object
	}
	var errs []string
	for _, object := range objects {
		switch normalizeReview(object.Review) {
		case reviewCorrect:
		case reviewIncorrect:
			errs = append(errs, fmt.Sprintf("still produces incorrect %s %s %s", object.Change, object.Kind, object.Identity))
		default:
			errs = append(errs, fmt.Sprintf("unreviewed %s %s %s", object.Change, object.Kind, object.Identity))
		}
	}
	for _, item := range expected.Objects {
		if normalizeReview(item.Review) != reviewCorrect {
			continue
		}
		if _, ok := current[objectKey(item.Kind, item.Identity, item.Change)]; !ok {
			errs = append(errs, fmt.Sprintf("missing expected correct %s %s %s", item.Change, item.Kind, item.Identity))
		}
	}
	sort.Strings(errs)
	return errs
}

func printCase(out io.Writer, idx, total int, result CaseResult) {
	_, _ = fmt.Fprintf(out, "\n[%d/%d] %s\n", idx, total, result.Name)
	if len(result.Objects) == 0 {
		_, _ = fmt.Fprintln(out, "  no element/connector/view diffs")
		return
	}
	for i, object := range result.Objects {
		lines := ""
		if object.AddedLines != 0 || object.RemovedLines != 0 {
			lines = fmt.Sprintf(" +%d -%d", object.AddedLines, object.RemovedLines)
		}
		summary := object.Summary
		if summary == "" {
			summary = object.Identity
		}
		_, _ = fmt.Fprintf(out, "  %2d. %-10s %-7s %-10s %s%s\n", i+1, "["+object.Review+"]", object.Kind, object.Change, summary, lines)
		_, _ = fmt.Fprintf(out, "      %s\n", object.Identity)
		if object.Comment != "" {
			_, _ = fmt.Fprintf(out, "      comment: %s\n", object.Comment)
		}
	}
}

func printJSONishObjects(out io.Writer, objects []ObjectDiff) {
	for i, object := range objects {
		_, _ = fmt.Fprintf(out, "%d kind=%s change=%s review=%s identity=%q summary=%q\n", i+1, object.Kind, object.Change, object.Review, object.Identity, object.Summary)
	}
}

func markObjects(objects []ObjectDiff, line string) error {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return fmt.Errorf("usage: %s N", fields[0])
	}
	mark := map[string]string{"c": reviewCorrect, "x": reviewIncorrect, "u": reviewUnreviewed}[fields[0]]
	index, err := strconv.Atoi(fields[1])
	if err != nil || index < 1 || index > len(objects) {
		return fmt.Errorf("object index must be between 1 and %d", len(objects))
	}
	objects[index-1].Review = mark
	return nil
}

func openPatchInEditor(caseDir string) error {
	manifest, err := loadManifest(caseDir)
	if err != nil {
		return err
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		return fmt.Errorf("EDITOR is not set")
	}
	patch := filepath.Join(caseDir, firstNonEmpty(manifest.Patch, "change.patch"))
	cmd := exec.Command(editor, patch)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, info.Mode().Perm())
	})
}

func initBaselineGit(repo string) error {
	if err := runCommand(repo, "git", "init"); err != nil {
		return err
	}
	if err := runCommand(repo, "git", "config", "user.email", "watchcase@example.com"); err != nil {
		return err
	}
	if err := runCommand(repo, "git", "config", "user.name", "Watchcase"); err != nil {
		return err
	}
	if err := runCommand(repo, "git", "add", "."); err != nil {
		return err
	}
	if err := runCommand(repo, "git", "commit", "-m", "watchcase baseline"); err != nil {
		return err
	}
	return nil
}

func runGitApply(repo, patch string) error {
	return runCommand(repo, "git", "apply", "--whitespace=nowarn", patch)
}

func runCommand(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, string(out))
	}
	return nil
}

func hasEmbeddingHealthCheck(embedding watch.EmbeddingConfig) bool {
	provider := strings.TrimSpace(embedding.Provider)
	return provider != "" && provider != "none" && provider != "local-lexical"
}

func objectKey(kind, identity, change string) string {
	return strings.TrimSpace(kind) + "\x00" + strings.TrimSpace(identity) + "\x00" + normalizedChange(change)
}

func normalizedChange(change string) string {
	change = strings.TrimSpace(change)
	if change == "" {
		return "updated"
	}
	return change
}

func normalizeReview(review string) string {
	switch strings.TrimSpace(review) {
	case reviewCorrect:
		return reviewCorrect
	case reviewIncorrect:
		return reviewIncorrect
	default:
		return reviewUnreviewed
	}
}

func ptrString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func keepTempPath(tempRoot string, opts runOptions) string {
	if opts.KeepTemp {
		return tempRoot
	}
	return ""
}
