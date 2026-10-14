package cmdutil

import (
	"fmt"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// EnsureConnectorPlacements makes a selected view's relationship visible while
// retaining every existing placement and coordinate in other views.
func EnsureConnectorPlacements(cmd *cobra.Command, sess *Session, ws *workspace.Workspace, view string, viewID int32, refs []string, ids []int32) ([]string, error) {
	if len(refs) != len(ids) {
		return nil, fmt.Errorf("endpoint IDs are required")
	}
	if err := workspace.CheckConnectorOwner(view, refs[0], refs[len(refs)-1]); err != nil {
		return nil, err
	}
	runner, err := sess.Runner()
	if err != nil {
		return nil, err
	}
	exported, err := runner.ExportWorkspace(cmd.Context())
	if err != nil {
		return nil, err
	}
	added := []string{}
	seen := map[int32]bool{}
	for i, ref := range refs {
		if seen[ids[i]] {
			continue
		}
		seen[ids[i]] = true
		present := false
		for _, p := range exported.Placements {
			if p.ViewId == viewID && p.ElementId == ids[i] {
				present = true
			}
		}
		if present {
			continue
		}
		x, y, _ := AutoPlace(ws, view, ref, nil)
		if err := runner.AddPlacement(sess.Context(cmd.Context()), viewID, ids[i], x, y); err != nil {
			if len(added) > 0 {
				return added, &CommandError{Cause: err, Code: "partial_mutation", Applied: true, Hint: "Some endpoint placements were saved. Retry the same command to finish connecting them."}
			}
			return added, err
		}
		ws.Elements[ref].Placements = append(ws.Elements[ref].Placements, workspace.ViewPlacement{ParentRef: view, PositionX: x, PositionY: y, PositionXSet: true, PositionYSet: true})
		added = append(added, ref)
	}
	return added, nil
}
