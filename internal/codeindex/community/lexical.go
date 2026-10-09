package community

import (
	"math"
	"sort"
	"strings"
)

// lexicalStopwords are boilerplate tokens that appear across a repository and
// therefore never discriminate one group. It mirrors the mapper's stopword set
// plus file-format tokens, so a group of .tsx files is never named "tsx".
var lexicalStopwords = map[string]struct{}{
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
	"tsx": {}, "jsx": {}, "css": {}, "scss": {}, "less": {}, "html": {}, "htm": {}, "json": {},
	"yaml": {}, "yml": {}, "toml": {}, "svg": {}, "png": {}, "jpg": {}, "jpeg": {}, "gif": {},
	"ico": {}, "lock": {}, "sql": {}, "proto": {}, "md": {}, "markdown": {},
}

func isASCIIUpper(c byte) bool { return c >= 'A' && c <= 'Z' }
func isASCIILower(c byte) bool { return c >= 'a' && c <= 'z' }
func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }
func isASCIIAlnum(c byte) bool { return isASCIIUpper(c) || isASCIILower(c) || isASCIIDigit(c) }

// splitNonAlnum splits on non-alphanumeric characters.
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

// camelTokens splits one alphanumeric piece on camelCase/PascalCase, acronym
// and letter/digit boundaries.
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

// lexicalTokens splits one string into lowercase identifier tokens, dropping
// short fragments, digits and stopwords.
func lexicalTokens(text string) []string {
	tokens := make([]string, 0)
	for _, piece := range splitNonAlnum(text) {
		for _, token := range camelTokens(piece) {
			token = strings.ToLower(token)
			if len(token) < 3 || allDigits(token) {
				continue
			}
			if _, stop := lexicalStopwords[token]; stop {
				continue
			}
			tokens = append(tokens, token)
		}
	}
	return tokens
}

// lexicalIndex scores distinctive identifier tokens across the whole dataset
// so many groups can be named without re-tokenizing.
type lexicalIndex struct {
	docs      []map[string]int
	frequency map[string]int
	total     int
}

func newLexicalIndex(files []File) *lexicalIndex {
	index := &lexicalIndex{
		docs:      make([]map[string]int, len(files)),
		frequency: map[string]int{},
		total:     len(files),
	}
	for i, file := range files {
		weights := map[string]int{}
		for _, entry := range []struct {
			text   string
			weight int
		}{{file.DisplayName, 2}, {file.Path, 1}} {
			for _, token := range lexicalTokens(entry.text) {
				if entry.weight > weights[token] {
					weights[token] = entry.weight
				}
			}
		}
		index.docs[i] = weights
		for token := range weights {
			index.frequency[token]++
		}
	}
	return index
}

// name returns the most distinctive token shared by the members, or "" when
// none qualifies. Score is count * weight * log((N+1)/(document_frequency+1)),
// with ties broken towards the higher weight, then count, then token order.
func (index *lexicalIndex) name(members []int) string {
	if index == nil || index.total == 0 {
		return ""
	}
	counts := map[string]int{}
	weights := map[string]int{}
	for _, member := range members {
		if member < 0 || member >= len(index.docs) {
			continue
		}
		for token, weight := range index.docs[member] {
			counts[token]++
			if weight > weights[token] {
				weights[token] = weight
			}
		}
	}
	type scored struct {
		score  float64
		weight int
		count  int
		token  string
	}
	items := make([]scored, 0, len(counts))
	for token, count := range counts {
		frequency := index.frequency[token]
		if count < 2 || float64(count)/float64(max(frequency, 1)) < 0.5 {
			continue
		}
		weight := weights[token]
		items = append(items, scored{
			score:  float64(count) * float64(weight) * math.Log(float64(index.total+1)/float64(frequency+1)),
			weight: weight,
			count:  count,
			token:  token,
		})
	}
	if len(items) == 0 {
		return ""
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if left.score != right.score {
			return left.score > right.score
		}
		if left.weight != right.weight {
			return left.weight > right.weight
		}
		if left.count != right.count {
			return left.count > right.count
		}
		return left.token < right.token
	})
	return items[0].token
}
