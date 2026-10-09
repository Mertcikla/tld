// Package suggest ranks codeindex primitives (files, symbols, repositories) as
// source-link candidates for a workspace element. It powers the `tld link`
// command: given an element's name/kind/technology it proposes the most likely
// files and declarations to link to.
//
// Ranking is heuristic and best-effort. Callers should present candidates as
// suggestions and offer an external fallback.
package suggest

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/repolink"
)

// Kind identifies a candidate resource type.
type Kind string

const (
	KindFile   Kind = "file"
	KindSymbol Kind = "symbol"
	KindRepo   Kind = "repo"
)

// Candidate is a ranked source-link target.
type Candidate struct {
	Kind           Kind
	RepositoryID   string
	RepositoryName string
	RemoteURL      string
	Root           string
	Path           string
	Symbol         string
	QualifiedName  string
	NodeType       string
	Language       string
	Score          float64
}

// Subject describes the element being linked, without importing the workspace
// model.
type Subject struct {
	Ref          string
	Name         string
	Kind         string
	Technology   string
	RepositoryID string
	Repo         string
	FilePath     string
}

// Options tunes candidate generation.
type Options struct {
	// Limit caps the number of candidates returned per call (default 5).
	Limit int
	// RepoID pins the search to a single codeindex repository.
	RepoID string
	// MaxFactsPerRepo bounds how many facts are scanned per repository.
	MaxFactsPerRepo int
	// Kinds restricts results to these candidate kinds. Empty means all.
	Kinds []Kind
}

const (
	defaultLimit        = 5
	defaultMaxFactsRepo = 20000
	factPageSize        = 1000
)

// Store is the subset of the codeindex store the suggester reads.
type Store interface {
	ListRepositories(ctx context.Context) ([]*pb.Repository, error)
	Latest(ctx context.Context, repositoryID string) (string, error)
	Snapshot(ctx context.Context, id string) (*pb.Snapshot, error)
	Facts(ctx context.Context, snapshotID string, kind pb.FactKind, pathPrefix, after string, limit int) ([]*pb.CodeFact, error)
}

// Suggester holds loaded repository data so a link session can rank repeatedly
// without re-reading the database.
type Suggester struct {
	store   Store
	repos   []*pb.Repository
	sources map[string][]string
	facts   map[string][]*pb.CodeFact
}

// New creates a suggester backed by store.
func New(store Store) *Suggester {
	return &Suggester{
		store:   store,
		sources: map[string][]string{},
		facts:   map[string][]*pb.CodeFact{},
	}
}

// Refresh reloads the repository list.
func (s *Suggester) Refresh(ctx context.Context) error {
	repos, err := s.store.ListRepositories(ctx)
	if err != nil {
		return err
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].GetId() < repos[j].GetId() })
	s.repos = repos
	return nil
}

// Repositories returns the loaded repositories.
func (s *Suggester) Repositories() []*pb.Repository { return s.repos }

func (s *Suggester) repositories(ctx context.Context, opts Options) ([]*pb.Repository, error) {
	if s.repos == nil {
		if err := s.Refresh(ctx); err != nil {
			return nil, err
		}
	}
	if opts.RepoID == "" {
		return s.repos, nil
	}
	for _, repo := range s.repos {
		if repo.GetId() == opts.RepoID {
			return []*pb.Repository{repo}, nil
		}
	}
	return nil, fmt.Errorf("repository %q is not indexed", opts.RepoID)
}

func (s *Suggester) sourcesFor(ctx context.Context, repo *pb.Repository) ([]string, error) {
	if cached, ok := s.sources[repo.GetId()]; ok {
		return cached, nil
	}
	if repo.GetLatestSnapshotId() == "" {
		s.sources[repo.GetId()] = nil
		return nil, nil
	}
	snapshot, err := s.store.Snapshot(ctx, repo.GetLatestSnapshotId())
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(snapshot.GetSources()))
	for _, source := range snapshot.GetSources() {
		if path := strings.TrimSpace(source.GetPath()); path != "" {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	s.sources[repo.GetId()] = paths
	return paths, nil
}

func (s *Suggester) factsFor(ctx context.Context, repo *pb.Repository, maxFacts int) ([]*pb.CodeFact, error) {
	if cached, ok := s.facts[repo.GetId()]; ok {
		return cached, nil
	}
	snapshotID := repo.GetLatestSnapshotId()
	if snapshotID == "" {
		s.facts[repo.GetId()] = nil
		return nil, nil
	}
	if maxFacts <= 0 {
		maxFacts = defaultMaxFactsRepo
	}
	after := ""
	facts := make([]*pb.CodeFact, 0)
	for len(facts) < maxFacts {
		page, err := s.store.Facts(ctx, snapshotID, pb.FactKind_FACT_KIND_UNSPECIFIED, "", after, factPageSize)
		if err != nil {
			return nil, err
		}
		facts = append(facts, page...)
		if len(page) < factPageSize {
			break
		}
		after = page[len(page)-1].GetId()
	}
	if len(facts) > maxFacts {
		facts = facts[:maxFacts]
	}
	s.facts[repo.GetId()] = facts
	return facts, nil
}

// ForSubject ranks codeindex primitives for an element. Files, symbols, and
// indexed repositories are considered. The result is sorted by descending
// score.
func (s *Suggester) ForSubject(ctx context.Context, subject Subject, opts Options) ([]Candidate, error) {
	opts = withDefaults(opts)
	repos, err := s.repositories(ctx, opts)
	if err != nil {
		return nil, err
	}
	subjectTokens := tokenize(strings.Join([]string{subject.Name, subject.Ref, subject.Kind, subject.Technology}, " "))
	var candidates []Candidate
	for _, repo := range repos {
		if opts.RepoID == "" && subject.RepositoryID != "" && repo.GetId() != subject.RepositoryID {
			continue
		}
		repoMeta := repoInfo(repo)
		if allows(opts, KindRepo) {
			candidates = append(candidates, Candidate{
				Kind:           KindRepo,
				RepositoryID:   repo.GetId(),
				RepositoryName: repoMeta.name,
				RemoteURL:      repo.GetRemoteUrl(),
				Root:           repo.GetRoot(),
				Score:          rankRepoTokens(subjectTokens, repoMeta),
			})
		}
		if allows(opts, KindFile) {
			paths, err := s.sourcesFor(ctx, repo)
			if err != nil {
				return nil, err
			}
			for _, path := range paths {
				candidates = append(candidates, Candidate{
					Kind:           KindFile,
					RepositoryID:   repo.GetId(),
					RepositoryName: repoMeta.name,
					RemoteURL:      repo.GetRemoteUrl(),
					Root:           repo.GetRoot(),
					Path:           path,
					Score:          rankPath(subject, subjectTokens, path),
				})
			}
		}
		if allows(opts, KindSymbol) {
			facts, err := s.factsFor(ctx, repo, opts.MaxFactsPerRepo)
			if err != nil {
				return nil, err
			}
			for _, fact := range facts {
				if !isDeclaration(fact.GetKind()) {
					continue
				}
				path := fact.GetAnchor().GetPath()
				candidates = append(candidates, Candidate{
					Kind:           KindSymbol,
					RepositoryID:   repo.GetId(),
					RepositoryName: repoMeta.name,
					RemoteURL:      repo.GetRemoteUrl(),
					Root:           repo.GetRoot(),
					Path:           path,
					Symbol:         fact.GetName(),
					QualifiedName:  fact.GetQualifiedName(),
					NodeType:       nodeTypeForKind(fact.GetKind()),
					Language:       fact.GetLanguage(),
					Score:          rankSymbol(subject, subjectTokens, fact),
				})
			}
		}
	}
	sortCandidates(candidates)
	if len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}
	return candidates, nil
}

// SearchFiles ranks indexed files matching an explicit query (a path or file
// name).
func (s *Suggester) SearchFiles(ctx context.Context, query string, opts Options) ([]Candidate, error) {
	opts = withDefaults(opts)
	opts.Kinds = []Kind{KindFile}
	repos, err := s.repositories(ctx, opts)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	var candidates []Candidate
	for _, repo := range repos {
		repoMeta := repoInfo(repo)
		paths, err := s.sourcesFor(ctx, repo)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			score, ok := matchPath(needle, path)
			if !ok {
				continue
			}
			candidates = append(candidates, Candidate{
				Kind:           KindFile,
				RepositoryID:   repo.GetId(),
				RepositoryName: repoMeta.name,
				RemoteURL:      repo.GetRemoteUrl(),
				Root:           repo.GetRoot(),
				Path:           path,
				Score:          score,
			})
		}
		for _, dir := range directorySet(paths) {
			score, ok := matchPath(needle, strings.TrimSuffix(dir, "/"))
			if !ok {
				continue
			}
			candidates = append(candidates, Candidate{
				Kind:           KindFile,
				RepositoryID:   repo.GetId(),
				RepositoryName: repoMeta.name,
				RemoteURL:      repo.GetRemoteUrl(),
				Root:           repo.GetRoot(),
				Path:           dir,
				Score:          score * 0.95,
			})
		}
	}
	sortCandidates(candidates)
	if len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}
	return candidates, nil
}

// SearchSymbols ranks declarations matching an explicit symbol query.
func (s *Suggester) SearchSymbols(ctx context.Context, query string, opts Options) ([]Candidate, error) {
	opts = withDefaults(opts)
	opts.Kinds = []Kind{KindSymbol}
	repos, err := s.repositories(ctx, opts)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	var candidates []Candidate
	for _, repo := range repos {
		repoMeta := repoInfo(repo)
		facts, err := s.factsFor(ctx, repo, opts.MaxFactsPerRepo)
		if err != nil {
			return nil, err
		}
		for _, fact := range facts {
			if !isDeclaration(fact.GetKind()) {
				continue
			}
			score, ok := matchSymbol(needle, fact)
			if !ok {
				continue
			}
			candidates = append(candidates, Candidate{
				Kind:           KindSymbol,
				RepositoryID:   repo.GetId(),
				RepositoryName: repoMeta.name,
				RemoteURL:      repo.GetRemoteUrl(),
				Root:           repo.GetRoot(),
				Path:           fact.GetAnchor().GetPath(),
				Symbol:         fact.GetName(),
				QualifiedName:  fact.GetQualifiedName(),
				NodeType:       nodeTypeForKind(fact.GetKind()),
				Language:       fact.GetLanguage(),
				Score:          score,
			})
		}
	}
	sortCandidates(candidates)
	if len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}
	return candidates, nil
}

// SearchRepositories ranks indexed repositories matching an explicit query
// (remote URL, name, or id).
func (s *Suggester) SearchRepositories(ctx context.Context, query string, opts Options) ([]Candidate, error) {
	opts = withDefaults(opts)
	opts.Kinds = []Kind{KindRepo}
	repos, err := s.repositories(ctx, opts)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(strings.TrimSpace(query))
	var candidates []Candidate
	for _, repo := range repos {
		meta := repoInfo(repo)
		score, ok := matchRepo(needle, repo, meta)
		if !ok {
			continue
		}
		candidates = append(candidates, Candidate{
			Kind:           KindRepo,
			RepositoryID:   repo.GetId(),
			RepositoryName: meta.name,
			RemoteURL:      repo.GetRemoteUrl(),
			Root:           repo.GetRoot(),
			Score:          score,
		})
	}
	sortCandidates(candidates)
	if len(candidates) > opts.Limit {
		candidates = candidates[:opts.Limit]
	}
	return candidates, nil
}

// Verify reports whether a candidate still exists in the codeindex. A file is
// verified against the snapshot source manifest; a symbol against the facts of
// its file. Repository candidates are verified by presence.
func (s *Suggester) Verify(ctx context.Context, candidate Candidate) bool {
	if candidate.RepositoryID == "" {
		return false
	}
	if s.repos == nil {
		if err := s.Refresh(ctx); err != nil {
			return false
		}
	}
	repo := s.repoByID(candidate.RepositoryID)
	if repo == nil {
		return false
	}
	switch candidate.Kind {
	case KindRepo:
		return true
	case KindFile:
		paths, err := s.sourcesFor(ctx, repo)
		if err != nil {
			return false
		}
		folder := strings.HasSuffix(candidate.Path, "/")
		for _, path := range paths {
			if path == candidate.Path {
				return true
			}
			if folder && strings.HasPrefix(path, candidate.Path) {
				return true
			}
		}
		return false
	case KindSymbol:
		if !s.Verify(ctx, Candidate{Kind: KindFile, RepositoryID: candidate.RepositoryID, Path: candidate.Path}) {
			return false
		}
		facts, err := s.factsFor(ctx, repo, defaultMaxFactsRepo)
		if err != nil {
			return false
		}
		for _, fact := range facts {
			if fact.GetAnchor().GetPath() != candidate.Path {
				continue
			}
			if strings.EqualFold(fact.GetName(), candidate.Symbol) {
				return true
			}
			if candidate.QualifiedName != "" && strings.EqualFold(fact.GetQualifiedName(), candidate.QualifiedName) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (s *Suggester) repoByID(id string) *pb.Repository {
	for _, repo := range s.repos {
		if repo.GetId() == id {
			return repo
		}
	}
	return nil
}

func withDefaults(opts Options) Options {
	if opts.Limit <= 0 {
		opts.Limit = defaultLimit
	}
	if opts.MaxFactsPerRepo <= 0 {
		opts.MaxFactsPerRepo = defaultMaxFactsRepo
	}
	return opts
}

func allows(opts Options, kind Kind) bool {
	if len(opts.Kinds) == 0 {
		return true
	}
	for _, allowed := range opts.Kinds {
		if allowed == kind {
			return true
		}
	}
	return false
}

func sortCandidates(candidates []Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		if candidates[i].RepositoryID != candidates[j].RepositoryID {
			return candidates[i].RepositoryID < candidates[j].RepositoryID
		}
		if candidates[i].Path != candidates[j].Path {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Symbol < candidates[j].Symbol
	})
}

type repoMetadata struct {
	name   string
	tokens []string
}

func repoInfo(repo *pb.Repository) repoMetadata {
	name := repositoryDisplayName(repo)
	tokens := tokenize(strings.Join([]string{
		name,
		repo.GetRemoteUrl(),
		repo.GetRoot(),
		repo.GetId(),
	}, " "))
	return repoMetadata{name: name, tokens: tokens}
}

// repositoryDisplayName mirrors the server: prefer the explicit name, then the
// remote URL's last segment, then the checkout basename.
func repositoryDisplayName(repo *pb.Repository) string {
	if name := strings.TrimSpace(repo.GetName()); name != "" {
		return name
	}
	if remote := repo.GetRemoteUrl(); remote != "" {
		if parsed, err := url.Parse(remote); err == nil {
			if base := path.Base(strings.TrimSuffix(parsed.Path, "/")); base != "" && base != "." && base != "/" {
				return base
			}
		}
		if key := repolink.RemoteKey(remote); key != "" {
			return key
		}
	}
	if repo.GetRoot() != "" {
		return filepath.Base(repo.GetRoot())
	}
	return repo.GetId()
}

func rankRepoTokens(subjectTokens []string, meta repoMetadata) float64 {
	return scoreTokens(subjectTokens, meta.tokens) * 0.7
}

func rankPath(subject Subject, subjectTokens []string, path string) float64 {
	base := filepath.Base(path)
	baseNoExt := strings.TrimSuffix(base, filepath.Ext(base))
	tokens := tokenize(strings.Join([]string{base, baseNoExt, path}, " "))
	score := scoreTokens(subjectTokens, tokens)
	if subject.Name != "" && strings.EqualFold(baseNoExt, subject.Name) {
		score += 0.4
	}
	if subject.Kind != "" && strings.EqualFold(strings.TrimSuffix(filepath.Ext(base), "."), subject.Kind) {
		score += 0.05
	}
	return clampScore(score)
}

func rankSymbol(subject Subject, subjectTokens []string, fact *pb.CodeFact) float64 {
	tokens := tokenize(strings.Join([]string{
		fact.GetName(),
		fact.GetQualifiedName(),
		fact.GetAnchor().GetPath(),
	}, " "))
	score := scoreTokens(subjectTokens, tokens)
	if subject.Name != "" && strings.EqualFold(fact.GetName(), subject.Name) {
		score += 0.4
	}
	return clampScore(score)
}

// directorySet returns every ancestor directory (with trailing slash) of the
// given file paths, sorted.
func directorySet(paths []string) []string {
	set := map[string]bool{}
	for _, path := range paths {
		dir := path
		for {
			idx := strings.LastIndexByte(dir, '/')
			if idx <= 0 {
				break
			}
			dir = dir[:idx]
			set[dir+"/"] = true
		}
	}
	out := make([]string, 0, len(set))
	for dir := range set {
		out = append(out, dir)
	}
	sort.Strings(out)
	return out
}

func matchPath(needle, path string) (float64, bool) {
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(path))
	switch {
	case needle == "":
		return 0, false
	case lower == needle:
		return 1.0, true
	case base == needle:
		return 0.95, true
	case strings.HasSuffix(lower, "/"+needle):
		return 0.9, true
	case strings.Contains(lower, needle):
		return 0.6, true
	default:
		return 0, false
	}
}

func matchSymbol(needle string, fact *pb.CodeFact) (float64, bool) {
	name := strings.ToLower(fact.GetName())
	qualified := strings.ToLower(fact.GetQualifiedName())
	switch {
	case needle == "":
		return 0, false
	case name == needle:
		return 1.0, true
	case qualified == needle:
		return 0.95, true
	case strings.HasSuffix(qualified, "."+needle) || strings.HasSuffix(qualified, "#"+needle):
		return 0.85, true
	case strings.Contains(name, needle):
		return 0.55, true
	case strings.Contains(qualified, needle):
		return 0.45, true
	default:
		return 0, false
	}
}

func matchRepo(needle string, repo *pb.Repository, meta repoMetadata) (float64, bool) {
	switch {
	case needle == "":
		return 0, false
	case strings.ToLower(repo.GetId()) == needle:
		return 1.0, true
	case repolink.RemoteKey(repo.GetRemoteUrl()) == needle:
		return 0.95, true
	case strings.ToLower(meta.name) == needle:
		return 0.9, true
	case strings.Contains(strings.ToLower(meta.name), needle):
		return 0.6, true
	case strings.Contains(strings.ToLower(repo.GetRemoteUrl()), needle):
		return 0.5, true
	default:
		return 0, false
	}
}

func isDeclaration(kind pb.FactKind) bool {
	switch kind {
	case pb.FactKind_FACT_KIND_FUNCTION,
		pb.FactKind_FACT_KIND_METHOD,
		pb.FactKind_FACT_KIND_CLASS,
		pb.FactKind_FACT_KIND_STRUCT,
		pb.FactKind_FACT_KIND_INTERFACE,
		pb.FactKind_FACT_KIND_ENUM,
		pb.FactKind_FACT_KIND_TYPE,
		pb.FactKind_FACT_KIND_CONSTRUCTOR:
		return true
	default:
		return false
	}
}

func nodeTypeForKind(kind pb.FactKind) string {
	switch kind {
	case pb.FactKind_FACT_KIND_FUNCTION:
		return "function"
	case pb.FactKind_FACT_KIND_METHOD:
		return "method"
	case pb.FactKind_FACT_KIND_CLASS:
		return "class"
	case pb.FactKind_FACT_KIND_STRUCT:
		return "struct"
	case pb.FactKind_FACT_KIND_INTERFACE:
		return "interface"
	case pb.FactKind_FACT_KIND_ENUM:
		return "enum"
	case pb.FactKind_FACT_KIND_TYPE:
		return "type"
	case pb.FactKind_FACT_KIND_CONSTRUCTOR:
		return "constructor"
	default:
		return "symbol"
	}
}

func clampScore(score float64) float64 {
	switch {
	case score < 0:
		return 0
	case score > 1:
		return 1
	default:
		return score
	}
}

// scoreTokens matches subject tokens against candidate tokens, allowing prefix
// matches, normalized to roughly 0..1.
func scoreTokens(subjectTokens, candidateTokens []string) float64 {
	if len(subjectTokens) == 0 || len(candidateTokens) == 0 {
		return 0
	}
	var sum float64
	for _, subjectToken := range subjectTokens {
		best := 0.0
		for _, candidateToken := range candidateTokens {
			switch {
			case candidateToken == subjectToken:
				best = 1
			case strings.HasPrefix(candidateToken, subjectToken), strings.HasPrefix(subjectToken, candidateToken):
				if 0.6 > best {
					best = 0.6
				}
			case strings.Contains(candidateToken, subjectToken):
				if 0.4 > best {
					best = 0.4
				}
			}
			if best == 1 {
				break
			}
		}
		sum += best
	}
	return sum / float64(len(subjectTokens))
}

var tokenStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "of": true,
	"to": true, "a": true, "an": true, "in": true, "on": true,
}

func tokenize(value string) []string {
	var tokens []string
	seen := map[string]bool{}
	for _, raw := range splitNonAlnum(value) {
		for _, token := range splitCamel(raw) {
			token = strings.ToLower(token)
			if len(token) < 2 || tokenStopwords[token] || seen[token] {
				continue
			}
			seen[token] = true
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func splitNonAlnum(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
}

func splitCamel(value string) []string {
	if value == "" {
		return nil
	}
	var out []string
	start := 0
	runes := []rune(value)
	for i := 1; i < len(runes); i++ {
		prev, cur := runes[i-1], runes[i]
		if isLower(prev) && isUpper(cur) {
			out = append(out, string(runes[start:i]))
			start = i
		}
	}
	out = append(out, string(runes[start:]))
	return out
}

func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
