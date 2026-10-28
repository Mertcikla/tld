package flow

import (
	"fmt"
	"slices"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/spf13/cobra"
)

func walk(steps []*diagv1.FlowStep, visit func(*diagv1.FlowStep)) {
	for _, s := range steps {
		visit(s)
		if b := s.GetBranches(); b != nil {
			for _, p := range b.Paths {
				walk(p.Steps, visit)
			}
		}
	}
}
func countSteps(steps []*diagv1.FlowStep) int {
	n := 0
	walk(steps, func(*diagv1.FlowStep) { n++ })
	return n
}
func stepAt(items *[]*diagv1.FlowStep, id string) (*diagv1.FlowStep, *[]*diagv1.FlowStep, int) {
	for i, s := range *items {
		if s.Id == id {
			return s, items, i
		}
		if b := s.GetBranches(); b != nil {
			for _, p := range b.Paths {
				if found, seq, index := stepAt(&p.Steps, id); found != nil {
					return found, seq, index
				}
			}
		}
	}
	return nil, nil, 0
}
func pathAt(items []*diagv1.FlowStep, id string) (*diagv1.FlowBranch, *diagv1.FlowBranches, int) {
	for _, s := range items {
		if b := s.GetBranches(); b != nil {
			for i, p := range b.Paths {
				if p.Id == id {
					return p, b, i
				}
				if found, owner, index := pathAt(p.Steps, id); found != nil {
					return found, owner, index
				}
			}
		}
	}
	return nil, nil, 0
}
func requireStep(f *diagv1.Flow, id string) (*diagv1.FlowStep, *[]*diagv1.FlowStep, int, error) {
	s, seq, i := stepAt(&f.Content.Steps, id)
	if s == nil {
		return nil, nil, 0, fmt.Errorf("step %q not found; use 'tld flow inspect' to find stable IDs", id)
	}
	return s, seq, i, nil
}
func requirePath(f *diagv1.Flow, id string) (*diagv1.FlowBranch, *diagv1.FlowBranches, int, error) {
	p, b, i := pathAt(f.Content.Steps, id)
	if p == nil {
		return nil, nil, 0, fmt.Errorf("path %q not found; use 'tld flow inspect' to find stable IDs", id)
	}
	return p, b, i, nil
}
func destination(c *cobra.Command, f *diagv1.Flow) (*[]*diagv1.FlowStep, error) {
	if id := str(c, "path"); id != "" {
		p, _, _, err := requirePath(f, id)
		if err != nil {
			return nil, err
		}
		return &p.Steps, nil
	}
	return &f.Content.Steps, nil
}
func insertion(c *cobra.Command, ids []string) (int, error) {
	before, after := str(c, "before"), str(c, "after")
	if before != "" && after != "" {
		return 0, fmt.Errorf("--before and --after are mutually exclusive")
	}
	if before == "" && after == "" {
		return len(ids), nil
	}
	anchor := before
	if anchor == "" {
		anchor = after
	}
	i := slices.Index(ids, anchor)
	if i < 0 {
		return 0, fmt.Errorf("ordering anchor %q is not in the destination sequence", anchor)
	}
	if after != "" {
		i++
	}
	return i, nil
}
func insertStep(c *cobra.Command, seq *[]*diagv1.FlowStep, s *diagv1.FlowStep) error {
	ids := []string{}
	for _, item := range *seq {
		ids = append(ids, item.Id)
	}
	i, err := insertion(c, ids)
	if err != nil {
		return err
	}
	*seq = slices.Insert(*seq, i, s)
	return nil
}
func ordering(c *cobra.Command, path bool) {
	if path {
		c.Flags().String("path", "", "destination branch path ID (default: top-level steps)")
	}
	c.Flags().String("before", "", "insert before this sibling ID")
	c.Flags().String("after", "", "insert after this sibling ID")
	c.MarkFlagsMutuallyExclusive("before", "after")
}
func contents(c *cobra.Command, update bool) {
	c.Flags().String("title", "", "step title")
	c.Flags().String("description", "", "step description")
	c.Flags().StringArray("element", nil, "element ref/name/ID to highlight; repeat to set the full list")
	c.Flags().StringArray("connector", nil, "connector key/ID to highlight; repeat to set the full list")
	c.Flags().String("go-to", "", "linked flow UUID or exact name")
	c.Flags().String("go-to-view", "", "scope the linked flow name to this view")
	if update {
		c.Flags().Bool("clear-elements", false, "remove all element highlights")
		c.Flags().Bool("clear-connectors", false, "remove all connector highlights")
		c.Flags().Bool("clear-go-to", false, "remove the linked flow")
		c.MarkFlagsMutuallyExclusive("element", "clear-elements")
		c.MarkFlagsMutuallyExclusive("connector", "clear-connectors")
		c.MarkFlagsMutuallyExclusive("go-to", "clear-go-to")
	}
}
func (e *environment) applyContents(c *cobra.Command, f *diagv1.Flow, s *diagv1.FlowStep) error {
	if c.Flags().Changed("title") {
		s.Title = str(c, "title")
	}
	if c.Flags().Changed("description") {
		s.Description = str(c, "description")
	}
	if flag(c, "clear-elements") {
		s.ElementIds = nil
	}
	if flag(c, "clear-connectors") {
		s.ConnectorIds = nil
	}
	if flag(c, "clear-go-to") {
		if s.GetGoToFlowId() != "" {
			s.Control = nil
		}
	}
	if c.Flags().Changed("element") || c.Flags().Changed("connector") {
		diagram, err := e.runner.ViewContent(c.Context(), f.ViewId)
		if err != nil {
			return err
		}
		if c.Flags().Changed("element") {
			s.ElementIds = nil
			for _, arg := range stringsFlag(c, "element") {
				ref, err := cmdutil.ResolveElementArg(e.ws, arg)
				if err != nil {
					return err
				}
				meta := e.ws.Meta.Elements[ref]
				if meta == nil {
					return fmt.Errorf("element %q not found", arg)
				}
				id := int32(meta.ID)
				placed := false
				for _, p := range diagram.Placements {
					placed = placed || p.ElementId == id
				}
				if !placed {
					return fmt.Errorf("element %q is not in the flow diagram", arg)
				}
				s.ElementIds = append(s.ElementIds, id)
			}
		}
		if c.Flags().Changed("connector") {
			s.ConnectorIds = nil
			for _, arg := range stringsFlag(c, "connector") {
				key, err := cmdutil.ResolveConnectorArg(e.ws, arg)
				if err != nil {
					return err
				}
				meta := e.ws.Meta.Connectors[key]
				if meta == nil {
					return fmt.Errorf("connector %q not found", arg)
				}
				id := int32(meta.ID)
				visible := false
				for _, x := range diagram.Connectors {
					visible = visible || x.Id == id
				}
				if !visible {
					return fmt.Errorf("connector %q is not in the flow diagram", arg)
				}
				s.ConnectorIds = append(s.ConnectorIds, id)
			}
		}
	}
	if c.Flags().Changed("go-to-view") && !c.Flags().Changed("go-to") {
		return fmt.Errorf("--go-to-view requires --go-to")
	}
	if c.Flags().Changed("go-to") {
		if s.GetBranches() != nil {
			return fmt.Errorf("step has branches; delete them before setting --go-to")
		}
		if str(c, "go-to") == "" {
			return fmt.Errorf("--go-to cannot be empty; use --clear-go-to")
		}
		view, err := e.view(str(c, "go-to-view"))
		if err != nil {
			return err
		}
		target, err := e.findFlow(str(c, "go-to"), view)
		if err != nil {
			return err
		}
		s.Control = &diagv1.FlowStep_GoToFlowId{GoToFlowId: target.Id}
	}
	return nil
}
func newStep() *cobra.Command {
	root := group("step", "Add, update, reorder, and delete flow steps")
	add := command("add <flow>", "Add a step; prints its stable ID", 1, true, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		id := str(c, "id")
		if c.Flags().Changed("id") && id == "" {
			return fmt.Errorf("--id must be nonempty")
		}
		if id == "" {
			id = uuid.NewString()
		}
		s := &diagv1.FlowStep{Id: id}
		if err = e.applyContents(c, f, s); err != nil {
			return err
		}
		seq, err := destination(c, f)
		if err != nil {
			return err
		}
		if err = insertStep(c, seq, s); err != nil {
			return err
		}
		return e.save(c, f, map[string]any{"step_id": id})
	})
	add.Flags().String("id", "", "stable step ID (default: generated UUID)")
	contents(add, false)
	ordering(add, true)
	update := command("update <flow> <step-id>", "Update selected step fields; omitted fields are preserved", 2, true, func(c *cobra.Command, e *environment, args []string) error {
		changed := false
		for _, name := range []string{"title", "description", "element", "connector", "go-to", "clear-elements", "clear-connectors", "clear-go-to"} {
			changed = changed || c.Flags().Changed(name)
		}
		if !changed {
			return fmt.Errorf("provide a step content flag to update")
		}
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		s, _, _, err := requireStep(f, args[1])
		if err != nil {
			return err
		}
		if err = e.applyContents(c, f, s); err != nil {
			return err
		}
		return e.save(c, f, map[string]any{"step_id": s.Id})
	})
	contents(update, true)
	move := command("move <flow> <step-id>", "Move a step and its descendants to a destination sequence", 2, true, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		s, seq, i, err := requireStep(f, args[1])
		if err != nil {
			return err
		}
		if str(c, "path") != "" {
			if p, _, _ := pathAt([]*diagv1.FlowStep{s}, str(c, "path")); p != nil {
				return fmt.Errorf("cannot move a step into its own descendant path")
			}
		}
		*seq = slices.Delete(*seq, i, i+1)
		dest, err := destination(c, f)
		if err != nil {
			return err
		}
		if err = insertStep(c, dest, s); err != nil {
			return err
		}
		return e.save(c, f, map[string]any{"step_id": s.Id})
	})
	ordering(move, true)
	del := command("delete <flow> <step-id>", "Delete a step and all its descendants", 2, true, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		s, seq, i, err := requireStep(f, args[1])
		if err != nil {
			return err
		}
		n := countSteps([]*diagv1.FlowStep{s})
		*seq = slices.Delete(*seq, i, i+1)
		return e.save(c, f, map[string]any{"step_id": s.Id, "deleted_steps": n})
	})
	root.AddCommand(add, update, move, del)
	return root
}
func branchMode(value string) (diagv1.FlowBranchMode, error) {
	switch value {
	case "alternate":
		return diagv1.FlowBranchMode_FLOW_BRANCH_MODE_ALTERNATE, nil
	case "parallel":
		return diagv1.FlowBranchMode_FLOW_BRANCH_MODE_PARALLEL, nil
	default:
		return 0, fmt.Errorf("--mode must be alternate or parallel")
	}
}
func newBranch() *cobra.Command {
	root := group("branch", "Manage alternate/parallel branches on flow steps")
	create := command("create <flow> <step-id>", "Create a branch with two to 26 named paths", 2, true, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		s, _, _, err := requireStep(f, args[1])
		if err != nil {
			return err
		}
		if s.Control != nil {
			return fmt.Errorf("step already has branches or a linked flow; remove that control first")
		}
		mode, err := branchMode(str(c, "mode"))
		if err != nil {
			return err
		}
		b := &diagv1.FlowBranches{Mode: mode}
		for _, name := range stringsFlag(c, "path") {
			b.Paths = append(b.Paths, &diagv1.FlowBranch{Id: uuid.NewString(), Name: name})
		}
		s.Control = &diagv1.FlowStep_Branches{Branches: b}
		return e.save(c, f, map[string]any{"paths": b.Paths})
	})
	create.Flags().String("mode", "", "alternate or parallel")
	create.Flags().StringArray("path", nil, "initial path display name; repeat at least twice")
	update := command("update <flow> <step-id>", "Change an existing branch mode", 2, true, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		s, _, _, err := requireStep(f, args[1])
		if err != nil {
			return err
		}
		b := s.GetBranches()
		if b == nil {
			return fmt.Errorf("step has no branches")
		}
		mode, err := branchMode(str(c, "mode"))
		if err != nil {
			return err
		}
		b.Mode = mode
		return e.save(c, f, nil)
	})
	update.Flags().String("mode", "", "alternate or parallel")
	del := command("delete <flow> <step-id>", "Remove branches and descendants, retaining the owning step", 2, true, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		s, _, _, err := requireStep(f, args[1])
		if err != nil {
			return err
		}
		b := s.GetBranches()
		if b == nil {
			return fmt.Errorf("step has no branches")
		}
		n := 0
		for _, p := range b.Paths {
			n += countSteps(p.Steps)
		}
		s.Control = nil
		return e.save(c, f, map[string]any{"deleted_steps": n})
	})
	root.AddCommand(create, update, del, newPath())
	return root
}
func newPath() *cobra.Command {
	root := group("path", "Manage branch paths by stable ID")
	add := command("add <flow> <step-id> <name>", "Add a named path to an existing branch", 3, true, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		s, _, _, err := requireStep(f, args[1])
		if err != nil {
			return err
		}
		b := s.GetBranches()
		if b == nil {
			return fmt.Errorf("step has no branches")
		}
		id := str(c, "id")
		if c.Flags().Changed("id") && id == "" {
			return fmt.Errorf("--id must be nonempty")
		}
		if id == "" {
			id = uuid.NewString()
		}
		p := &diagv1.FlowBranch{Id: id, Name: args[2]}
		ids := []string{}
		for _, path := range b.Paths {
			ids = append(ids, path.Id)
		}
		i, err := insertion(c, ids)
		if err != nil {
			return err
		}
		b.Paths = slices.Insert(b.Paths, i, p)
		return e.save(c, f, map[string]any{"path_id": id})
	})
	add.Flags().String("id", "", "stable path ID (default: generated UUID)")
	ordering(add, false)
	update := command("update <flow> <path-id>", "Rename a branch path", 2, true, func(c *cobra.Command, e *environment, args []string) error {
		if !c.Flags().Changed("name") {
			return fmt.Errorf("--name is required")
		}
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		p, _, _, err := requirePath(f, args[1])
		if err != nil {
			return err
		}
		p.Name = str(c, "name")
		return e.save(c, f, map[string]any{"path_id": p.Id})
	})
	update.Flags().String("name", "", "new path display name")
	move := command("move <flow> <path-id>", "Reorder a path within its branch", 2, true, func(c *cobra.Command, e *environment, args []string) error {
		if str(c, "before") == "" && str(c, "after") == "" {
			return fmt.Errorf("provide --before or --after")
		}
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		p, b, i, err := requirePath(f, args[1])
		if err != nil {
			return err
		}
		b.Paths = slices.Delete(b.Paths, i, i+1)
		ids := []string{}
		for _, other := range b.Paths {
			ids = append(ids, other.Id)
		}
		i, err = insertion(c, ids)
		if err != nil {
			return err
		}
		b.Paths = slices.Insert(b.Paths, i, p)
		return e.save(c, f, map[string]any{"path_id": p.Id})
	})
	ordering(move, false)
	del := command("delete <flow> <path-id>", "Delete a path and descendants; at least two paths must remain", 2, true, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		p, b, i, err := requirePath(f, args[1])
		if err != nil {
			return err
		}
		if len(b.Paths) <= 2 {
			return fmt.Errorf("branches require at least two paths; delete the branch to remove all paths")
		}
		b.Paths = slices.Delete(b.Paths, i, i+1)
		return e.save(c, f, map[string]any{"path_id": p.Id, "deleted_steps": countSteps(p.Steps)})
	})
	root.AddCommand(add, update, move, del)
	return root
}
