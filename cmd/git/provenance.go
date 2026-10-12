package git

import (
	"fmt"
	"io"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

func writeProvenance(opts compareOptions, scene *pb.ImpactScene, stderr io.Writer) error {
	if !opts.debugProvenance || scene == nil {
		return nil
	}
	if _, err := fmt.Fprintf(stderr, "impact provenance: %d changed files represented; radius %d/%d; complete manifest=%t\n", len(scene.Coverage), scene.EffectiveRadius, scene.RequestedRadius, scene.ManifestComplete); err != nil {
		return err
	}
	for _, stage := range scene.GetStages() {
		if _, err := fmt.Fprintf(stderr, "  %s: +%d added, %d annotated, %d reused, %d suppressed\n", stage.Name, stage.Added, stage.Annotated, stage.Reused, stage.Suppressed); err != nil {
			return err
		}
	}
	for _, decision := range scene.GetDecisions() {
		if _, err := fmt.Fprintf(stderr, "  [%s] %s %s: %s\n", decision.Stage, decision.Action, decision.Key, decision.Reason); err != nil {
			return err
		}
	}
	return nil
}
