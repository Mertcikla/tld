package git

import (
	"fmt"
	"io"

	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
)

func writeProvenance(opts compareOptions, scene *model.Assembly, stderr io.Writer) error {
	if !opts.debugProvenance || scene == nil {
		return nil
	}
	public := impact.PublicScene(scene, true)
	if _, err := fmt.Fprintf(stderr, "impact provenance: %d changed files represented; radius %d/%d; complete manifest=%t\n", public.GetSummary().GetFiles(), public.GetComparison().GetEffectiveRadius(), public.GetComparison().GetRequestedRadius(), public.GetComparison().GetManifestComplete()); err != nil {
		return err
	}
	for _, decision := range public.GetProvenance().GetDecisions() {
		if _, err := fmt.Fprintf(stderr, "  [%s] %s %s: %s\n", decision.Stage, decision.Action, decision.Target, decision.Reason); err != nil {
			return err
		}
	}
	return nil
}
