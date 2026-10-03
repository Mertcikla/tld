package mapper

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// NameStopwords are language-agnostic boilerplate tokens: path roots, layer
// words and generic type nouns that show up across whole repositories and so
// never discriminate a cluster.
var NameStopwords = map[string]struct{}{
	"src": {}, "main": {}, "test": {}, "tests": {}, "spec": {}, "specs": {}, "lib": {}, "source": {},
	"package": {}, "packages": {}, "internal": {}, "app": {}, "apps": {}, "core": {}, "util": {},
	"utils": {}, "common": {}, "helper": {}, "helpers": {}, "index": {}, "init": {}, "base": {},
	"abstract": {}, "impl": {}, "implementation": {}, "manager": {}, "handler": {}, "interface": {},
	"service": {}, "services": {}, "model": {}, "models": {}, "view": {}, "views": {}, "controller": {},
	"controllers": {}, "data": {}, "types": {}, "type": {}, "node": {}, "constants": {}, "exception": {},
	"exceptions": {}, "error": {}, "errors": {}, "config": {}, "settings": {}, "option": {}, "options": {},
	"result": {}, "results": {}, "request": {}, "response": {}, "client": {}, "server": {}, "api": {},
	"module": {}, "modules": {}, "org": {}, "com": {}, "net": {}, "io": {}, "github": {}, "www": {},
	"py": {}, "js": {}, "ts": {}, "go": {}, "rb": {}, "rs": {}, "java": {}, "kt": {}, "swift": {},
	"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "this": {}, "that": {}, "into": {}, "out": {},
}

// NameOptions controls lexical domain naming.
type NameOptions struct {
	Top                  int
	MinCount             int
	MaxDocumentFrequency float64
	MinExclusivity       float64
}

// DefaultNameOptions mirrors the Python defaults.
func DefaultNameOptions() NameOptions {
	return NameOptions{Top: 1, MinCount: 2, MaxDocumentFrequency: 0.35, MinExclusivity: 0.5}
}

// Validate checks the option ranges.
func (o NameOptions) Validate() error {
	if o.Top < 1 {
		return fmt.Errorf("top must be a positive integer")
	}
	if o.MinCount < 1 {
		return fmt.Errorf("min_count must be a positive integer")
	}
	if o.MaxDocumentFrequency <= 0 || o.MaxDocumentFrequency > 1 {
		return fmt.Errorf("max_document_frequency must be in (0, 1]")
	}
	if o.MinExclusivity <= 0 || o.MinExclusivity > 1 {
		return fmt.Errorf("min_exclusivity must be in (0, 1]")
	}
	return nil
}

func isASCIIUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isASCIILower(c byte) bool { return c >= 'a' && c <= 'z' }
func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }
func isASCIIAlnum(c byte) bool { return isASCIIUpper(c) || isASCIILower(c) || isASCIIDigit(c) }

// splitNonAlnum mirrors re.split(r"[^A-Za-z0-9]+", text).
func splitNonAlnum(text string) []string {
	pieces := make([]string, 0)
	start := -1
	for i := 0; i < len(text); i++ {
		if isASCIIAlnum(text[i]) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			pieces = append(pieces, text[start:i])
			start = -1
		}
	}
	if start >= 0 {
		pieces = append(pieces, text[start:])
	}
	return pieces
}

// camelTokens splits one alphanumeric piece on camelCase/PascalCase, acronym and
// letter/digit boundaries, matching the Python regex
// [A-Z]+(?=[A-Z][a-z])|[A-Z]?[a-z]+|[A-Z]+|[0-9]+.
func camelTokens(piece string) []string {
	tokens := make([]string, 0)
	for i := 0; i < len(piece); {
		c := piece[i]
		switch {
		case isASCIIDigit(c):
			j := i
			for j < len(piece) && isASCIIDigit(piece[j]) {
				j++
			}
			tokens = append(tokens, piece[i:j])
			i = j
		case isASCIIUpper(c):
			j := i
			for j < len(piece) && isASCIIUpper(piece[j]) {
				j++
			}
			if j < len(piece) && isASCIILower(piece[j]) {
				if j-i > 1 {
					// Acronym run followed by a capitalized word: keep the last
					// uppercase for the word start.
					tokens = append(tokens, piece[i:j-1])
					i = j - 1
				} else {
					k := j
					for k < len(piece) && isASCIILower(piece[k]) {
						k++
					}
					tokens = append(tokens, piece[i:k])
					i = k
				}
			} else {
				tokens = append(tokens, piece[i:j])
				i = j
			}
		case isASCIILower(c):
			j := i
			for j < len(piece) && isASCIILower(piece[j]) {
				j++
			}
			tokens = append(tokens, piece[i:j])
			i = j
		default:
			i++
		}
	}
	return tokens
}

func allDigits(token string) bool {
	if token == "" {
		return false
	}
	for i := 0; i < len(token); i++ {
		if !isASCIIDigit(token[i]) {
			return false
		}
	}
	return true
}

// tokenize splits one string into lowercase identifier tokens, dropping short
// fragments, digits and stopwords.
func tokenize(text string) []string {
	tokens := make([]string, 0)
	for _, piece := range splitNonAlnum(text) {
		for _, token := range camelTokens(piece) {
			token = strings.ToLower(token)
			if len(token) < 3 || allDigits(token) {
				continue
			}
			if _, stop := NameStopwords[token]; stop {
				continue
			}
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// nameTokens maps each token in a fact to its weight, deduplicated within the
// fact. Display-name tokens weigh 2 and path tokens weigh 1.
func nameTokens(fact Fact) map[string]int {
	weights := map[string]int{}
	for _, entry := range []struct {
		text   string
		weight int
	}{{fact.DisplayName, 2}, {fact.Path, 1}} {
		for _, token := range tokenize(entry.text) {
			if entry.weight > weights[token] {
				weights[token] = entry.weight
			}
		}
	}
	return weights
}

// stopwordTokens returns tokens implied by a repository root path.
func stopwordTokens(root string) map[string]struct{} {
	segments := strings.FieldsFunc(root, func(r rune) bool { return r == '\\' || r == '/' })
	out := map[string]struct{}{}
	for _, token := range tokenize(strings.Join(segments, " ")) {
		out[token] = struct{}{}
	}
	return out
}

// folderFallback is the most common non-root source folder across members,
// breaking ties by the lexicographically smallest folder. It mirrors
// _domain_folder in the reference implementation.
func folderFallback(paths []string, members []int) string {
	counts := map[string]int{}
	for _, member := range members {
		if member < 0 || member >= len(paths) {
			continue
		}
		path := paths[member]
		folder := ""
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			folder = path[:idx]
		}
		if folder == "" || folder == "." {
			continue
		}
		counts[folder]++
	}
	if len(counts) == 0 {
		return ""
	}
	best := ""
	bestCount := -1
	for folder, count := range counts {
		if count > bestCount || (count == bestCount && folder < best) {
			best = folder
			bestCount = count
		}
	}
	return best
}

func domainFolder(dataset *Dataset, domain Domain) string {
	paths := make([]string, len(dataset.Facts))
	for i := range dataset.Facts {
		paths[i] = dataset.Facts[i].Path
	}
	return folderFallback(paths, domain.Members)
}

// NameIndex precomputes per-fact token weights and document frequencies so many
// member sets (clusters, bins, folders) can be named without re-tokenizing the
// dataset. Build one with NewNameIndex and call Name.
type NameIndex struct {
	docs              []map[string]int
	documentFrequency map[string]int
	paths             []string
	total             int
	opts              NameOptions
}

// NewNameIndex tokenizes every fact once and computes document frequencies,
// folding the repository root tokens into the ignore set.
func NewNameIndex(dataset *Dataset, opts NameOptions) (*NameIndex, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	ignored := make(map[string]struct{}, len(NameStopwords)+8)
	for token := range NameStopwords {
		ignored[token] = struct{}{}
	}
	if dataset.Root != nil && *dataset.Root != "" {
		for token := range stopwordTokens(*dataset.Root) {
			ignored[token] = struct{}{}
		}
	}
	docs := make([]map[string]int, len(dataset.Facts))
	paths := make([]string, len(dataset.Facts))
	documentFrequency := map[string]int{}
	for i := range dataset.Facts {
		paths[i] = dataset.Facts[i].Path
		weights := map[string]int{}
		for token, weight := range nameTokens(dataset.Facts[i]) {
			if _, skip := ignored[token]; skip {
				continue
			}
			weights[token] = weight
			documentFrequency[token]++
		}
		docs[i] = weights
	}
	return &NameIndex{
		docs:              docs,
		documentFrequency: documentFrequency,
		paths:             paths,
		total:             len(dataset.Facts),
		opts:              opts,
	}, nil
}

// Name infers a short, distinctive name for a set of member fact indices. It
// scores tokens by count * weight * log((N+1)/(document_frequency+1)) and joins
// the best Top distinct tokens. It returns "" when no token qualifies, leaving
// the fallback to the caller.
func (n *NameIndex) Name(members []int) string {
	counts := map[string]int{}
	symbols := map[string]int{}
	for _, member := range members {
		if member < 0 || member >= len(n.docs) {
			continue
		}
		for token, weight := range n.docs[member] {
			counts[token]++
			if weight > symbols[token] {
				symbols[token] = weight
			}
		}
	}
	type scored struct {
		score  float64
		weight int
		count  int
		token  string
	}
	ceiling := n.opts.MaxDocumentFrequency * float64(n.total)
	items := make([]scored, 0, len(counts))
	for token, count := range counts {
		frequency := n.documentFrequency[token]
		if count < n.opts.MinCount || float64(count)/float64(frequency) < n.opts.MinExclusivity {
			continue
		}
		// Ignore the frequency ceiling for a token whose occurrences all fall
		// inside this member set, so small but distinct groups still get named.
		if float64(frequency) > ceiling && frequency > count {
			continue
		}
		weight := symbols[token]
		score := float64(count) * float64(weight) * math.Log(float64(n.total+1)/float64(frequency+1))
		items = append(items, scored{score: score, weight: weight, count: count, token: token})
	}
	sort.SliceStable(items, func(a, b int) bool {
		x, y := items[a], items[b]
		if x.score != y.score {
			return x.score > y.score
		}
		if x.weight != y.weight {
			return x.weight > y.weight
		}
		if x.count != y.count {
			return x.count > y.count
		}
		return x.token < y.token
	})
	limit := n.opts.Top
	if limit > len(items) {
		limit = len(items)
	}
	if limit == 0 {
		return ""
	}
	chosen := make([]string, 0, limit)
	for _, item := range items[:limit] {
		chosen = append(chosen, item.token)
	}
	return strings.Join(chosen, " ")
}

// FallbackName is the most common non-root source folder across members, or ""
// when every member sits at the repository root.
func (n *NameIndex) FallbackName(members []int) string {
	return folderFallback(n.paths, members)
}

// NameDomains infers a short, distinctive name for each domain from identifier
// tokens. It is a faithful port of dilute/research/utils.py:name_domains.
//
// A token is eligible when it is frequent enough inside the cluster
// (count >= MinCount) and mostly exclusive to it
// (count/document_frequency >= MinExclusivity), with a document-frequency
// ceiling of max(MaxDocumentFrequency*N, count). Eligible tokens score as
// count * weight * log((N+1)/(document_frequency+1)) and rank by score, weight,
// count, then alphabetically; the best Top distinct tokens are joined. Domains
// with no eligible token fall back to their most common non-root source folder
// or "cluster <rank>". The returned map has an entry for every domain index.
func NameDomains(dataset *Dataset, domains []Domain, opts NameOptions) (map[int]string, error) {
	index, err := NewNameIndex(dataset, opts)
	if err != nil {
		return nil, err
	}
	names := make(map[int]string, len(domains))
	for i, domain := range domains {
		if name := index.Name(domain.Members); name != "" {
			names[i] = name
			continue
		}
		if folder := domainFolder(dataset, domain); folder != "" {
			names[i] = folder
		} else {
			names[i] = fmt.Sprintf("cluster %d", i+1)
		}
	}
	return names, nil
}
