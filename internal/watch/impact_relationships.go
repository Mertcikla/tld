package watch

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	tldgit "github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// RelationshipOptions configures observed relationship detection. It runs a
// focused scan of the changed files through the existing evidence engine
// (Tree-sitter symbols plus LSP reference resolution when available).
type RelationshipOptions struct {
	RepoRoot     string
	RemoteURL    string
	Branch       string
	HeadCommit   string
	ChangedFiles []string
	Elements     map[string]*workspace.Element
	Settings     Settings
	DataDir      string
	Force        bool
	Logger       EventLogger
	Progress     ProgressSink
}

// DetectObservedRelationships scans changed files and maps resolved references
// onto authored elements. It returns evidence only and never mutates bindings or
// architecture.
func DetectObservedRelationships(ctx context.Context, store *Store, opts RelationshipOptions) ([]RelationshipEvidence, error) {
	if store == nil || len(opts.ChangedFiles) == 0 {
		return nil, nil
	}
	repoRoot := opts.RepoRoot
	if resolved, err := tldgit.RepoRoot(repoRoot); err == nil && strings.TrimSpace(resolved) != "" {
		repoRoot = resolved
	}
	settings := NormalizeSettings(opts.Settings)
	scanner := NewScanner(store)
	scanner.Settings = settings
	scanner.Logger = opts.Logger
	scanner.Progress = opts.Progress
	defer func() { _ = scanner.Close() }()

	repoInput := RepositoryInput{
		RemoteURL:    opts.RemoteURL,
		RepoRoot:     repoRoot,
		DisplayName:  filepath.Base(repoRoot),
		Branch:       opts.Branch,
		HeadCommit:   opts.HeadCommit,
		SettingsHash: stableHash(settings),
	}
	repo, err := store.EnsureRepository(ctx, repoInput)
	if err != nil {
		return nil, err
	}
	scan, err := scanner.ScanFilesWithOptions(ctx, repo, opts.ChangedFiles, ScanOptions{Force: opts.Force, DataDir: opts.DataDir})
	if err != nil {
		return nil, err
	}

	level := EvidenceModerate
	if scan.LSP.Summary.Active > 0 {
		level = EvidenceStrong
	}

	bindings := DeriveBindings(opts.Elements, repoRoot, opts.RemoteURL)
	elementForSymbol := func(symbol Symbol) string {
		for _, binding := range bindings {
			switch {
			case binding.Pattern != "":
				if bindingPatternMatch(binding.Pattern, symbol.FilePath) {
					return binding.ElementRef
				}
			case binding.Name != "":
				if namePathMatch(binding.Name, symbol.FilePath) {
					return binding.ElementRef
				}
			}
		}
		return ""
	}

	relFiles := make([]string, 0, len(opts.ChangedFiles))
	for _, file := range opts.ChangedFiles {
		if normalized := normalizeCodePath(file); normalized != "" {
			relFiles = append(relFiles, normalized)
		}
	}
	symbols, err := store.QuerySymbolsByFiles(ctx, repo.ID, relFiles)
	if err != nil {
		return nil, err
	}
	sourceByID := make(map[int64]Symbol, len(symbols))
	sourceIDs := make([]int64, 0, len(symbols))
	for _, symbol := range symbols {
		sourceByID[symbol.ID] = symbol
		sourceIDs = append(sourceIDs, symbol.ID)
	}
	if len(sourceIDs) == 0 {
		return nil, nil
	}
	refs, err := store.QueryReferencesBySourceIDs(ctx, repo.ID, sourceIDs)
	if err != nil {
		return nil, err
	}
	targetIDs := make([]int64, 0, len(refs))
	for _, ref := range refs {
		targetIDs = append(targetIDs, ref.TargetSymbolID)
	}
	targets, err := store.QuerySymbolsByIDs(ctx, repo.ID, targetIDs)
	if err != nil {
		return nil, err
	}
	targetByID := make(map[int64]Symbol, len(targets))
	for _, symbol := range targets {
		targetByID[symbol.ID] = symbol
	}

	seen := map[string]struct{}{}
	var out []RelationshipEvidence
	for _, ref := range refs {
		source, ok := sourceByID[ref.SourceSymbolID]
		if !ok {
			continue
		}
		target, ok := targetByID[ref.TargetSymbolID]
		if !ok {
			continue
		}
		sourceRef := elementForSymbol(source)
		targetRef := elementForSymbol(target)
		if sourceRef == "" || targetRef == "" || sourceRef == targetRef {
			continue
		}
		key := sourceRef + "\x00" + targetRef + "\x00" + normalizeCodePath(source.FilePath)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, RelationshipEvidence{
			SourceRef: sourceRef,
			TargetRef: targetRef,
			File:      normalizeCodePath(source.FilePath),
			Line:      ref.Line,
			Kind:      firstNonEmpty(strings.TrimSpace(ref.Kind), "call"),
			Level:     level,
			Observed:  true,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceRef != out[j].SourceRef {
			return out[i].SourceRef < out[j].SourceRef
		}
		if out[i].TargetRef != out[j].TargetRef {
			return out[i].TargetRef < out[j].TargetRef
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}
