package git

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
)

// Each independent radius fixture has a different required presentation size.
// None of its direct representatives can be silently dropped to meet budgets.
func autoFallbackFixture(nodes int) *model.Diagram {
	d := autoCompareFixture(1, nodes-1)
	for _, p := range d.Scene.Provenance {
		if p.ResourceId != 1 {
			p.Role, p.Distance = pb.ImpactRole_IMPACT_ROLE_DIRECT, 0
		}
	}
	for i := 1; i < nodes; i++ {
		autoCompareEdge(d, int32(i), int32(i+1))
	}
	return d
}

func TestAutoCompareReassemblesSmallerRadiiForBudgets(t *testing.T) {
	for _, test := range []struct {
		name    string
		opts    compareOptions
		want    []uint32
		skipped bool
	}{
		{"readability", compareOptions{blastRadius: 2}, []uint32{2, 1}, false},
		{"nodes", compareOptions{blastRadius: 2, maxNodes: 4}, []uint32{2, 1, 0}, false},
		{"elements", compareOptions{blastRadius: 2, maxElements: 4}, []uint32{2, 1, 0}, false},
		{"connectors", compareOptions{blastRadius: 2, maxConnectors: 2}, []uint32{2, 1, 0}, false},
		{"bytes", compareOptions{blastRadius: 2, maxBytes: 25}, []uint32{2, 1, 0}, false},
		{"explicit one", compareOptions{blastRadius: 1, maxNodes: 4}, []uint32{1, 0}, false},
		{"explicit zero", compareOptions{blastRadius: 0}, []uint32{0}, false},
		{"zero still too large", compareOptions{blastRadius: 2, maxNodes: 1}, []uint32{2, 1, 0}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			scenes := map[uint32]*model.Diagram{2: autoFallbackFixture(26), 1: autoFallbackFixture(5), 0: autoFallbackFixture(2)}
			var loaded []uint32
			load := func(radius uint32) (*model.Diagram, error) {
				loaded = append(loaded, radius)
				return scenes[radius], nil
			}
			build := func(d *model.Diagram, _ uint32) (string, int, error) {
				text := strings.Repeat("x", impact.SceneNodeCount(d.Scene)*10)
				return text, len(text), nil
			}
			result, report, reason, err := selectAutoCompare(scenes[2], test.opts, build, load)
			if err != nil || !reflect.DeepEqual(loaded, test.want) || !reflect.DeepEqual(report.AttemptedRadii, test.want) {
				t.Fatalf("loaded=%v report=%+v err=%v", loaded, report, err)
			}
			if (reason != "") != test.skipped {
				t.Fatalf("unexpected reason: %q", reason)
			}
			if test.skipped {
				if result.text != "" || !strings.Contains(reason, "radius 0") {
					t.Fatalf("bad skip: %+v %s", result, reason)
				}
			} else if result.text == "" {
				t.Fatal("missing successful output")
			}
			chosen := test.want[len(test.want)-1]
			if result.radius != chosen || result.diagram.Scene.EffectiveRadius != chosen || result.diagram.Scene.RequestedRadius != uint32(test.opts.blastRadius) {
				t.Fatalf("incorrect radius metadata: %+v", result)
			}
			if impact.SceneNodeCount(result.diagram.Scene) != impact.SceneNodeCount(scenes[chosen].Scene) {
				t.Fatal("retained larger scene's authored context")
			}
			if len(result.diagram.Scene.Files) != 1 || impact.ValidateCoverage(result.diagram.Scene) != nil {
				t.Fatal("lost changed file")
			}
			if impact.SceneNodeCount(scenes[2].Scene) != 26 || scenes[2].Scene.RequestedRadius != 2 {
				t.Fatal("mutated cached assembly")
			}
		})
	}
}

func TestAutoCompareFallsBackOnConnectorReadability(t *testing.T) {
	large := autoFallbackFixture(2)
	for range 40 {
		autoCompareEdge(large, 1, 2)
	}
	small := autoFallbackFixture(2)
	opts := compareOptions{blastRadius: 2, markdown: true}
	render := compareRenderer{ctx: context.Background(), opts: opts}
	var loaded []uint32
	result, report, reason, err := selectAutoCompare(large, opts, render.build, func(radius uint32) (*model.Diagram, error) {
		loaded = append(loaded, radius)
		if radius > 0 {
			return large, nil
		}
		return small, nil
	})
	if err != nil || reason != "" || !reflect.DeepEqual(loaded, []uint32{2, 1, 0}) || result.radius != 0 || report.Diagrams != 1 || !strings.Contains(result.text, "```mermaid") {
		t.Fatalf("result=%+v report=%+v loaded=%v reason=%s err=%v", result, report, loaded, reason, err)
	}
}

func TestAutoCompareDoesNotHideReassemblyErrors(t *testing.T) {
	expected := errors.New("snapshot unavailable")
	var loaded []uint32
	opts := compareOptions{blastRadius: 2}
	render := compareRenderer{ctx: context.Background(), opts: opts}
	_, _, _, err := selectAutoCompare(autoFallbackFixture(26), opts, render.build, func(radius uint32) (*model.Diagram, error) { loaded = append(loaded, radius); return nil, expected })
	if !errors.Is(err, expected) || !reflect.DeepEqual(loaded, []uint32{2}) {
		t.Fatalf("error=%v attempts=%v", err, loaded)
	}
}

func TestAutoComparePlainIsLastFallback(t *testing.T) {
	for _, markdown := range []bool{false, true} {
		t.Run(map[bool]string{false: "mermaid", true: "markdown"}[markdown], func(t *testing.T) {
			d := autoFallbackFixture(2)
			opts := compareOptions{blastRadius: 2, mermaid: !markdown, markdown: markdown}
			// Pick a budget which fits exactly the flat output, but no nested output.
			plainOpts := opts
			plainOpts.plain = true
			plainRenderer := compareRenderer{ctx: context.Background(), opts: plainOpts}
			plain, _, reason, err := selectAutoCompare(d, plainOpts, plainRenderer.build, autoFixtureLoader(d))
			if err != nil || reason != "" {
				t.Fatalf("plain: %s %v", reason, err)
			}
			opts.maxBytes = len(plain.text)
			renderer := compareRenderer{ctx: context.Background(), opts: opts}
			var loaded []uint32
			result, report, reason, err := selectAutoCompare(d, opts, renderer.build, func(r uint32) (*model.Diagram, error) { loaded = append(loaded, r); return d, nil })
			if err != nil || reason != "" || !report.PlainFallback || result.radius != 0 || !reflect.DeepEqual(loaded, []uint32{2, 1, 0}) {
				t.Fatalf("report=%+v loads=%v reason=%s err=%v", report, loaded, reason, err)
			}
			if strings.Contains(result.text, "subgraph") || !strings.Contains(result.text, "file-01.go") || !strings.Contains(result.text, "file-02.go") {
				t.Fatalf("invalid plain fallback: %s", result.text)
			}
			if !result.diagram.Scene.GetComparison().GetPlain() || d.Scene.GetComparison().GetPlain() {
				t.Fatal("plain selection metadata mutated or missing")
			}
			opts.maxBytes = 1
			_, report, reason, err = selectAutoCompare(d, opts, renderer.build, autoFixtureLoader(d))
			if err != nil || reason == "" || !report.PlainFallback {
				t.Fatalf("failure did not exhaust plain: %+v %q %v", report, reason, err)
			}
		})
	}
}
