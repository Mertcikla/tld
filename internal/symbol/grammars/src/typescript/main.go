// WASM grammar module for TypeScript/JavaScript source files.
// Compiled with: GOOS=wasip1 GOARCH=wasm go build -o ../../typescript.wasm .
//
// Protocol:
//
//	stdin  TypeScript/JavaScript source code
//	stdout JSON: {"symbols":[...],"refs":[...]}
//	exit 1 parse error written to stderr
//
// Uses a proper tokenizer (no regexp) to identify declarations and call sites.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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

// tokenKind enumerates lexical token kinds.
type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokTemplate
	tokLineComment
	tokBlockComment
	tokPunct // single char punctuation / operators
	tokNewline
)

type token struct {
	kind tokenKind
	text string
	line int
}

// tokenize produces a flat stream of tokens from src.
func tokenize(src []byte) []token {
	var tokens []token
	i := 0
	line := 1
	n := len(src)

	for i < n {
		ch := src[i]
		switch {
		case ch == '\n':
			tokens = append(tokens, token{tokNewline, "\n", line})
			line++
			i++
		case ch == '\r':
			i++
		case ch == ' ' || ch == '\t':
			i++
		case ch == '/' && i+1 < n && src[i+1] == '/':
			// Line comment
			start := i
			for i < n && src[i] != '\n' {
				i++
			}
			tokens = append(tokens, token{tokLineComment, string(src[start:i]), line})
		case ch == '/' && i+1 < n && src[i+1] == '*':
			// Block comment
			start := i
			startLine := line
			i += 2
			for i+1 < n && !(src[i] == '*' && src[i+1] == '/') {
				if src[i] == '\n' {
					line++
				}
				i++
			}
			i += 2
			tokens = append(tokens, token{tokBlockComment, string(src[start:i]), startLine})
		case ch == '"' || ch == '\'':
			// String literal
			quote := ch
			start := i
			i++
			for i < n && src[i] != quote {
				if src[i] == '\\' {
					i++ // skip escaped char
				}
				if src[i] == '\n' {
					line++
				}
				i++
			}
			i++ // closing quote
			tokens = append(tokens, token{tokString, string(src[start:i]), line})
		case ch == '`':
			// Template literal (skip contents)
			start := i
			startLine := line
			i++
			depth := 0
			for i < n {
				c := src[i]
				if c == '`' && depth == 0 {
					i++
					break
				}
				if c == '$' && i+1 < n && src[i+1] == '{' {
					depth++
					i += 2
					continue
				}
				if c == '}' && depth > 0 {
					depth--
					i++
					continue
				}
				if c == '\n' {
					line++
				}
				i++
			}
			tokens = append(tokens, token{tokTemplate, string(src[start:i]), startLine})
		case isIdentStart(rune(ch)):
			start := i
			for i < n && isIdentPart(rune(src[i])) {
				i++
			}
			tokens = append(tokens, token{tokIdent, string(src[start:i]), line})
		default:
			tokens = append(tokens, token{tokPunct, string([]byte{ch}), line})
			i++
		}
	}
	tokens = append(tokens, token{tokEOF, "", line})
	return tokens
}

func isIdentStart(r rune) bool { return r == '_' || r == '$' || unicode.IsLetter(r) }
func isIdentPart(r rune) bool {
	return r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// parser walks the token stream to collect symbols and call-site refs.
type tsParser struct {
	toks []token
	pos  int
}

func (p *tsParser) peek(offset int) token {
	idx := p.pos + offset
	if idx >= len(p.toks) {
		return token{kind: tokEOF}
	}
	return p.toks[idx]
}

// skipNonCode advances past newlines and comment tokens, returns first code token.
func (p *tsParser) cur() token {
	for p.pos < len(p.toks) {
		t := p.toks[p.pos]
		if t.kind != tokNewline && t.kind != tokLineComment && t.kind != tokBlockComment {
			return t
		}
		p.pos++
	}
	return token{kind: tokEOF}
}

func (p *tsParser) advance() token {
	t := p.cur()
	p.pos++
	return t
}

func (p *tsParser) parse() ([]Symbol, []Ref) {
	var syms []Symbol
	var refs []Ref

	for {
		t := p.cur()
		if t.kind == tokEOF {
			break
		}

		// Collect call-site refs: IDENT followed by '(' that isn't a declaration keyword
		if t.kind == tokIdent {
			declKws := map[string]bool{
				"function": true, "class": true, "if": true, "for": true,
				"while": true, "switch": true, "catch": true, "new": true,
			}
			next := p.peekCode(1)
			if next.kind == tokPunct && next.text == "(" && !declKws[t.text] {
				refs = append(refs, Ref{Name: t.text, Line: t.line})
			}
		}

		// function NAME(...) or async function NAME(...)
		if t.kind == tokIdent && (t.text == "function" || t.text == "async") {
			start := p.pos
			p.pos++
			cur := p.cur()
			if t.text == "async" && cur.kind == tokIdent && cur.text == "function" {
				p.pos++
				cur = p.cur()
			}
			if cur.kind == tokIdent {
				syms = append(syms, Symbol{Name: cur.text, Kind: "function", Line: cur.line})
				p.pos++
				continue
			}
			p.pos = start + 1
			continue
		}

		// class NAME
		if t.kind == tokIdent && t.text == "class" {
			p.pos++
			cur := p.cur()
			if cur.kind == tokIdent {
				syms = append(syms, Symbol{Name: cur.text, Kind: "class", Line: cur.line})
				p.pos++
				continue
			}
			continue
		}

		// const/let/var NAME = [async] function | NAME = [async] (...) =>
		if t.kind == tokIdent && (t.text == "const" || t.text == "let" || t.text == "var") {
			p.pos++
			nameT := p.cur()
			if nameT.kind != tokIdent {
				continue
			}
			p.pos++
			eq := p.cur()
			if eq.kind == tokPunct && eq.text == "=" {
				p.pos++
				// optional async
				asyncT := p.cur()
				if asyncT.kind == tokIdent && asyncT.text == "async" {
					p.pos++
				}
				afterAsync := p.cur()
				if afterAsync.kind == tokIdent && afterAsync.text == "function" {
					syms = append(syms, Symbol{Name: nameT.text, Kind: "function", Line: nameT.line})
					p.pos++
					continue
				}
				// Arrow function: ( or IDENT =>
				if afterAsync.kind == tokPunct && afterAsync.text == "(" {
					// scan for => after matching paren
					depth := 0
					save := p.pos
					for p.pos < len(p.toks) {
						tt := p.toks[p.pos]
						if tt.kind == tokPunct && tt.text == "(" {
							depth++
						} else if tt.kind == tokPunct && tt.text == ")" {
							depth--
							if depth == 0 {
								p.pos++
								break
							}
						}
						p.pos++
					}
					arrow := p.cur()
					if arrow.kind == tokPunct && arrow.text == "=" {
						p.pos++
						gt := p.cur()
						if gt.kind == tokPunct && gt.text == ">" {
							syms = append(syms, Symbol{Name: nameT.text, Kind: "function", Line: nameT.line})
							p.pos++
							continue
						}
					}
					p.pos = save
					continue
				}
			}
			continue
		}

		p.pos++
	}
	return syms, refs
}

// peekCode peeks n non-comment, non-newline tokens ahead from current position.
func (p *tsParser) peekCode(n int) token {
	saved := p.pos
	p.advance() // consume current
	for i := 1; i < n; i++ {
		p.advance()
	}
	result := p.cur()
	p.pos = saved
	return result
}

func main() {
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
		os.Exit(1)
	}

	toks := tokenize(src)
	tp := &tsParser{toks: toks}
	syms, refs := tp.parse()

	result := Result{
		Symbols: syms,
		Refs:    refs,
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
