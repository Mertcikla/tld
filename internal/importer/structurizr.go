package importer

import (
	"fmt"
	"strings"
	"unicode"
)

// ---- Tokenizer ----

type tokenKind int

const (
	tokEOF     tokenKind = iota
	tokIdent             // bare word, !directive
	tokString            // "quoted string"
	tokLBrace            // {
	tokRBrace            // }
	tokEquals            // =
	tokArrow             // -> or --xxx->
	tokNewline           // logical statement separator
)

type dslToken struct {
	kind  tokenKind
	value string
}

func joinLineContinuations(s string) string {
	var sb strings.Builder
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\\' {
			j := i + 1
			for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t') {
				j++
			}
			if j < len(runes) && runes[j] == '\n' {
				i = j
				for i+1 < len(runes) && (runes[i+1] == ' ' || runes[i+1] == '\t') {
					i++
				}
				sb.WriteRune(' ')
				continue
			}
		}
		sb.WriteRune(runes[i])
	}
	return sb.String()
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.'
}

func dslRuneAt(runes []rune, i int) rune {
	if i < len(runes) {
		return runes[i]
	}
	return 0
}

func skipLineComment(runes []rune, pos int) int {
	for pos < len(runes) && runes[pos] != '\n' {
		pos++
	}
	return pos
}

func skipBlockComment(runes []rune, pos int) int {
	pos += 2 // consume /*
	for pos < len(runes) {
		if runes[pos] == '*' && dslRuneAt(runes, pos+1) == '/' {
			return pos + 2
		}
		pos++
	}
	return pos
}

func scanQuotedString(runes []rune, pos int) (string, int) {
	var sb strings.Builder
	if dslRuneAt(runes, pos) == '"' && dslRuneAt(runes, pos+1) == '"' {
		pos += 2
		for pos < len(runes) {
			if runes[pos] == '"' && dslRuneAt(runes, pos+1) == '"' && dslRuneAt(runes, pos+2) == '"' {
				return sb.String(), pos + 3
			}
			sb.WriteRune(runes[pos])
			pos++
		}
		return sb.String(), pos
	}
	for pos < len(runes) {
		c := runes[pos]
		if c == '\\' && dslRuneAt(runes, pos+1) == '"' {
			sb.WriteRune('"')
			pos += 2
			continue
		}
		if c == '"' {
			return sb.String(), pos + 1
		}
		sb.WriteRune(c)
		pos++
	}
	return sb.String(), pos
}

func scanArrow(runes []rune, pos int) (bool, int) {
	if dslRuneAt(runes, pos+1) == '>' {
		return true, pos + 2
	}
	if dslRuneAt(runes, pos+1) == '-' {
		pos += 2
		for pos < len(runes) {
			if runes[pos] == '-' && dslRuneAt(runes, pos+1) == '>' {
				return true, pos + 2
			}
			pos++
		}
		return true, pos
	}
	return false, pos + 1
}

func scanBangIdent(runes []rune, pos int) (string, int) {
	var sb strings.Builder
	sb.WriteRune('!')
	for pos < len(runes) && isIdentRune(runes[pos]) {
		sb.WriteRune(runes[pos])
		pos++
	}
	return sb.String(), pos
}

func scanIdent(runes []rune, pos int) (string, int) {
	var sb strings.Builder
	for pos < len(runes) && isIdentRune(runes[pos]) {
		sb.WriteRune(runes[pos])
		pos++
	}
	return sb.String(), pos
}

func tokenizeDSL(raw string) []dslToken {
	raw = joinLineContinuations(raw)
	runes := []rune(raw)
	pos := 0
	var tokens []dslToken
	lastWasNewline := true

	emitNewline := func() {
		if !lastWasNewline {
			tokens = append(tokens, dslToken{kind: tokNewline})
			lastWasNewline = true
		}
	}

	for pos < len(runes) {
		r := runes[pos]

		if r == ' ' || r == '\t' || r == '\r' {
			pos++
			continue
		}
		if r == '\n' {
			pos++
			emitNewline()
			continue
		}
		if (r == '/' && dslRuneAt(runes, pos+1) == '/') || r == '#' {
			pos = skipLineComment(runes, pos)
			continue
		}
		if r == '/' && dslRuneAt(runes, pos+1) == '*' {
			pos = skipBlockComment(runes, pos)
			continue
		}
		if r == '"' {
			val, newPos := scanQuotedString(runes, pos+1)
			tokens = append(tokens, dslToken{kind: tokString, value: val})
			lastWasNewline = false
			pos = newPos
			continue
		}
		if r == '{' {
			tokens = append(tokens, dslToken{kind: tokLBrace})
			lastWasNewline = false
			pos++
			continue
		}
		if r == '}' {
			tokens = append(tokens, dslToken{kind: tokRBrace})
			lastWasNewline = false
			pos++
			continue
		}
		if r == '=' {
			tokens = append(tokens, dslToken{kind: tokEquals})
			lastWasNewline = false
			pos++
			continue
		}
		if r == '-' {
			isArrow, newPos := scanArrow(runes, pos)
			if isArrow {
				tokens = append(tokens, dslToken{kind: tokArrow})
				lastWasNewline = false
			}
			pos = newPos
			continue
		}
		if r == '!' {
			val, newPos := scanBangIdent(runes, pos+1)
			if len(val) > 1 {
				tokens = append(tokens, dslToken{kind: tokIdent, value: val})
				lastWasNewline = false
			}
			pos = newPos
			continue
		}
		if isIdentStart(r) {
			val, newPos := scanIdent(runes, pos)
			tokens = append(tokens, dslToken{kind: tokIdent, value: val})
			lastWasNewline = false
			pos = newPos
			continue
		}
		pos++
	}

	tokens = append(tokens, dslToken{kind: tokEOF})
	return tokens
}

// ---- Parser ----

type dslParser struct {
	tokens      []dslToken
	pos         int
	identMode   string
	scopeStack  []string
	registry    map[string]*ParsedElement
	anonCounter int
	workspace   *ParsedWorkspace
}

func (p *dslParser) peek() dslToken {
	return p.peekAt(0)
}

func (p *dslParser) peekAt(offset int) dslToken {
	i := p.pos + offset
	if i >= len(p.tokens) {
		return dslToken{kind: tokEOF}
	}
	return p.tokens[i]
}

func (p *dslParser) peekKind() tokenKind {
	return p.peek().kind
}

func (p *dslParser) next() dslToken {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *dslParser) skipNewlines() {
	for p.peekKind() == tokNewline {
		p.next()
	}
}

func (p *dslParser) skipToNewline() {
	for {
		k := p.peekKind()
		if k == tokNewline || k == tokEOF {
			return
		}
		p.next()
	}
}

func (p *dslParser) skipBlock() {
	depth := 1
	for depth > 0 {
		t := p.next()
		switch t.kind {
		case tokLBrace:
			depth++
		case tokRBrace:
			depth--
		case tokEOF:
			return
		case tokIdent, tokString, tokEquals, tokArrow, tokNewline:
			// ignore other tokens
		}
	}
}

func (p *dslParser) warn(msg string) {
	p.workspace.Warnings = append(p.workspace.Warnings, msg)
}

func (p *dslParser) currentScope() string {
	if len(p.scopeStack) == 0 {
		return ""
	}
	return p.scopeStack[len(p.scopeStack)-1]
}

func (p *dslParser) makeCanonicalID(declaredID string) string {
	if declaredID == "" {
		p.anonCounter++
		declaredID = fmt.Sprintf("_anon_%d", p.anonCounter)
	}
	if p.identMode == "hierarchical" && len(p.scopeStack) > 0 {
		return p.currentScope() + "." + declaredID
	}
	return declaredID
}

func (p *dslParser) resolveRef(ref string) string {
	if ref == "" {
		return ""
	}
	if p.identMode == "flat" {
		return ref
	}
	for i := len(p.scopeStack); i >= 1; i-- {
		candidate := p.scopeStack[i-1] + "." + ref
		if _, ok := p.registry[candidate]; ok {
			return candidate
		}
	}
	if _, ok := p.registry[ref]; ok {
		return ref
	}
	return ref
}

var elementKindMap = map[string]string{
	"person":         "person",
	"softwaresystem": "system",
	"container":      "container",
	"component":      "component",
	"element":        "element",
}

func isElementKw(s string) bool {
	_, ok := elementKindMap[strings.ToLower(s)]
	return ok
}

func normalizeKind(kw string) string {
	if t, ok := elementKindMap[strings.ToLower(kw)]; ok {
		return t
	}
	return "element"
}

// ---- Top-level ----

func (p *dslParser) parseFile() {
	p.skipNewlines()
	if p.peekKind() == tokEOF {
		return
	}
	if strings.ToLower(p.peek().value) == "workspace" {
		p.parseWorkspaceDecl()
	} else {
		p.parseWorkspaceBody()
	}
}

func (p *dslParser) parseWorkspaceDecl() {
	p.next() // consume "workspace"
	if strings.ToLower(p.peek().value) == "extends" {
		p.next()
		for p.peekKind() == tokString {
			p.next()
		}
	}
	for p.peekKind() == tokString {
		p.next()
	}
	p.skipNewlines()
	if p.peekKind() == tokLBrace {
		p.next()
		p.parseWorkspaceBody()
		if p.peekKind() == tokRBrace {
			p.next()
		}
	} else {
		p.parseWorkspaceBody()
		if p.peekKind() == tokRBrace {
			p.next()
		}
	}
}

func (p *dslParser) parseWorkspaceBody() {
	for {
		p.skipNewlines()
		t := p.peek()
		if t.kind == tokEOF || t.kind == tokRBrace {
			return
		}
		if t.kind != tokIdent {
			p.next()
			continue
		}
		val := strings.ToLower(t.value)
		switch val {
		case "model":
			p.next()
			p.skipNewlines()
			if p.peekKind() == tokLBrace {
				p.next()
				p.parseModelBody()
				if p.peekKind() == tokRBrace {
					p.next()
				}
			}
		case "views":
			p.next()
			p.skipNewlines()
			if p.peekKind() == tokLBrace {
				p.next()
				p.skipBlock()
			}
		case "!identifiers":
			p.next()
			p.skipNewlines()
			if p.peekKind() == tokIdent {
				mode := strings.ToLower(p.next().value)
				if mode == "hierarchical" || mode == "flat" {
					p.identMode = mode
				}
			}
		default:
			p.next()
			if strings.HasPrefix(val, "!") {
				p.skipNewlines()
				if p.peekKind() == tokLBrace {
					p.next()
					p.skipBlock()
				} else {
					p.skipToNewline()
				}
			} else {
				p.skipToNewline()
			}
		}
	}
}

// ---- Model body ----

func (p *dslParser) parseModelBody() {
	p.parseBodyStatements(nil)
}

func (p *dslParser) parseElementBody(e *ParsedElement) {
	p.parseBodyStatements(e)
}

func (p *dslParser) parseBodyStatements(e *ParsedElement) {
	for {
		p.skipNewlines()
		t := p.peek()
		if t.kind == tokEOF || t.kind == tokRBrace {
			return
		}

		if t.kind == tokArrow {
			p.parseRelationship("")
			continue
		}

		if t.kind != tokIdent {
			p.next()
			continue
		}

		val := strings.ToLower(t.value)

		if strings.HasPrefix(val, "!") {
			p.parseDirective()
			continue
		}

		if e != nil {
			switch val {
			case "description":
				p.next()
				if p.peekKind() == tokString {
					e.Description = p.next().value
				}
				continue
			case "technology":
				p.next()
				if p.peekKind() == tokString {
					e.Technology = p.next().value
				}
				continue
			case "tags", "tag":
				p.next()
				for p.peekKind() == tokString {
					p.next()
				}
				continue
			case "url":
				p.next()
				if p.peekKind() == tokString {
					p.next()
				}
				continue
			}
		}

		pk1 := p.peekAt(1)
		if pk1.kind == tokEquals {
			p.parseAssignment()
			continue
		}

		if pk1.kind == tokArrow {
			src := p.next().value
			p.parseRelationship(src)
			continue
		}

		if val == "group" {
			p.parseGroup()
			continue
		}

		if isElementKw(val) {
			p.parseElement("", val)
			continue
		}

		p.next()
		p.skipToNewline()
	}
}

func (p *dslParser) parseDirective() {
	val := strings.ToLower(p.peek().value)
	p.next()
	switch val {
	case "!identifiers":
		p.skipNewlines()
		if p.peekKind() == tokIdent {
			mode := strings.ToLower(p.next().value)
			if mode == "hierarchical" || mode == "flat" {
				p.identMode = mode
			}
		}
	default:
		p.skipNewlines()
		if p.peekKind() == tokLBrace {
			p.next()
			p.skipBlock()
		} else {
			p.skipToNewline()
		}
	}
}

func (p *dslParser) parseAssignment() {
	assignedID := p.next().value
	p.next()
	p.skipNewlines()

	t := p.peek()
	if t.kind == tokArrow {
		p.parseRelationship("")
		return
	}
	if t.kind != tokIdent {
		p.skipToNewline()
		return
	}

	val := strings.ToLower(t.value)

	if val == "group" {
		p.parseGroup()
		return
	}

	if isElementKw(val) {
		p.parseElement(assignedID, val)
		return
	}

	if p.peekAt(1).kind == tokArrow {
		src := p.next().value
		p.parseRelationship(src)
		return
	}

	if p.peekAt(1).kind == tokString {
		kwName := p.next().value
		p.warn(fmt.Sprintf("unknown element type %q (archetype instance), treating as element", kwName))
		p.parseElementBody_archetype(assignedID)
		return
	}

	p.skipToNewline()
}

func (p *dslParser) parseGroup() {
	p.next()
	if p.peekKind() == tokString {
		p.next()
	}
	p.skipNewlines()
	if p.peekKind() == tokLBrace {
		p.next()
		p.parseBodyStatements(nil)
		if p.peekKind() == tokRBrace {
			p.next()
		}
	}
}

func (p *dslParser) parseElement(declaredID string, val string) {
	p.next()

	var strs []string
	for p.peekKind() == tokString {
		strs = append(strs, p.next().value)
	}

	name := ""
	description := ""
	technology := ""
	if len(strs) > 0 {
		name = strs[0]
	}
	if len(strs) > 1 {
		description = strs[1]
	}
	kwLow := strings.ToLower(val)
	if len(strs) > 2 && (kwLow == "container" || kwLow == "component" || kwLow == "element") {
		technology = strs[2]
	}

	canonicalID := p.makeCanonicalID(declaredID)
	element := &ParsedElement{
		ID:          canonicalID,
		Name:        name,
		Kind:        normalizeKind(val),
		Description: description,
		Technology:  technology,
	}
	p.registry[canonicalID] = element
	p.workspace.Elements = append(p.workspace.Elements, *element)

	p.skipNewlines()
	if p.peekKind() == tokLBrace {
		p.next()
		p.scopeStack = append(p.scopeStack, canonicalID)
		p.parseElementBody(element)
		p.scopeStack = p.scopeStack[:len(p.scopeStack)-1]
		p.registry[canonicalID] = element
		for i := range p.workspace.Elements {
			if p.workspace.Elements[i].ID == canonicalID {
				p.workspace.Elements[i] = *element
				break
			}
		}
		if p.peekKind() == tokRBrace {
			p.next()
		}
	}
}

func (p *dslParser) parseElementBody_archetype(declaredID string) {
	var strs []string
	for p.peekKind() == tokString {
		strs = append(strs, p.next().value)
	}
	name := ""
	description := ""
	if len(strs) > 0 {
		name = strs[0]
	}
	if len(strs) > 1 {
		description = strs[1]
	}
	canonicalID := p.makeCanonicalID(declaredID)
	element := &ParsedElement{
		ID:          canonicalID,
		Name:        name,
		Kind:        "element",
		Description: description,
	}
	p.registry[canonicalID] = element
	p.workspace.Elements = append(p.workspace.Elements, *element)

	p.skipNewlines()
	if p.peekKind() == tokLBrace {
		p.next()
		p.scopeStack = append(p.scopeStack, canonicalID)
		p.parseElementBody(element)
		p.scopeStack = p.scopeStack[:len(p.scopeStack)-1]
		p.registry[canonicalID] = element
		for i := range p.workspace.Elements {
			if p.workspace.Elements[i].ID == canonicalID {
				p.workspace.Elements[i] = *element
				break
			}
		}
		if p.peekKind() == tokRBrace {
			p.next()
		}
	}
}

func (p *dslParser) parseRelationship(srcRef string) {
	if p.peekKind() != tokArrow {
		p.skipToNewline()
		return
	}
	p.next()

	if srcRef == "" {
		if len(p.scopeStack) == 0 {
			p.warn("implicit relationship outside element scope")
			p.skipToNewline()
			return
		}
		srcRef = p.currentScope()
	}

	if p.peekKind() != tokIdent {
		p.skipToNewline()
		return
	}
	dstRef := p.next().value
	if strings.ToLower(dstRef) == "this" {
		if len(p.scopeStack) > 0 {
			dstRef = p.currentScope()
		}
	}

	var strs []string
	for p.peekKind() == tokString {
		strs = append(strs, p.next().value)
	}

	label := ""
	technology := ""
	if len(strs) > 0 {
		label = strs[0]
	}
	if len(strs) > 1 {
		technology = strs[1]
	}

	p.skipNewlines()
	if p.peekKind() == tokLBrace {
		p.next()
		p.skipBlock()
	}

	connector := ParsedConnector{
		SourceID:   p.resolveRef(srcRef),
		TargetID:   p.resolveRef(dstRef),
		Label:      label,
		Technology: technology,
	}
	p.workspace.Connectors = append(p.workspace.Connectors, connector)
}

func ParseStructurizr(input string) (*ParsedWorkspace, error) {
	tokens := tokenizeDSL(input)
	p := &dslParser{
		tokens:    tokens,
		identMode: "flat",
		registry:  make(map[string]*ParsedElement),
		workspace: &ParsedWorkspace{},
	}
	p.parseFile()
	return p.workspace, nil
}
