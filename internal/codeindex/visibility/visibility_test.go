package visibility

import (
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/project"
)

func fn(ref, path string) project.Element {
	return project.Element{Ref: ref, Name: ref, Kind: pb.FactKind_FACT_KIND_FUNCTION, FilePath: path}
}

func decisionsByRef(ds []Decision) map[string]Decision {
	out := make(map[string]Decision, len(ds))
	for _, d := range ds {
		out[d.Ref] = d
	}
	return out
}

func TestComputeVisibility(t *testing.T) {
	proj := project.Result{
		Elements: []project.Element{
			fn("ref-A", "a.go"),
			fn("ref-B", "b.go"),
			fn("ref-C", "c.go"),
			{Ref: "ref-D", Name: "D", Kind: pb.FactKind_FACT_KIND_DEPLOYABLE, FilePath: "deploy.yaml"},
		},
		Connectors: []project.Connector{{Ref: "e-AB", Kind: pb.EdgeKind_EDGE_KIND_CALLS, FromRef: "ref-A", ToRef: "ref-B", Weight: 1}},
	}
	in := Input{Projection: proj, ChangedFiles: map[string]bool{"a.go": true}}
	byRef := decisionsByRef(Compute(in, DefaultConfig()))

	if !byRef["ref-A"].Visible || byRef["ref-A"].Score != 100 {
		t.Fatalf("changed element A = %+v", byRef["ref-A"])
	}
	// B is unchanged but adjacent to the changed seed, so proximity keeps it visible.
	if !byRef["ref-B"].Visible || byRef["ref-B"].Reason != "proximity" {
		t.Fatalf("proximity element B = %+v", byRef["ref-B"])
	}
	// C is untouched and disconnected: below the core threshold.
	if byRef["ref-C"].Visible {
		t.Fatalf("untouched element C should be hidden: %+v", byRef["ref-C"])
	}
	// Infrastructure high-signal facts surface without a change.
	if !byRef["ref-D"].Visible {
		t.Fatalf("deployable D should be visible: %+v", byRef["ref-D"])
	}
}

func TestForceHideWins(t *testing.T) {
	proj := project.Result{Elements: []project.Element{fn("ref-A", "a.go")}}
	in := Input{Projection: proj, ChangedFiles: map[string]bool{"a.go": true}, ForceHide: map[string]bool{"ref-A": true}}
	d := decisionsByRef(Compute(in, DefaultConfig()))["ref-A"]
	if d.Visible {
		t.Fatalf("force-hidden element is visible: %+v", d)
	}
}

func TestHighDegreePenalty(t *testing.T) {
	proj := project.Result{Elements: []project.Element{fn("ref-H", "h.go")}}
	for i := 0; i < 12; i++ {
		ref := "ref-N" + string(rune('A'+i))
		proj.Elements = append(proj.Elements, fn(ref, ref+".go"))
		proj.Connectors = append(proj.Connectors, project.Connector{Ref: "e-" + ref, FromRef: "ref-H", ToRef: ref, Weight: 1})
	}
	d := decisionsByRef(Compute(Input{Projection: proj}, DefaultConfig()))["ref-H"]
	if d.Score != -1.5 {
		t.Fatalf("high-degree score = %v, want -1.5", d.Score)
	}
	if d.Visible {
		t.Fatalf("high-degree element should be hidden: %+v", d)
	}
}
