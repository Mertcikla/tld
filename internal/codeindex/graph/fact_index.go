package graph

import (
	"cmp"
	"sort"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

// FactIndex is a read-only lookup built after declaration extraction. Rebuild it
// when facts or their anchors change; edges and symbol metadata can change
// without invalidating it. Keeping it phase-local avoids stale Graph caches.
type FactIndex struct{ paths map[string]factSpans }
type factSpans struct {
	facts  []*pb.CodeFact
	maxEnd []uint32
}

func NewFactIndex(facts map[string]*pb.CodeFact) *FactIndex {
	index := &FactIndex{paths: make(map[string]factSpans)}
	for _, f := range facts {
		if f == nil || f.Anchor == nil {
			continue
		}
		spans := index.paths[f.Anchor.Path]
		spans.facts = append(spans.facts, f)
		index.paths[f.Anchor.Path] = spans
	}
	for path, spans := range index.paths {
		sort.Slice(spans.facts, func(i, j int) bool {
			a, b := spans.facts[i], spans.facts[j]
			if a.Anchor.StartByte != b.Anchor.StartByte {
				return a.Anchor.StartByte < b.Anchor.StartByte
			}
			if a.Anchor.EndByte != b.Anchor.EndByte {
				return a.Anchor.EndByte < b.Anchor.EndByte
			}
			return compareFactIdentity(a, b) < 0
		})
		spans.maxEnd = make([]uint32, len(spans.facts))
		var end uint32
		for i, f := range spans.facts {
			end = max(end, f.Anchor.EndByte)
			spans.maxEnd[i] = end
		}
		index.paths[path] = spans
	}
	return index
}

// Enclosing returns the smallest containing declaration. Equal spans use
// stable declaration identity rather than map order.
func (index *FactIndex) Enclosing(anchor *pb.SourceAnchor) *pb.CodeFact {
	return index.enclosing(anchor, "", false)
}

// NamedEnclosing additionally requires an exact declaration name match.
func (index *FactIndex) NamedEnclosing(anchor *pb.SourceAnchor, name string) *pb.CodeFact {
	return index.enclosing(anchor, name, true)
}

func (index *FactIndex) enclosing(anchor *pb.SourceAnchor, name string, named bool) *pb.CodeFact {
	if anchor == nil {
		return nil
	}
	spans := index.paths[anchor.Path]
	i := sort.Search(len(spans.facts), func(i int) bool { return spans.facts[i].Anchor.StartByte > anchor.StartByte })
	var best *pb.CodeFact
	for i--; i >= 0 && spans.maxEnd[i] >= anchor.EndByte; i-- {
		f := spans.facts[i]
		if f.Anchor.EndByte < anchor.EndByte || (named && f.Name != name) {
			continue
		}
		if betterEnclosing(f, best) {
			best = f
		}
	}
	return best
}

func betterEnclosing(f, best *pb.CodeFact) bool {
	if best == nil {
		return true
	}
	a, b := f.Anchor.EndByte-f.Anchor.StartByte, best.Anchor.EndByte-best.Anchor.StartByte
	return a < b || (a == b && compareFactIdentity(f, best) < 0)
}
func compareFactIdentity(a, b *pb.CodeFact) int {
	if n := cmp.Compare(a.LogicalKey, b.LogicalKey); n != 0 {
		return n
	}
	if n := cmp.Compare(a.Kind, b.Kind); n != 0 {
		return n
	}
	if n := cmp.Compare(a.Name, b.Name); n != 0 {
		return n
	}
	if n := cmp.Compare(a.BodyHash, b.BodyHash); n != 0 {
		return n
	}
	if n := cmp.Compare(a.Anchor.StartByte, b.Anchor.StartByte); n != 0 {
		return n
	}
	return cmp.Compare(a.Anchor.EndByte, b.Anchor.EndByte)
}
