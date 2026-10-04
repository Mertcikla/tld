package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

func ID(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
func Hash(b []byte) string            { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func RepositoryID(root string) string { return ID(filepath.Clean(root)) }

type Source struct {
	Path, Language, Hash string
	Text                 []byte
	InputBlob            string
	Dirty                bool
	SyntaxCache          string
	// FileCache stores this source's whole-file fact and chunks keyed by the
	// content hash they were built from, so unchanged files are not re-chunked.
	FileCache string
	// lines caches the byte offset of each line start so Position and Offset
	// resolve anchors in O(log n) instead of rescanning (and copying) the
	// source for every occurrence.
	lines []int
}

func (s *Source) Anchor(start, end int) *pb.SourceAnchor {
	if start < 0 {
		start = 0
	}
	if end > len(s.Text) {
		end = len(s.Text)
	}
	if end < start {
		end = start
	}
	sl, sc := s.Position(start)
	el, ec := s.Position(end)
	return &pb.SourceAnchor{Path: s.Path, StartByte: uint32(start), EndByte: uint32(end), StartLine: sl, StartColumn: sc, EndLine: el, EndColumn: ec, SourceHash: s.Hash}
}

// lineStarts lazily indexes the first byte of every line in Text. Each entry
// points at the byte following the preceding newline, so a source ending in a
// newline gains a final entry equal to len(Text).
func (s *Source) lineStarts() []int {
	if s.lines == nil {
		starts := make([]int, 1, 1+len(s.Text)/40)
		for i, c := range s.Text {
			if c == '\n' {
				starts = append(starts, i+1)
			}
		}
		s.lines = starts
	}
	return s.lines
}

// Position translates a byte offset into a 0-based line and byte column.
func (s *Source) Position(off int) (uint32, uint32) {
	if off > len(s.Text) {
		off = len(s.Text)
	}
	if off < 0 {
		off = 0
	}
	starts := s.lineStarts()
	lo, hi := 0, len(starts)
	for lo < hi {
		mid := (lo + hi) / 2
		if starts[mid] <= off {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	line := lo - 1
	return uint32(line), uint32(off - starts[line])
}

// Offset translates a SCIP line/character pair into a UTF-8 byte offset.
func (s *Source) Offset(line, character int, encoding string) (int, error) {
	if line < 0 || character < 0 {
		return 0, fmt.Errorf("negative position")
	}
	starts := s.lineStarts()
	if line >= len(starts) {
		return 0, fmt.Errorf("line outside source")
	}
	start := starts[line]
	end := len(s.Text)
	if line+1 < len(starts) {
		end = starts[line+1] - 1
	}
	if end > start && s.Text[end-1] == '\r' {
		end--
	}
	chunk := s.Text[start:end]
	switch strings.ToLower(encoding) {
	case "utf-8", "utf8", "":
		if character > len(chunk) {
			return 0, fmt.Errorf("character outside line")
		}
		return start + character, nil
	case "utf-16", "utf16", "utf-32", "utf32":
		units := 0
		for i := 0; i < len(chunk); {
			if units == character {
				return start + i, nil
			}
			r, n := utf8.DecodeRune(chunk[i:])
			if r == utf8.RuneError && n == 1 {
				return 0, fmt.Errorf("invalid UTF-8 source")
			}
			step := 1
			if strings.Contains(encoding, "16") {
				step = len(utf16.Encode([]rune{r}))
			}
			units += step
			i += n
			if units > character {
				return 0, fmt.Errorf("position splits codepoint")
			}
		}
		if units == character {
			return start + len(chunk), nil
		}
		return 0, fmt.Errorf("character outside line")
	default:
		return 0, fmt.Errorf("unsupported encoding %q", encoding)
	}
}

// OffsetForLineColumn resolves a 1-based line/column span to byte offsets in the
// source. It backs the infrastructure fact extractors.
func (s *Source) OffsetForLineColumn(line, column, endLine, endColumn int) (int, int) {
	start := s.columnOffset(line, column)
	end := start
	if endLine > 0 && endColumn > 0 {
		end = s.columnOffset(endLine, endColumn)
	}
	if end < start {
		end = start
	}
	return start, end
}

func (s *Source) columnOffset(line, column int) int {
	if line <= 0 {
		return 0
	}
	if column <= 0 {
		column = 1
	}
	starts := s.lineStarts()
	start := len(s.Text)
	if line-1 < len(starts) {
		start = starts[line-1]
	}
	offset := start + column - 1
	if offset > len(s.Text) {
		offset = len(s.Text)
	}
	return offset
}

// Position is a byte-slice convenience wrapper. Prefer Source.Position so the
// line index is built once and reused across every anchor for a file.
func Position(b []byte, off int) (uint32, uint32) {
	return (&Source{Text: b}).Position(off)
}

// Offset is a byte-slice convenience wrapper. Prefer Source.Offset so the line
// index is built once and reused across every anchor for a file.
func Offset(b []byte, line, character int, encoding string) (int, error) {
	return (&Source{Text: b}).Offset(line, character, encoding)
}

type ProjectArtifact struct {
	Fingerprint string
	Data        []byte
}

type Graph struct {
	ProjectArtifacts map[string]ProjectArtifact
	RepositoryID     string
	SnapshotID       string
	Sources          map[string]*Source
	Facts            map[string]*pb.CodeFact
	Chunks           map[string]*pb.Chunk
	EdgeFacts        map[string]*pb.EdgeFact
	// Reused records entity ids whose content was carried over unchanged from a
	// previous snapshot. Publishing can skip re-writing their immutable rows and
	// only record membership.
	Reused map[string]bool
}

func NewGraph(repo, snapshot string) *Graph {
	return &Graph{ProjectArtifacts: map[string]ProjectArtifact{}, RepositoryID: repo, SnapshotID: snapshot, Sources: map[string]*Source{}, Facts: map[string]*pb.CodeFact{}, Chunks: map[string]*pb.Chunk{}, EdgeFacts: map[string]*pb.EdgeFact{}, Reused: map[string]bool{}}
}
func (g *Graph) AddFact(kind pb.FactKind, name, language string, anchor *pb.SourceAnchor, code, signature string, evidence *pb.Evidence) *pb.CodeFact {
	id := ID(g.SnapshotID, "fact", anchor.Path, fmt.Sprint(anchor.StartByte), fmt.Sprint(anchor.EndByte), kind.String(), name)
	if f := g.Facts[id]; f != nil {
		f.Evidence = appendEvidence(f.Evidence, evidence)
		return f
	}
	f := &pb.CodeFact{Id: id, RepositoryId: g.RepositoryID, SnapshotId: g.SnapshotID, Kind: kind, Name: name, Language: language, Anchor: anchor, Code: code, Signature: signature, LogicalKey: LogicalFactKey(kind, name, anchor.Path)}
	f.Evidence = appendEvidence(f.Evidence, evidence)
	g.Facts[id] = f
	return f
}

// LogicalFactKey is the structural identity of a declaration that survives
// across snapshots, letting the same Fact be joined between versions.
func LogicalFactKey(kind pb.FactKind, name, path string) string {
	return strings.Join([]string{"fact", path, kind.String(), name}, "|")
}

// AddInfraFact records an infrastructure or configuration fact. Infra facts
// reuse the code-fact fields rather than extending the schema: name holds the
// object value, qualified_name the owning subject, and evidence.producer the
// scanner that produced the fact. Imports and logical_key are unchanged.
func (g *Graph) AddInfraFact(kind pb.FactKind, subject, object, language, extractor string, anchor *pb.SourceAnchor, text string) *pb.CodeFact {
	id := ID(g.SnapshotID, "fact", kind.String(), subject, object, anchor.Path, fmt.Sprint(anchor.StartByte))
	if f := g.Facts[id]; f != nil {
		return f
	}
	f := &pb.CodeFact{
		Id: id, RepositoryId: g.RepositoryID, SnapshotId: g.SnapshotID,
		Kind: kind, Name: object, QualifiedName: subject, Language: language,
		Anchor: anchor, Code: text,
		LogicalKey: LogicalInfraKey(kind, subject, object, anchor.Path),
	}
	if extractor != "" {
		f.Evidence = appendEvidence(f.Evidence, &pb.Evidence{Producer: extractor, Anchor: anchor})
	}
	g.Facts[id] = f
	return f
}

// LogicalInfraKey is the cross-snapshot identity of an infrastructure fact.
func LogicalInfraKey(kind pb.FactKind, subject, object, path string) string {
	return strings.Join([]string{"fact", path, kind.String(), subject, object}, "|")
}

// LogicalEdgeKey is the cross-snapshot identity of a relationship, ignoring the
// observing anchor so repeated observations aggregate onto one logical edge.
func (g *Graph) LogicalEdgeKey(kind pb.EdgeKind, from, target string) string {
	fromKey := ""
	if f := g.Facts[from]; f != nil {
		fromKey = f.LogicalKey
	}
	return strings.Join([]string{"edge", kind.String(), fromKey, target}, "|")
}
func (g *Graph) AddChunk(factID string, anchor *pb.SourceAnchor, text, context string, index, total uint32) *pb.Chunk {
	id := ID(g.SnapshotID, "chunk", factID, fmt.Sprint(index))
	c := &pb.Chunk{Id: id, FactId: factID, SnapshotId: g.SnapshotID, Anchor: anchor, Text: text, Context: context, Index: index, Total: total}
	g.Chunks[id] = c
	return c
}
func (g *Graph) AddEdgeFact(kind pb.EdgeKind, from, to, targetKey string, anchor *pb.SourceAnchor, evidence *pb.Evidence) *pb.EdgeFact {
	if anchor == nil || kind == pb.EdgeKind_EDGE_KIND_UNSPECIFIED {
		return nil
	}
	id := ID(g.SnapshotID, "edge-fact", kind.String(), from, to, targetKey, anchor.Path, fmt.Sprint(anchor.StartByte), fmt.Sprint(anchor.EndByte))
	if e := g.EdgeFacts[id]; e != nil {
		e.Evidence = appendEvidence(e.Evidence, evidence)
		return e
	}
	target := targetKey
	if to != "" {
		if f := g.Facts[to]; f != nil {
			target = f.LogicalKey
		}
	}
	e := &pb.EdgeFact{Id: id, RepositoryId: g.RepositoryID, SnapshotId: g.SnapshotID, Kind: kind, FromFactId: from, ToFactId: to, TargetSymbolKey: targetKey, Anchor: anchor, LogicalKey: g.LogicalEdgeKey(kind, from, target)}
	e.Evidence = appendEvidence(e.Evidence, evidence)
	g.EdgeFacts[id] = e
	return e
}

// AdoptFact re-keys an existing Fact from another snapshot into this graph,
// preserving its code, evidence, and stable logical identity. The caller is
// responsible for remapping ParentFactId once all facts are adopted.
func (g *Graph) AdoptFact(f *pb.CodeFact) *pb.CodeFact {
	if f == nil || f.Anchor == nil {
		return nil
	}
	id := ID(g.SnapshotID, "fact", f.Anchor.Path, fmt.Sprint(f.Anchor.StartByte), fmt.Sprint(f.Anchor.EndByte), f.Kind.String(), f.Name)
	if existing := g.Facts[id]; existing != nil {
		return existing
	}
	c := &pb.CodeFact{
		Id: id, RepositoryId: g.RepositoryID, SnapshotId: g.SnapshotID,
		Language: f.Language, Anchor: f.Anchor, Kind: f.Kind, Name: f.Name,
		QualifiedName: f.QualifiedName, SymbolKey: f.SymbolKey, Signature: f.Signature,
		Documentation: f.Documentation, Code: f.Code, LogicalKey: f.LogicalKey,
		Evidence: append([]*pb.Evidence(nil), f.Evidence...),
		Imports:  append([]string(nil), f.Imports...),
	}
	g.Facts[id] = c
	return c
}

// AdoptFactAnchored carries a fact into this graph at a new anchor, refreshing
// its code and preserving its identity. The original id is retained so an
// unchanged declaration keeps the same fact across snapshots, which lets the
// publisher and incremental cache treat it as reusable.
func (g *Graph) AdoptFactAnchored(f *pb.CodeFact, anchor *pb.SourceAnchor, code, signature string) *pb.CodeFact {
	if f == nil || anchor == nil {
		return nil
	}
	id := f.Id
	if id == "" {
		id = ID(g.SnapshotID, "fact", anchor.Path, fmt.Sprint(anchor.StartByte), fmt.Sprint(anchor.EndByte), f.Kind.String(), f.Name)
	}
	if existing := g.Facts[id]; existing != nil {
		return existing
	}
	c := &pb.CodeFact{
		Id: id, RepositoryId: g.RepositoryID, SnapshotId: g.SnapshotID,
		Language: f.Language, Anchor: anchor, Kind: f.Kind, Name: f.Name,
		QualifiedName: f.QualifiedName, SymbolKey: f.SymbolKey, Signature: signature,
		Documentation: f.Documentation, Code: code, LogicalKey: f.LogicalKey,
		Evidence: append([]*pb.Evidence(nil), f.Evidence...),
		Imports:  append([]string(nil), f.Imports...),
	}
	g.Facts[id] = c
	g.Reused[id] = true
	return c
}

// AdoptChunkAnchored inserts an already re-anchored chunk, preserving its id so
// unchanged chunks keep their identity across snapshots.
func (g *Graph) AdoptChunkAnchored(c *pb.Chunk, factID string) *pb.Chunk {
	if c == nil {
		return nil
	}
	id := c.Id
	if id == "" {
		id = ID(g.SnapshotID, "chunk", factID, fmt.Sprint(c.Index))
	}
	n := &pb.Chunk{Id: id, FactId: factID, SnapshotId: g.SnapshotID, Anchor: c.Anchor, Text: c.Text, Context: c.Context, Index: c.Index, Total: c.Total}
	g.Chunks[id] = n
	g.Reused[id] = true
	return n
}

// AdoptChunk re-keys a chunk into this graph under a (possibly remapped) Fact id.
func (g *Graph) AdoptChunk(c *pb.Chunk, factID string) *pb.Chunk {
	id := ID(g.SnapshotID, "chunk", factID, fmt.Sprint(c.Index))
	n := &pb.Chunk{Id: id, FactId: factID, SnapshotId: g.SnapshotID, Anchor: c.Anchor, Text: c.Text, Context: c.Context, Index: c.Index, Total: c.Total}
	g.Chunks[id] = n
	return n
}

// AdoptEdgeFact re-keys an EdgeFact from another snapshot into this graph with
// already-remapped endpoints, carrying all evidence. It returns nil when the
// relationship can no longer be represented because both endpoints are gone.
func (g *Graph) AdoptEdgeFact(e *pb.EdgeFact, from, to, targetKey string) *pb.EdgeFact {
	if e == nil || e.Anchor == nil || e.Kind == pb.EdgeKind_EDGE_KIND_UNSPECIFIED {
		return nil
	}
	if from == "" {
		return nil
	}
	if to == "" && targetKey == "" {
		return nil
	}
	id := ID(g.SnapshotID, "edge-fact", e.Kind.String(), from, to, targetKey, e.Anchor.Path, fmt.Sprint(e.Anchor.StartByte), fmt.Sprint(e.Anchor.EndByte))
	if existing := g.EdgeFacts[id]; existing != nil {
		return existing
	}
	target := targetKey
	if to != "" {
		if f := g.Facts[to]; f != nil {
			target = f.LogicalKey
		}
	}
	n := &pb.EdgeFact{Id: id, RepositoryId: g.RepositoryID, SnapshotId: g.SnapshotID, Kind: e.Kind, FromFactId: from, ToFactId: to, TargetSymbolKey: targetKey, Anchor: e.Anchor, LogicalKey: g.LogicalEdgeKey(e.Kind, from, target)}
	n.Evidence = append([]*pb.Evidence(nil), e.Evidence...)
	g.EdgeFacts[id] = n
	return n
}

// FactsByLogicalKey indexes this graph's facts by their stable logical identity.
func (g *Graph) FactsByLogicalKey() map[string]*pb.CodeFact {
	out := make(map[string]*pb.CodeFact, len(g.Facts))
	for _, f := range g.Facts {
		if f.LogicalKey != "" {
			out[f.LogicalKey] = f
		}
	}
	return out
}

func appendEvidence(all []*pb.Evidence, e *pb.Evidence) []*pb.Evidence {
	if e == nil {
		return all
	}
	for _, x := range all {
		if x.Producer == e.Producer && x.OriginalId == e.OriginalId && x.OriginalRange == e.OriginalRange && sameAnchor(x.Anchor, e.Anchor) {
			return all
		}
	}
	return append(all, e)
}
func sameAnchor(a, b *pb.SourceAnchor) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Path == b.Path && a.SourceHash == b.SourceHash && a.StartByte == b.StartByte && a.EndByte == b.EndByte
}

// EnclosingFact returns the smallest declaration containing the source site.
func (g *Graph) EnclosingFact(anchor *pb.SourceAnchor) *pb.CodeFact {
	if anchor == nil {
		return nil
	}
	var best *pb.CodeFact
	for _, f := range g.Facts {
		a := f.Anchor
		if a.Path != anchor.Path || a.StartByte > anchor.StartByte || a.EndByte < anchor.EndByte {
			continue
		}
		if best == nil || a.EndByte-a.StartByte < best.Anchor.EndByte-best.Anchor.StartByte {
			best = f
		}
	}
	return best
}
func (g *Graph) SortedFacts() []*pb.CodeFact {
	out := make([]*pb.CodeFact, 0, len(g.Facts))
	for _, f := range g.Facts {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out
}
func (g *Graph) SortedChunks() []*pb.Chunk {
	out := make([]*pb.Chunk, 0, len(g.Chunks))
	for _, c := range g.Chunks {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out
}
func (g *Graph) SortedEdgeFacts() []*pb.EdgeFact {
	out := make([]*pb.EdgeFact, 0, len(g.EdgeFacts))
	for _, e := range g.EdgeFacts {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Id < out[j].Id })
	return out
}
