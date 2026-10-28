// Package flow provides live-target story authoring independent of workspace YAML.
package flow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/flowdoc"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

type environment struct {
	runner  exec.FlowRunner
	ws      *workspace.Workspace
	catalog flowdoc.Catalog
}

func NewFlowCmd() *cobra.Command {
	c := group("flow", "Create, inspect, and edit flows over live diagrams")
	c.Long = `Manage saved flows and their steps, branches, and portable YAML.

Flows always read and write the configured live target using global configuration.
Workspace YAML, --workspace, and --yaml do not affect these commands. Use --target
and --data-dir to select a target. Flow names must resolve uniquely; use a UUID or
--view to disambiguate. Views accept root, a numeric view ID, or a live owner ref/name.
Mutations save immediately; --dry-run validates without applying resource changes.
Step and path IDs are stable across reordering. Inspect prints the entire tree.`
	c.Example = `  tld flow create "Checkout" --view root
  tld flow step add Checkout --view root --id request --title "Receive request" --element api
  tld flow inspect Checkout --view root
  tld flow export Checkout --view root --complete -o checkout.flow.yaml
  tld flow import checkout.flow.yaml --new-view "Checkout demo" --dry-run`
	c.PersistentFlags().String("view", "", "scope to root, a live owner ref/name, or numeric view ID")
	_ = c.RegisterFlagCompletionFunc("view", completeView)
	c.AddCommand(newList(), newInspect(), newCreate(), newUpdate(), newDelete(), newImport(), newExport(), newStep(), newBranch())
	return c
}
func group(use, short string) *cobra.Command {
	return &cobra.Command{Use: use, Short: short, RunE: func(c *cobra.Command, args []string) error {
		if len(args) == 0 {
			return c.Help()
		}
		return cobra.NoArgs(c, args)
	}}
}

type handler func(*cobra.Command, *environment, []string) error

func command(use, short string, n int, mutation bool, run handler) *cobra.Command {
	c := &cobra.Command{Use: use, Short: short, Args: cobra.ExactArgs(n)}
	if mutation {
		c.Flags().Bool("dry-run", false, "validate and preview without applying resource changes")
		c.Flags().Int32("expected-revision", 0, "require this flow revision before applying the change")
	}
	c.RunE = func(cmd *cobra.Command, args []string) error {
		sess, err := cmdutil.OpenTargetSession(cmd)
		if err != nil {
			return err
		}
		defer func() { _ = sess.Close() }()
		runner, err := sess.Runner()
		if err != nil {
			return err
		}
		fr, ok := runner.(exec.FlowRunner)
		if !ok {
			return fmt.Errorf("target does not support flows")
		}
		ws, err := sess.LoadWorkspace()
		if err != nil {
			return err
		}
		views, err := runner.ListViews(cmd.Context())
		if err != nil {
			return err
		}
		flows, err := fr.ListFlows(cmd.Context(), 0)
		if err != nil {
			return flowError(err)
		}
		env := &environment{runner: fr, ws: ws, catalog: flowdoc.Catalog{Flows: flows, Views: views}}
		return flowError(run(cmd, env, args))
	}
	c.ValidArgsFunction = completeFlow
	return c
}
func str(c *cobra.Command, name string) string { v, _ := c.Flags().GetString(name); return v }
func flag(c *cobra.Command, name string) bool  { v, _ := c.Flags().GetBool(name); return v }
func stringsFlag(c *cobra.Command, name string) []string {
	v, _ := c.Flags().GetStringArray(name)
	return v
}
func flowError(err error) error {
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) == connect.CodeAborted {
		return cmdutil.WithCode(err, "conflict", "Run 'tld flow inspect' for the current revision, review the changes, and retry.")
	}
	return err
}
func (e *environment) view(arg string) (int32, error) {
	if arg == "" {
		return 0, nil
	}
	if arg == "root" {
		for _, v := range e.catalog.Views {
			if v.OwnerElementId == nil && v.ParentViewId == nil {
				return v.Id, nil
			}
		}
	}
	if id, err := strconv.Atoi(arg); err == nil && id > 0 {
		for _, v := range e.catalog.Views {
			if int(v.Id) == id {
				return v.Id, nil
			}
		}
	}
	ref, err := cmdutil.ResolveViewArg(e.ws, arg)
	if err == nil && e.ws.Meta != nil {
		if m := e.ws.Meta.Views[ref]; m != nil {
			return int32(m.ID), nil
		}
	}
	matches := []int32{}
	for _, v := range e.catalog.Views {
		if v.Name == arg {
			matches = append(matches, v.Id)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return 0, cmdutil.WithCode(fmt.Errorf("view %q is missing or ambiguous", arg), "invalid_value", "Use root, a unique live owner ref/name, or a numeric view ID.")
}
func (e *environment) selectFlow(c *cobra.Command, arg string) (*diagv1.Flow, error) {
	view, err := e.view(str(c, "view"))
	if err != nil {
		return nil, err
	}
	return e.findFlow(arg, view)
}
func (e *environment) findFlow(arg string, view int32) (*diagv1.Flow, error) {
	matches := []*diagv1.Flow{}
	for _, f := range e.catalog.Flows {
		if (view == 0 || f.ViewId == view) && f.Id == arg {
			return f, nil
		}
	}
	for _, f := range e.catalog.Flows {
		if (view == 0 || f.ViewId == view) && f.GetContent().GetName() == arg {
			matches = append(matches, f)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	code, message := "not_found", "not found"
	if len(matches) > 1 {
		code, message = "ambiguous_selector", "ambiguous"
	}
	return nil, cmdutil.WithCode(fmt.Errorf("flow %q %s", arg, message), code, "Run 'tld flow list' and use a UUID or narrow the selection with --view.")
}
func checkRevision(c *cobra.Command, f *diagv1.Flow) error {
	n, _ := c.Flags().GetInt32("expected-revision")
	if c.Flags().Changed("expected-revision") && (n <= 0 || n != f.Revision) {
		return cmdutil.WithCode(fmt.Errorf("flow revision is %d, expected %d", f.Revision, n), "conflict", "Inspect the flow and retry with its current revision.")
	}
	return nil
}
func (e *environment) save(c *cobra.Command, f *diagv1.Flow, extra map[string]any) error {
	if f.Id != "" {
		if err := checkRevision(c, f); err != nil {
			return err
		}
	}
	if err := flowdoc.ValidateSave(f); err != nil {
		return cmdutil.WithCode(err, "invalid_value", "Correct the flow fields and retry; use --help for supported flags.")
	}
	warnings, err := e.warnings(c, f)
	if err != nil {
		return err
	}
	if !flag(c, "dry-run") {
		saved, err := e.runner.SaveFlow(c.Context(), &diagv1.SaveFlowRequest{Id: f.Id, ViewId: f.ViewId, ExpectedRevision: f.Revision, Content: f.Content})
		if err != nil {
			return err
		}
		f = saved
	}
	return report(c, f, extra, warnings)
}
func flowDTO(f *diagv1.Flow) map[string]any {
	data, _ := (protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}).Marshal(f)
	result := map[string]any{}
	_ = json.Unmarshal(data, &result)
	return result
}
func report(c *cobra.Command, f *diagv1.Flow, extra map[string]any, warnings []string) error {
	if extra == nil {
		extra = map[string]any{}
	}
	extra["flow"] = flowDTO(f)
	command := strings.TrimPrefix(c.CommandPath(), "tld ")
	if cmdutil.WantsJSONFromCmd(c) {
		return cmdutil.WriteJSON(c.OutOrStdout(), cmdutil.CompactFromCmd(c), cmdutil.JSONOutput{Command: command, Status: "ok", DryRun: flag(c, "dry-run"), Warnings: warnings, Items: []cmdutil.JSONItem{{Ref: f.Id, ResourceType: "flow", Action: strings.TrimPrefix(command, "flow "), Name: f.GetContent().GetName()}}, Extra: extra})
	}
	for _, w := range warnings {
		term.Warnf(c.ErrOrStderr(), "Warning: %s", w)
	}
	prefix := ""
	if flag(c, "dry-run") {
		prefix = "dry-run: "
	}
	term.Successf(c.OutOrStdout(), "%s%s: %s (id=%s, view=%d, revision=%d, steps=%d)", prefix, command, f.GetContent().GetName(), f.Id, f.ViewId, f.Revision, countSteps(f.GetContent().GetSteps()))
	if name, ok := extra["new_view"].(string); ok {
		term.Infof(c.OutOrStdout(), "new view: %s (planned)", name)
	}
	if n, ok := extra["elements"]; ok {
		term.Infof(c.OutOrStdout(), "planned elements: %v; connectors: %v", n, extra["connectors"])
	}
	if id, ok := extra["step_id"]; ok {
		term.Infof(c.OutOrStdout(), "step id: %v", id)
	}
	if id, ok := extra["path_id"]; ok {
		term.Infof(c.OutOrStdout(), "path id: %v", id)
	}
	if paths, ok := extra["paths"].([]*diagv1.FlowBranch); ok {
		for _, p := range paths {
			term.Infof(c.OutOrStdout(), "path: %s (id=%s)", p.Name, p.Id)
		}
	}
	if n, ok := extra["deleted_steps"]; ok {
		term.Infof(c.OutOrStdout(), "deleted steps: %v", n)
	}
	return nil
}
func newList() *cobra.Command {
	c := command("list", "List saved flows", 0, false, func(c *cobra.Command, e *environment, _ []string) error {
		view, err := e.view(str(c, "view"))
		if err != nil {
			return err
		}
		search := strings.ToLower(str(c, "search"))
		flows := []*diagv1.Flow{}
		for _, f := range e.catalog.Flows {
			if (view == 0 || view == f.ViewId) && strings.Contains(strings.ToLower(f.Content.Name+" "+f.Content.Description), search) {
				flows = append(flows, f)
			}
		}
		sort.Slice(flows, func(i, j int) bool {
			if flows[i].ViewId != flows[j].ViewId {
				return flows[i].ViewId < flows[j].ViewId
			}
			if flows[i].Content.Name != flows[j].Content.Name {
				return flows[i].Content.Name < flows[j].Content.Name
			}
			return flows[i].Id < flows[j].Id
		})
		if cmdutil.WantsJSONFromCmd(c) {
			rows := []map[string]any{}
			for _, f := range flows {
				rows = append(rows, flowDTO(f))
			}
			return cmdutil.WriteJSON(c.OutOrStdout(), cmdutil.CompactFromCmd(c), cmdutil.JSONOutput{Command: "flow list", Status: "ok", Summary: map[string]int{"count": len(flows), "total": len(e.catalog.Flows)}, Extra: map[string]any{"flows": rows}})
		}
		w := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "ID\tNAME\tVIEW\tREVISION\tSTEPS")
		for _, f := range flows {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%d\n", f.Id, f.Content.Name, f.ViewId, f.Revision, countSteps(f.Content.Steps))
		}
		if err = w.Flush(); err != nil {
			return err
		}
		term.Infof(c.OutOrStdout(), "%d flow(s)", len(flows))
		return nil
	})
	c.Flags().String("search", "", "substring filter across flow name and description")
	return c
}
func newInspect() *cobra.Command {
	return command("inspect <flow>", "Inspect a flow and its complete step tree", 1, false, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		warnings, err := e.warnings(c, f)
		if err != nil {
			return err
		}
		if err = report(c, f, nil, warnings); err != nil {
			return err
		}
		if !cmdutil.WantsJSONFromCmd(c) {
			term.Infof(c.OutOrStdout(), "description: %s", f.Content.Description)
			printTree(c, f.Content.Steps, "")
		}
		return nil
	})
}
func newCreate() *cobra.Command {
	c := command("create <name>", "Create an empty saved flow in an existing view", 1, true, func(c *cobra.Command, e *environment, args []string) error {
		if str(c, "view") == "" {
			return fmt.Errorf("--view is required")
		}
		if c.Flags().Changed("expected-revision") {
			return fmt.Errorf("--expected-revision applies only to existing flows")
		}
		view, err := e.view(str(c, "view"))
		if err != nil {
			return err
		}
		return e.save(c, &diagv1.Flow{ViewId: view, Content: &diagv1.FlowContent{Name: args[0], Description: str(c, "description")}}, nil)
	})
	c.Flags().String("description", "", "flow description")
	return c
}
func newUpdate() *cobra.Command {
	c := command("update <flow>", "Update flow metadata while preserving steps", 1, true, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		if !c.Flags().Changed("name") && !c.Flags().Changed("description") {
			return fmt.Errorf("provide --name or --description")
		}
		if c.Flags().Changed("name") {
			f.Content.Name = str(c, "name")
		}
		if c.Flags().Changed("description") {
			f.Content.Description = str(c, "description")
		}
		return e.save(c, f, nil)
	})
	c.Flags().String("name", "", "new flow name")
	c.Flags().String("description", "", "flow description (empty clears it)")
	return c
}
func newDelete() *cobra.Command {
	return command("delete <flow>", "Delete a flow while retaining diagram resources", 1, true, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		if err = checkRevision(c, f); err != nil {
			return err
		}
		warnings := []string{}
		for _, other := range e.catalog.Flows {
			if other.Id == f.Id {
				continue
			}
			walk(other.GetContent().GetSteps(), func(s *diagv1.FlowStep) {
				if s.GetGoToFlowId() == f.Id {
					warnings = append(warnings, fmt.Sprintf("flow %q step %s links to this flow and will become unavailable", other.Content.Name, s.Id))
				}
			})
		}
		if !flag(c, "dry-run") {
			if err = e.runner.DeleteFlow(c.Context(), &diagv1.DeleteFlowRequest{Id: f.Id, ViewId: f.ViewId, ExpectedRevision: f.Revision}); err != nil {
				return err
			}
		}
		return report(c, f, map[string]any{"deleted_steps": countSteps(f.Content.Steps)}, warnings)
	})
}
func (e *environment) warnings(c *cobra.Command, f *diagv1.Flow) ([]string, error) {
	diagram, err := e.runner.ViewContent(c.Context(), f.ViewId)
	if err != nil {
		return nil, err
	}
	elements := map[int32]bool{}
	connectors := map[int32]bool{}
	flows := map[string]bool{}
	for _, p := range diagram.Placements {
		elements[p.ElementId] = true
	}
	for _, x := range diagram.Connectors {
		connectors[x.Id] = true
	}
	for _, x := range e.catalog.Flows {
		flows[x.Id] = true
	}
	warnings := []string{}
	walk(f.GetContent().GetSteps(), func(s *diagv1.FlowStep) {
		for _, id := range s.ElementIds {
			if !elements[id] {
				warnings = append(warnings, fmt.Sprintf("step %s: element %d is unavailable", s.Id, id))
			}
		}
		for _, id := range s.ConnectorIds {
			if !connectors[id] {
				warnings = append(warnings, fmt.Sprintf("step %s: connector %d is unavailable", s.Id, id))
			}
		}
		if id := s.GetGoToFlowId(); id != "" && !flows[id] {
			warnings = append(warnings, fmt.Sprintf("step %s: linked flow %s is unavailable", s.Id, id))
		}
	})
	return warnings, nil
}
func printTree(c *cobra.Command, steps []*diagv1.FlowStep, indent string) {
	for _, s := range steps {
		term.Infof(c.OutOrStdout(), "%s- %s (id=%s, elements=%v, connectors=%v)", indent, s.Title, s.Id, s.ElementIds, s.ConnectorIds)
		if s.Description != "" {
			term.Infof(c.OutOrStdout(), "%s  %s", indent, s.Description)
		}
		if id := s.GetGoToFlowId(); id != "" {
			term.Infof(c.OutOrStdout(), "%s  go to: %s", indent, id)
		}
		if b := s.GetBranches(); b != nil {
			term.Infof(c.OutOrStdout(), "%s  branches: %s", indent, b.Mode.String())
			for _, p := range b.Paths {
				term.Infof(c.OutOrStdout(), "%s  path: %s (id=%s)", indent, p.Name, p.Id)
				printTree(c, p.Steps, indent+"    ")
			}
		}
	}
}

// Shell completions read live target data through the same workspace-free session.
func complete(c *cobra.Command, views bool) ([]string, cobra.ShellCompDirective) {
	sess, err := cmdutil.OpenTargetSession(c)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	defer func() { _ = sess.Close() }()
	r, err := sess.Runner()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	out := []string{}
	if views {
		vs, err := r.ListViews(c.Context())
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		out = append(out, "root")
		for _, v := range vs {
			out = append(out, fmt.Sprint(v.Id)+"\t"+v.Name)
		}
	} else {
		fr, ok := r.(exec.FlowRunner)
		if !ok {
			return nil, cobra.ShellCompDirectiveError
		}
		fs, err := fr.ListFlows(c.Context(), 0)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		for _, f := range fs {
			out = append(out, f.Id+"\t"+f.GetContent().GetName())
		}
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
func completeFlow(c *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return complete(c, false)
}
func completeView(c *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return complete(c, true)
}
