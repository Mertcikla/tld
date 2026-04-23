// WASM grammar module for Python source files.
// Compiled with: GOOS=wasip1 GOARCH=wasm go build -o ../../python.wasm .
//
// Protocol:
//
//	stdin  Python source code
//	stdout JSON: {"symbols":[...],"refs":[...]}
//
// Uses a proper tokenizer (no regexp) to identify def/class declarations and call sites.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"
)

type Symbol struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Line int    `json:"line"`
}

type Ref struct {
	Name string `json:"name"`
	Line int    `json:"line"`
}

type Result struct {
	Symbols []Symbol `json:"symbols"`
	Refs    []Ref    `json:"refs"`
}

// tokenKind enumerates Python lexical kinds (simplified).
type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokComment
	tokNewline
	tokIndent
	tokPunct
)

type token struct {
	kind tokenKind
	text string
}

// tokenizeLine tokenizes a single logical line of Python code (no newlines inside strings handled here).
func tokenizeLine(line string) []token {
	var toks []token
	i := 0
	n := len(line)
	for i < n {
		ch := rune(line[i])
		switch {
		case ch == '#':
			// Rest is comment
			toks = append(toks, token{tokComment, line[i:]})
			return toks
		case ch == '"' || ch == '\'':
			// String literal find matching close quote, skipping escapes
			quote := ch
			tripleQ := i+2 < n && rune(line[i+1]) == quote && rune(line[i+2]) == quote
			if tripleQ {
				// Triple-quoted: consume through end of string (may span lines simplified: treat as opaque token)
				end := strings.Index(line[i+3:], string([]rune{quote, quote, quote}))
				if end >= 0 {
					toks = append(toks, token{tokString, line[i : i+3+end+3]})
					i += 3 + end + 3
				} else {
					// Unterminated triple-quote: consume rest
					toks = append(toks, token{tokString, line[i:]})
					return toks
				}
			} else {
				j := i + 1
				for j < n {
					if line[j] == '\\' {
						j += 2
						continue
					}
					if rune(line[j]) == quote {
						j++
						break
					}
					j++
				}
				toks = append(toks, token{tokString, line[i:j]})
				i = j
			}
		case unicode.IsSpace(ch):
			i++
		case ch == '_' || unicode.IsLetter(ch):
			j := i
			for j < n && (rune(line[j]) == '_' || unicode.IsLetter(rune(line[j])) || unicode.IsDigit(rune(line[j]))) {
				j++
			}
			toks = append(toks, token{tokIdent, line[i:j]})
			i = j
		default:
			toks = append(toks, token{tokPunct, string(ch)})
			i++
		}
	}
	return toks
}

func main() {
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
		os.Exit(1)
	}

	var result Result
	scanner := bufio.NewScanner(strings.NewReader(string(src)))
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		toks := tokenizeLine(line)

		if len(toks) == 0 {
			continue
		}

		// Look for: def NAME( or class NAME( or class NAME:
		for idx, t := range toks {
			if t.kind != tokIdent {
				continue
			}

			switch t.text {
			case "def":
				if idx+1 < len(toks) && toks[idx+1].kind == tokIdent {
					result.Symbols = append(result.Symbols, Symbol{
						Name: toks[idx+1].text,
						Kind: "function",
						Line: lineNum,
					})
				}
			case "class":
				if idx+1 < len(toks) && toks[idx+1].kind == tokIdent {
					result.Symbols = append(result.Symbols, Symbol{
						Name: toks[idx+1].text,
						Kind: "class",
						Line: lineNum,
					})
				}
			default:
				// Call site: IDENT followed by '('
				if idx+1 < len(toks) && toks[idx+1].kind == tokPunct && toks[idx+1].text == "(" {
					// Skip keywords that look like calls
					kwds := map[string]bool{
						"def": true, "class": true, "if": true, "elif": true,
						"while": true, "for": true, "with": true, "assert": true,
						"raise": true, "return": true, "yield": true, "lambda": true,
						"print": true,
					}
					if !kwds[t.text] {
						result.Refs = append(result.Refs, Ref{Name: t.text, Line: lineNum})
					}
				}
			}
		}
	}

	if result.Symbols == nil {
		result.Symbols = []Symbol{}
	}
	if result.Refs == nil {
		result.Refs = []Ref{}
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}
