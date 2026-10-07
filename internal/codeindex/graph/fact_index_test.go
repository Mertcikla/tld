package graph

import (
	"fmt"
	"math/rand/v2"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

func TestFactIndexOverlappingSpans(t *testing.T) {
	g := NewGraph("repo", "snap")
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range 2000 {
		start := uint32(rng.IntN(200))
		f := &pb.CodeFact{Id: fmt.Sprint(i), Name: fmt.Sprint(i % 3), LogicalKey: fmt.Sprint(i), Anchor: &pb.SourceAnchor{Path: fmt.Sprint(i % 4), StartByte: start, EndByte: start + uint32(rng.IntN(200))}}
		g.Facts[f.Id] = f
	}
	g.Facts["nil-anchor"] = &pb.CodeFact{}
	index := NewFactIndex(g.Facts)
	if index.Enclosing(nil) != nil {
		t.Fatal("nil anchor matched")
	}
	for i := range 5000 {
		a := &pb.SourceAnchor{Path: fmt.Sprint(i % 5), StartByte: uint32(rng.IntN(300))}
		a.EndByte = a.StartByte + uint32(rng.IntN(30))
		for _, name := range []string{"", "1", "missing"} {
			var want *pb.CodeFact
			for _, f := range g.Facts {
				if f.Anchor != nil && f.Anchor.Path == a.Path && f.Anchor.StartByte <= a.StartByte && f.Anchor.EndByte >= a.EndByte && (name == "" || f.Name == name) && betterEnclosing(f, want) {
					want = f
				}
			}
			if got := index.enclosing(a, name, name != ""); got != want {
				t.Fatalf("anchor=%v name=%q: got %v want %v", a, name, got, want)
			}
		}
	}
}

func TestEnclosingFactEqualSpanStable(t *testing.T) {
	g := NewGraph("repo", "snap")
	for _, key := range []string{"z", "a"} {
		g.Facts[key] = &pb.CodeFact{Id: key, LogicalKey: key, Anchor: &pb.SourceAnchor{Path: "a", StartByte: 1, EndByte: 10}}
	}
	site := &pb.SourceAnchor{Path: "a", StartByte: 3, EndByte: 4}
	index := NewFactIndex(g.Facts)
	for range 100 {
		if g.EnclosingFact(site).Id != "a" || index.Enclosing(site).Id != "a" {
			t.Fatal("unstable equal-span owner")
		}
	}
}

func BenchmarkFactLookup(b *testing.B) {
	g := NewGraph("repo", "snap")
	for i := range 32000 {
		g.Facts[fmt.Sprint(i)] = &pb.CodeFact{Id: fmt.Sprint(i), LogicalKey: fmt.Sprint(i), Anchor: &pb.SourceAnchor{Path: fmt.Sprint(i / 32), StartByte: uint32(i % 32 * 100), EndByte: uint32(i%32*100 + 100)}}
	}
	site := &pb.SourceAnchor{Path: "500", StartByte: 150, EndByte: 151}
	b.Run("scan", func(b *testing.B) {
		for b.Loop() {
			g.EnclosingFact(site)
		}
	})
	index := NewFactIndex(g.Facts)
	b.Run("indexed", func(b *testing.B) {
		for b.Loop() {
			index.Enclosing(site)
		}
	})
	b.Run("build", func(b *testing.B) {
		for b.Loop() {
			NewFactIndex(g.Facts)
		}
	})
}
