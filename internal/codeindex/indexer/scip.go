package indexer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	scip "github.com/scip-code/scip/bindings/go/scip"
)

type occurrence struct {
	key        string
	anchor     *pb.SourceAnchor
	roles      uint32
	version    string
	scipBacked bool
}
type symbolRelationship struct {
	fromKey string
	toKey   string
	kind    pb.EdgeKind
	version string
}
type symbols struct {
	definitions     map[string]string
	definitionSites map[string]*pb.SourceAnchor
	references      []occurrence
	relationships   []symbolRelationship
	metadata        map[string]*scip.SymbolInformation
	// skip, when set, names repository-relative documents already carried over
	// from a previous snapshot, so an incremental import ignores them.
	skip map[string]bool
}

func newSymbols() *symbols {
	return &symbols{definitions: map[string]string{}, definitionSites: map[string]*pb.SourceAnchor{}, metadata: map[string]*scip.SymbolInformation{}}
}
func symbolKey(symbol, path string) string {
	if strings.HasPrefix(symbol, "local ") {
		return path + "\x00" + symbol
	}
	return symbol
}
func importSCIP(ctx context.Context, g *graph.Graph, project *pb.Project, artifact string, hashes map[string]string, strict, scipBacked bool, table *symbols) error {
	file, err := os.Open(artifact)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return importSCIPReader(ctx, g, project, file, hashes, strict, scipBacked, table)
}
func importSCIPReader(ctx context.Context, g *graph.Graph, project *pb.Project, r io.Reader, hashes map[string]string, strict, scipBacked bool, table *symbols) error {
	var version string
	projectRoot := strings.Trim(project.Root, "/")
	visitor := &scip.IndexVisitor{}
	visitor.VisitMetadata = func(_ context.Context, m *scip.Metadata) error {
		if m.GetToolInfo() == nil {
			return fmt.Errorf("SCIP metadata lacks tool info")
		}
		version = m.GetToolInfo().GetVersion()
		return nil
	}
	visitor.VisitDocument = func(_ context.Context, d *scip.Document) error {
		rel := filepath.ToSlash(filepath.Clean(d.GetRelativePath()))
		if rel == "." || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
			return nil
		}
		path := rel
		if projectRoot != "" && projectRoot != "." {
			path = projectRoot + "/" + rel
		}
		if isTestSource(path) {
			return nil
		}
		if table.skip != nil && table.skip[path] {
			return nil
		}
		source := g.Sources[path]
		if source == nil {
			if strict {
				return fmt.Errorf("imported SCIP document %s has no captured source", path)
			}
			return nil
		}
		if d.GetText() == "" && hashes[rel] != source.Hash {
			return fmt.Errorf("SCIP document %s has no embedded text; supply a source-hash manifest", path)
		}
		if d.GetText() != "" && graph.Hash([]byte(d.GetText())) != source.Hash {
			return fmt.Errorf("SCIP source mismatch: %s", path)
		}
		encoding := "utf-8"
		declaredEncoding := true
		switch d.GetPositionEncoding() {
		case scip.PositionEncoding_UTF8CodeUnitOffsetFromLineStart:
			encoding = "utf-8"
		case scip.PositionEncoding_UTF16CodeUnitOffsetFromLineStart:
			encoding = "utf-16"
		case scip.PositionEncoding_UTF32CodeUnitOffsetFromLineStart:
			encoding = "utf-32"
		default:
			declaredEncoding = false
		}
		var declarations *declarationIndex
		if scipBacked {
			declarations = newDeclarationIndex(source)
			defer declarations.Release()
		}
		for _, info := range d.GetSymbols() {
			fromKey := symbolKey(info.GetSymbol(), path)
			table.metadata[fromKey] = info
			for _, relation := range info.GetRelationships() {
				if relation.GetSymbol() == "" {
					continue
				}
				toKey := symbolKey(relation.GetSymbol(), path)
				if relation.GetIsImplementation() {
					table.relationships = append(table.relationships, symbolRelationship{fromKey: fromKey, toKey: toKey, kind: pb.EdgeKind_EDGE_KIND_IMPLEMENTS, version: version})
				}
				if relation.GetIsTypeDefinition() {
					table.relationships = append(table.relationships, symbolRelationship{fromKey: fromKey, toKey: toKey, kind: pb.EdgeKind_EDGE_KIND_TYPE_DEFINITION, version: version})
				}
			}
		}
		for _, o := range d.GetOccurrences() {
			symbol := o.GetSymbol()
			if symbol == "" || strings.HasPrefix(symbol, "local ") {
				// Local symbols are document-scoped and are never published as
				// Facts or cross-file references; some indexers emit malformed
				// ranges for them, so they are skipped before anchor resolution.
				continue
			}
			anchor, ok := scipAnchorResilient(source, o, encoding, declaredEncoding)
			if !ok {
				// Some indexers emit malformed ranges for a subset of
				// occurrences; skip those rather than aborting the document.
				continue
			}
			key := symbolKey(symbol, path)
			if o.GetSymbolRoles()&int32(scip.SymbolRole_Definition) != 0 {
				if scipBacked {
					if fact := synthesizeFact(g, source, o, symbol, table.metadata[key], anchor, encoding, version, declarations); fact != nil {
						table.definitions[key] = fact.Id
						table.definitionSites[key] = anchor
					}
				} else if fact := matchDefinition(g, source, anchor); fact != nil {
					table.definitions[key] = fact.Id
					table.definitionSites[key] = anchor
					fact.SymbolKey = symbol
					fact.QualifiedName = symbol
					fact.Evidence = append(fact.Evidence, &pb.Evidence{Producer: "scip", Version: version, OriginalId: symbol, Anchor: anchor, Derivation: "matched declaration"})
					if info := table.metadata[key]; info != nil {
						enrichFact(fact, info)
					}
				}
				continue
			}
			table.references = append(table.references, occurrence{key: key, anchor: anchor, roles: uint32(o.GetSymbolRoles()), version: version, scipBacked: scipBacked})
		}
		return nil
	}
	if err := visitor.ParseStreaming(ctx, r); err != nil {
		return err
	}
	for key, id := range table.definitions {
		if fact := g.Facts[id]; fact != nil {
			if info := table.metadata[key]; info != nil {
				enrichFact(fact, info)
			}
		}
	}
	return nil
}

// synthesizeFact creates a code Fact directly from a SCIP definition occurrence
// for languages without a tree-sitter declaration extractor. The occurrence's
// enclosing range, when the indexer provides it, bounds the full declaration;
// otherwise the tree-sitter declaration containing the name is used, so
// languages whose indexers omit enclosing ranges still capture full
// declarations rather than just symbol names. FactKind comes from the
// indexer-provided symbol kind, falling back to the SCIP descriptor suffix.
func synthesizeFact(g *graph.Graph, source *graph.Source, o *scip.Occurrence, symbol string, info *scip.SymbolInformation, occurrenceAnchor *pb.SourceAnchor, encoding, version string, declarations *declarationIndex) *pb.CodeFact {
	kind := factKindFromSCIP(info.GetKind())
	if kind == pb.FactKind_FACT_KIND_UNSPECIFIED && info.GetKind() == scip.SymbolInformation_UnspecifiedKind {
		kind = factKindFromSymbol(symbol)
	}
	if kind == pb.FactKind_FACT_KIND_UNSPECIFIED {
		return nil
	}
	start, end := int(occurrenceAnchor.StartByte), int(occurrenceAnchor.EndByte)
	if lr, ok := occurrenceEnclosingLineRange(o); ok {
		s, err := source.Offset(lr.startLine, lr.startChar, encoding)
		if err == nil {
			e, endErr := source.Offset(lr.endLine, lr.endChar, encoding)
			if endErr == nil && e >= s && e <= len(source.Text) {
				start, end = s, e
			}
		}
	}
	if declStart, declEnd, ok := declarations.span(start, end); ok && declEnd-declStart > end-start {
		start, end = declStart, declEnd
	}
	if end <= start || start < 0 || end > len(source.Text) {
		return nil
	}
	name := info.GetDisplayName()
	if name == "" && int(occurrenceAnchor.EndByte) <= len(source.Text) {
		name = string(source.Text[occurrenceAnchor.StartByte:occurrenceAnchor.EndByte])
	}
	if name == "" {
		name = descriptorName(symbol)
	}
	if name == "" {
		return nil
	}
	anchor := source.Anchor(start, end)
	code := string(source.Text[start:end])
	signature := info.GetSignatureDocumentation().GetText()
	if signature == "" {
		signature = signatureContext(source.Text, start, end, name)
	}
	fact := g.AddFact(kind, name, source.Language, anchor, code, signature, &pb.Evidence{Producer: "scip", Version: version, OriginalId: symbol, Anchor: anchor, Derivation: "synthesized from definition occurrence"})
	fact.SymbolKey = symbol
	fact.QualifiedName = symbol
	// Descriptors contain declaration scope and overload disambiguators, while
	// package versions can change without changing a declaration's identity.
	identity, err := scip.DescriptorOnlyFormatter.Format(symbol)
	if err != nil {
		identity = symbol
	}
	fact.LogicalKey += "|symbol|" + identity
	if docs := info.GetDocumentation(); len(docs) > 0 {
		fact.Documentation = strings.Join(docs, "\n")
	}
	return fact
}

// factKindFromSCIP maps an indexer-provided symbol kind to a published Fact
// kind. Kinds outside the code-bearing declaration set return UNSPECIFIED so
// they are not synthesized as Facts.
func factKindFromSCIP(kind scip.SymbolInformation_Kind) pb.FactKind {
	switch kind {
	case scip.SymbolInformation_Class, scip.SymbolInformation_Object, scip.SymbolInformation_SingletonClass:
		return pb.FactKind_FACT_KIND_CLASS
	case scip.SymbolInformation_Struct, scip.SymbolInformation_Union:
		return pb.FactKind_FACT_KIND_STRUCT
	case scip.SymbolInformation_Interface, scip.SymbolInformation_Trait, scip.SymbolInformation_Protocol:
		return pb.FactKind_FACT_KIND_INTERFACE
	case scip.SymbolInformation_Enum:
		return pb.FactKind_FACT_KIND_ENUM
	case scip.SymbolInformation_Constructor:
		return pb.FactKind_FACT_KIND_CONSTRUCTOR
	case scip.SymbolInformation_Method, scip.SymbolInformation_AbstractMethod, scip.SymbolInformation_MethodSpecification,
		scip.SymbolInformation_MethodAlias, scip.SymbolInformation_StaticMethod, scip.SymbolInformation_SingletonMethod,
		scip.SymbolInformation_PureVirtualMethod, scip.SymbolInformation_ProtocolMethod, scip.SymbolInformation_TraitMethod,
		scip.SymbolInformation_TypeClassMethod:
		return pb.FactKind_FACT_KIND_METHOD
	case scip.SymbolInformation_Function, scip.SymbolInformation_Macro, scip.SymbolInformation_Getter,
		scip.SymbolInformation_Setter, scip.SymbolInformation_Operator:
		return pb.FactKind_FACT_KIND_FUNCTION
	case scip.SymbolInformation_Type, scip.SymbolInformation_TypeAlias, scip.SymbolInformation_AssociatedType,
		scip.SymbolInformation_TypeClass, scip.SymbolInformation_TypeFamily, scip.SymbolInformation_DataFamily,
		scip.SymbolInformation_Delegate, scip.SymbolInformation_Extension, scip.SymbolInformation_Mixin,
		scip.SymbolInformation_Concept:
		return pb.FactKind_FACT_KIND_TYPE
	}
	return pb.FactKind_FACT_KIND_UNSPECIFIED
}

// scipCallableKind reports whether a symbol kind denotes something that can be
// called, used to classify SCIP references as CALLS for languages indexed only
// through SCIP.
func scipCallableKind(kind scip.SymbolInformation_Kind) bool {
	switch kind {
	case scip.SymbolInformation_Function, scip.SymbolInformation_Method, scip.SymbolInformation_Constructor,
		scip.SymbolInformation_AbstractMethod, scip.SymbolInformation_MethodSpecification,
		scip.SymbolInformation_StaticMethod, scip.SymbolInformation_SingletonMethod,
		scip.SymbolInformation_PureVirtualMethod, scip.SymbolInformation_ProtocolMethod,
		scip.SymbolInformation_TraitMethod, scip.SymbolInformation_TypeClassMethod,
		scip.SymbolInformation_Macro, scip.SymbolInformation_Getter, scip.SymbolInformation_Setter,
		scip.SymbolInformation_Accessor, scip.SymbolInformation_MethodAlias, scip.SymbolInformation_Operator:
		return true
	}
	return false
}

// factKindCallable reports whether a published fact kind denotes a callable
// declaration, used as a fallback when an indexer omits SymbolInformation.Kind.
func factKindCallable(kind pb.FactKind) bool {
	switch kind {
	case pb.FactKind_FACT_KIND_FUNCTION, pb.FactKind_FACT_KIND_METHOD, pb.FactKind_FACT_KIND_CONSTRUCTOR:
		return true
	}
	return false
}

// factKindFromSymbol is the fallback for indexers that omit SymbolInformation.Kind.
// It reads the symbol descriptor suffix defined by the SCIP symbol grammar.
func factKindFromSymbol(symbol string) pb.FactKind {
	s := symbol
	if i := strings.IndexByte(s, ' '); i >= 0 {
		s = s[i+1:]
	}
	switch {
	case strings.HasSuffix(s, "()."):
		if strings.Contains(strings.TrimSuffix(s, "()."), "#") {
			return pb.FactKind_FACT_KIND_METHOD
		}
		return pb.FactKind_FACT_KIND_FUNCTION
	case strings.HasSuffix(s, ")."):
		return pb.FactKind_FACT_KIND_METHOD
	case strings.HasSuffix(s, "#"), strings.HasSuffix(s, "."):
		return pb.FactKind_FACT_KIND_TYPE
	}
	return pb.FactKind_FACT_KIND_UNSPECIFIED
}

// descriptorName extracts the trailing name segment from a SCIP symbol string.
func descriptorName(symbol string) string {
	s := symbol
	if i := strings.LastIndexByte(s, ' '); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	s = strings.TrimSuffix(s, "().")
	s = strings.TrimSuffix(s, ").")
	s = strings.TrimSuffix(s, ".")
	s = strings.TrimSuffix(s, "#")
	s = strings.TrimSuffix(s, "!")
	s = strings.TrimSuffix(s, ":")
	if i := strings.IndexByte(s, '('); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(s, "`")
}

func matchDefinition(g *graph.Graph, source *graph.Source, anchor *pb.SourceAnchor) *pb.CodeFact {
	if int(anchor.EndByte) > len(source.Text) {
		return nil
	}
	name := string(source.Text[anchor.StartByte:anchor.EndByte])
	var best *pb.CodeFact
	for _, f := range g.Facts {
		a := f.Anchor
		if a.Path != anchor.Path || a.StartByte > anchor.StartByte || a.EndByte < anchor.EndByte || f.Name != name {
			continue
		}
		if best == nil || a.EndByte-a.StartByte < best.Anchor.EndByte-best.Anchor.StartByte {
			best = f
		}
	}
	return best
}
func enrichFact(f *pb.CodeFact, info *scip.SymbolInformation) {
	if info.GetDisplayName() != "" {
		f.Name = info.GetDisplayName()
	}
	if docs := info.GetDocumentation(); len(docs) > 0 {
		f.Documentation = strings.Join(docs, "\n")
	}
	if sig := info.GetSignatureDocumentation(); sig != nil && sig.GetText() != "" {
		f.Signature = sig.GetText()
	}
}
func (table *symbols) apply(g *graph.Graph) {
	for _, ref := range table.references {
		to := table.definitions[ref.key]
		if to == "" {
			continue
		}
		owner := g.EnclosingFact(ref.anchor)
		if owner == nil {
			continue
		}
		g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_REFERENCES, owner.Id, to, "", ref.anchor, &pb.Evidence{Producer: "scip", Version: ref.version, OriginalId: ref.key, Anchor: ref.anchor})
		if ref.scipBacked {
			callable := false
			if info := table.metadata[ref.key]; info != nil && scipCallableKind(info.GetKind()) {
				callable = true
			} else if def := g.Facts[to]; def != nil && factKindCallable(def.Kind) {
				// Some indexers omit SymbolInformation.Kind. Fall back to the
				// referenced definition's fact kind so a reference to a
				// function, method, or constructor still yields a call edge.
				callable = true
			}
			if callable {
				g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, owner.Id, to, "", ref.anchor, &pb.Evidence{Producer: "scip", Version: ref.version, OriginalId: ref.key, Anchor: ref.anchor, Derivation: "reference to callable symbol"})
			}
		}
	}
	for _, relation := range table.relationships {
		from := table.definitions[relation.fromKey]
		if from == "" {
			continue
		}
		anchor := table.definitionSites[relation.fromKey]
		if anchor == nil {
			anchor = g.Facts[from].Anchor
		}
		to := table.definitions[relation.toKey]
		targetKey := ""
		if to == "" {
			targetKey = relation.toKey
		}
		g.AddEdgeFact(relation.kind, from, to, targetKey, anchor, &pb.Evidence{Producer: "scip", Version: relation.version, OriginalId: relation.fromKey + " -> " + relation.toKey, Anchor: anchor, Derivation: "symbol relationship"})
	}
	table.applyRustImpls(g)
}

// applyRustImpls derives IMPLEMENTS edges from Rust source. rust-analyzer emits
// no is_implementation relationships, so the tree-sitter grammar locates each
// "impl Trait for Type" block and the SCIP occurrences inside it resolve the
// trait and type identifiers to facts.
func (table *symbols) applyRustImpls(g *graph.Graph) {
	lang := parserLanguage("rust")
	if lang == nil {
		return
	}
	refs := table.byPath()
	for path, src := range g.Sources {
		if src == nil || src.Language != "rust" {
			continue
		}
		tree := parseDeclarationTree(lang, src.Text)
		if tree == nil {
			continue
		}
		root := wrapNode(tree.RootNode(), lang)
		var walk func(*tsNode)
		walk = func(n *tsNode) {
			if n == nil {
				return
			}
			if n.Kind() == "impl_item" {
				table.rustImplEdge(g, refs[path], n)
			}
			for i := 0; i < n.NamedChildCount(); i++ {
				walk(n.NamedChild(i))
			}
		}
		walk(root)
		tree.Release()
	}
}

// rustImplEdge records one IMPLEMENTS edge for an impl_item node when both the
// implementing type and the trait resolve to facts (the trait may be external).
func (table *symbols) rustImplEdge(g *graph.Graph, refs []occurrence, impl *tsNode) {
	traitNode := impl.ChildByFieldName("trait")
	typeNode := impl.ChildByFieldName("type")
	if traitNode == nil || typeNode == nil {
		return
	}
	typeKey := rustSymbolInSpan(refs, int(typeNode.StartByte()), int(typeNode.EndByte()))
	if typeKey == "" {
		return
	}
	fromType := table.definitions[typeKey]
	if fromType == "" {
		return
	}
	fromFact := g.Facts[fromType]
	if fromFact == nil || fromFact.Anchor == nil {
		return
	}
	traitKey := rustSymbolInSpan(refs, int(traitNode.StartByte()), int(traitNode.EndByte()))
	if traitKey == "" {
		return
	}
	to := table.definitions[traitKey]
	targetKey := ""
	if to == "" {
		// The trait lives in another crate; keep it as an external target.
		targetKey = traitKey
	}
	g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_IMPLEMENTS, fromType, to, targetKey, fromFact.Anchor, &pb.Evidence{Producer: "scip", Derivation: "rust impl_item"})
}

// rustSymbolInSpan returns the symbol of the largest SCIP occurrence fully
// contained in [start, end), which picks the type or trait identifier out of a
// larger syntactic node such as a generic type.
func rustSymbolInSpan(refs []occurrence, start, end int) string {
	best := ""
	bestLen := -1
	for i := range refs {
		a := refs[i].anchor
		if a == nil {
			continue
		}
		s, e := int(a.StartByte), int(a.EndByte)
		if s < start || e > end || e <= s {
			continue
		}
		if e-s > bestLen {
			bestLen = e - s
			best = refs[i].key
		}
	}
	return best
}
func (table *symbols) byPath() map[string][]occurrence {
	out := map[string][]occurrence{}
	for _, ref := range table.references {
		out[ref.anchor.Path] = append(out[ref.anchor.Path], ref)
	}
	for _, list := range out {
		sort.Slice(list, func(i, j int) bool { return list[i].anchor.StartByte < list[j].anchor.StartByte })
	}
	return out
}

// lineRange is a SCIP half-open [start, end) line/character range.
type lineRange struct {
	startLine, startChar, endLine, endChar int
}

func occurrenceLineRange(o *scip.Occurrence) (lineRange, bool) {
	if r := o.GetSingleLineRange(); r != nil {
		return lineRange{int(r.Line), int(r.StartCharacter), int(r.Line), int(r.EndCharacter)}, true
	}
	if r := o.GetMultiLineRange(); r != nil {
		return lineRange{int(r.StartLine), int(r.StartCharacter), int(r.EndLine), int(r.EndCharacter)}, true
	}
	// The untyped range is a legacy fallback for SCIP producers that do not
	// populate the typed single/multi-line ranges yet.
	return sliceLineRange(o.GetRange()) //nolint:staticcheck
}

func occurrenceEnclosingLineRange(o *scip.Occurrence) (lineRange, bool) {
	if r := o.GetSingleLineEnclosingRange(); r != nil {
		return lineRange{int(r.Line), int(r.StartCharacter), int(r.Line), int(r.EndCharacter)}, true
	}
	if r := o.GetMultiLineEnclosingRange(); r != nil {
		return lineRange{int(r.StartLine), int(r.StartCharacter), int(r.EndLine), int(r.EndCharacter)}, true
	}
	return sliceLineRange(o.GetEnclosingRange()) //nolint:staticcheck
}

func sliceLineRange(r []int32) (lineRange, bool) {
	switch len(r) {
	case 3:
		return lineRange{int(r[0]), int(r[1]), int(r[0]), int(r[2])}, true
	case 4:
		return lineRange{int(r[0]), int(r[1]), int(r[2]), int(r[3])}, true
	}
	return lineRange{}, false
}

func scipAnchor(s *graph.Source, o *scip.Occurrence, enc string) (*pb.SourceAnchor, error) {
	lr, ok := occurrenceLineRange(o)
	if !ok {
		return nil, fmt.Errorf("invalid range")
	}
	return anchorFromLineRange(s, lr, enc)
}

// scipAnchorResilient resolves an occurrence anchor, falling back to the other
// SCIP position encodings when the document left the encoding unspecified, and
// reporting false when no encoding yields a valid range.
func scipAnchorResilient(s *graph.Source, o *scip.Occurrence, enc string, declared bool) (*pb.SourceAnchor, bool) {
	if a, err := scipAnchor(s, o, enc); err == nil {
		return a, true
	}
	if declared {
		return nil, false
	}
	for _, alt := range []string{"utf-16", "utf-32", "utf-8"} {
		if alt == enc {
			continue
		}
		if a, err := scipAnchor(s, o, alt); err == nil {
			return a, true
		}
	}
	return nil, false
}

func anchorFromLineRange(s *graph.Source, lr lineRange, enc string) (*pb.SourceAnchor, error) {
	start, err := s.Offset(lr.startLine, lr.startChar, enc)
	if err != nil {
		return nil, err
	}
	end, err := s.Offset(lr.endLine, lr.endChar, enc)
	if err != nil {
		return nil, err
	}
	if end < start {
		return nil, fmt.Errorf("reversed range")
	}
	return s.Anchor(start, end), nil
}
