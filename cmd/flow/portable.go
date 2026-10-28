package flow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/flowdoc"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"
)

func newImport() *cobra.Command {
	c := command("import <file|->", "Import portable flow YAML; optionally create a complete diagram", 1, true, runImport)
	c.Long = `Import tld-flow version 1 YAML (or JSON); use - for stdin.

--view resolves highlights against an existing live diagram. Embedded resources
are ignored with a warning. Import creates a new saved flow unless --replace is
supplied, which replaces the selected flow's complete content.

--new-view requires a complete bundle and creates fresh resources in a new
owner element's view under Root. Linked flows are not bundled and must resolve
by name and explicit view. Resources and flow use separate existing RPC calls;
on a definite save failure cleanup is attempted. Interrupted imports may leave
resources behind. Errors report recovery information when target writes remain.

--schema prints the accepted schema without opening a target.`
	c.Example = `  tld flow import checkout.flow.yaml --view root --dry-run
  tld flow import checkout.flow.yaml --view root --replace Checkout
  tld flow import checkout.complete.yaml --new-view "Checkout demo"
  cat checkout.flow.yaml | tld flow import - --view root
  tld flow import --schema`
	c.Flags().String("new-view", "", "create a new element-owned diagram with this name from bundled resources")
	c.Flags().String("replace", "", "replace the selected existing flow instead of creating a flow")
	c.Flags().Bool("schema", false, "print the accepted JSON schema without a file or target")
	c.Args = func(c *cobra.Command, args []string) error {
		if flag(c, "schema") {
			return cobra.NoArgs(c, args)
		}
		return cobra.ExactArgs(1)(c, args)
	}
	run := c.RunE
	c.RunE = func(c *cobra.Command, args []string) error {
		if flag(c, "schema") {
			data, err := assets.FS.ReadFile("schemas/flow.schema.json")
			if err != nil {
				return err
			}
			_, err = c.OutOrStdout().Write(data)
			return err
		}
		return run(c, args)
	}
	c.ValidArgsFunction = nil
	return c
}
func readDocument(c *cobra.Command, file string) (*flowdoc.Document, error) {
	input := c.InOrStdin()
	if file != "-" {
		f, err := os.Open(file)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		input = f
	}
	data, err := io.ReadAll(io.LimitReader(input, flowdoc.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	return flowdoc.Parse(data)
}
func runImport(c *cobra.Command, e *environment, args []string) error {
	doc, err := readDocument(c, args[0])
	if err != nil {
		return err
	}
	newView, view, replace := str(c, "new-view"), str(c, "view"), str(c, "replace")
	if c.Flags().Changed("new-view") && strings.TrimSpace(newView) == "" {
		return fmt.Errorf("--new-view requires a nonempty name")
	}
	if newView != "" {
		if view != "" || replace != "" || c.Flags().Changed("expected-revision") {
			return fmt.Errorf("--new-view cannot be combined with --view, --replace, or --expected-revision")
		}
		return importComplete(c, e, doc, newView)
	}
	if view == "" {
		return fmt.Errorf("provide --view for an existing diagram or --new-view for a complete bundle")
	}
	viewID, err := e.view(view)
	if err != nil {
		return err
	}
	var original *diagv1.Flow
	if replace != "" {
		original, err = e.findFlow(replace, viewID)
		if err != nil {
			return err
		}
		if err = checkRevision(c, original); err != nil {
			return err
		}
	} else if c.Flags().Changed("expected-revision") {
		return fmt.Errorf("--expected-revision requires --replace")
	}
	diagram, err := e.runner.ViewContent(c.Context(), viewID)
	if err != nil {
		return err
	}
	f, err := flowdoc.Import(doc, viewID, diagram, e.catalog)
	if err != nil {
		return err
	}
	if original != nil {
		f.Id = original.Id
		f.Revision = original.Revision
	}
	if doc.Resources != nil {
		warning := "bundled resource definitions are unused with --view; highlights resolve against the existing diagram"
		if cmdutil.WantsJSONFromCmd(c) {
			return saveImport(c, e, f, []string{warning})
		}
		term.Warnf(c.ErrOrStderr(), "Warning: %s", warning)
	}
	return e.save(c, f, nil)
}
func saveImport(c *cobra.Command, e *environment, f *diagv1.Flow, warnings []string) error {
	if err := flowdoc.ValidateSave(f); err != nil {
		return err
	}
	if !flag(c, "dry-run") {
		saved, err := e.runner.SaveFlow(c.Context(), &diagv1.SaveFlowRequest{Id: f.Id, ViewId: f.ViewId, ExpectedRevision: f.Revision, Content: f.Content})
		if err != nil {
			return err
		}
		f = saved
	}
	return report(c, f, nil, warnings)
}
func newExport() *cobra.Command {
	c := command("export <flow>", "Export portable YAML to stdout or a file", 1, false, func(c *cobra.Command, e *environment, args []string) error {
		f, err := e.selectFlow(c, args[0])
		if err != nil {
			return err
		}
		diagram, err := e.runner.ViewContent(c.Context(), f.ViewId)
		if err != nil {
			return err
		}
		data, err := flowdoc.Export(f, diagram, e.catalog, flag(c, "complete"))
		if err != nil {
			return err
		}
		warnings := []string{}
		if flag(c, "complete") {
			linked := false
			walk(f.Content.Steps, func(s *diagv1.FlowStep) { linked = linked || s.GetGoToFlowId() != "" })
			if linked {
				warnings = append(warnings, "linked flows are not included; they must exist in the destination and same-view links need a view when importing into a new diagram")
			}
		}
		output := str(c, "output")
		if output != "" {
			if err = os.WriteFile(output, data, 0600); err != nil {
				return fmt.Errorf("write output: %w", err)
			}
		}
		if cmdutil.WantsJSONFromCmd(c) {
			return report(c, f, map[string]any{"content": string(data), "output": output, "complete": flag(c, "complete")}, warnings)
		}
		for _, w := range warnings {
			term.Warnf(c.ErrOrStderr(), "Warning: %s", w)
		}
		if output != "" {
			term.Successf(c.OutOrStdout(), "wrote %s", output)
			return nil
		}
		_, err = c.OutOrStdout().Write(data)
		return err
	})
	c.Flags().Bool("complete", false, "include highlighted resources and connector endpoints")
	c.Flags().StringP("output", "o", "", "write YAML to a file instead of stdout")
	return c
}
func bundleBatch(doc *flowdoc.Document, name string) *diagv1.ApplyWorkspaceBatchRequest {
	req := &diagv1.ApplyWorkspaceBatchRequest{Elements: []*diagv1.BatchElement{{Ref: "flow-owner", Name: name, Description: proto.String(doc.Description), HasView: true, Placements: []*diagv1.BatchViewPlacement{{ParentRef: "root"}}}}}
	refs := map[string]string{}
	for i, e := range doc.Resources.Elements {
		ref := fmt.Sprintf("element-%d", i)
		refs[e.Name] = ref
		item := &diagv1.BatchElement{Ref: ref, Name: e.Name, Kind: proto.String(e.Kind), Description: proto.String(e.Description), Technology: proto.String(e.Technology), Url: proto.String(e.URL), LogoUrl: proto.String(e.LogoURL), Tags: e.Tags, Placements: []*diagv1.BatchViewPlacement{{ParentRef: "flow-owner", PositionX: e.X, PositionY: e.Y}}}
		for _, link := range e.TechnologyConnectors {
			item.TechnologyLinks = append(item.TechnologyLinks, &diagv1.TechnologyLink{Type: link.Type, Label: link.Label, Slug: proto.String(link.Slug), IsPrimaryIcon: link.Primary})
		}
		req.Elements = append(req.Elements, item)
	}
	for i, c := range doc.Resources.Connectors {
		req.Connectors = append(req.Connectors, &diagv1.BatchConnector{Ref: fmt.Sprintf("connector-%d", i), ViewRef: "flow-owner", SourceElementRef: refs[c.Source], TargetElementRef: refs[c.Target], Label: c.Label, Description: proto.String(c.Description), Relationship: proto.String(c.Relationship), Direction: optional(c.Direction), Style: optional(c.Style), Url: proto.String(c.URL), SourceHandle: proto.String(c.SourceHandle), TargetHandle: proto.String(c.TargetHandle), Tags: c.Tags})
	}
	return req
}
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
func importComplete(c *cobra.Command, e *environment, doc *flowdoc.Document, name string) error {
	if doc.Resources == nil {
		return fmt.Errorf("--new-view requires bundled elements and connectors; export with --complete")
	}
	// A positive, nonexistent preview view prevents unqualified links from binding
	// to a different diagram before the new home view exists.
	const previewView int32 = 2147483647
	preview, err := flowdoc.Import(doc, previewView, flowdoc.PreviewResources(doc.Resources, previewView), e.catalog)
	if err != nil {
		return err
	}
	batch := bundleBatch(doc, name)
	if err = api.ValidateWorkspaceBatchRequest(batch); err != nil {
		return err
	}

	if flag(c, "dry-run") {
		preview.ViewId = 0
		return report(c, preview, map[string]any{"new_view": name, "elements": len(doc.Resources.Elements) + 1, "connectors": len(doc.Resources.Connectors)}, nil)
	}
	applied, err := e.runner.ApplyBatch(c.Context(), batch)
	if err != nil {
		if uncertain(err) {
			return cmdutil.WithCode(err, "import_outcome_unknown", "The resource batch may have committed. Inspect live elements/views before retrying; no cleanup was attempted without confirmed IDs.")
		}
		return err
	}
	meta := applied.ViewMetadata["flow-owner"]
	if meta == nil || meta.Id <= 0 {
		return partialImport(fmt.Errorf("batch response omitted the new view ID"), applied, "Inspect the returned/live resource IDs before retrying; no flow was saved.")
	}
	viewID := meta.Id
	diagram, err := e.runner.ViewContent(c.Context(), viewID)
	if err == nil {
		preview, err = flowdoc.Import(doc, viewID, diagram, e.catalog)
	}
	if err != nil {
		return cleanupImport(c, e, applied, viewID, err)
	}
	saved, err := e.runner.SaveFlow(c.Context(), &diagv1.SaveFlowRequest{ViewId: viewID, Content: preview.Content})
	if err != nil {
		if uncertain(err) {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Context()), 10*time.Second)
			defer cancel()
			flows, readErr := e.runner.ListFlows(ctx, viewID)
			if readErr == nil {
				for _, f := range flows {
					if proto.Equal(f.Content, preview.Content) {
						return report(c, f, map[string]any{"new_view": viewID}, []string{"save response was interrupted; the saved flow was verified on the target"})
					}
				}
			}
			// A successful read with no flow does not prove an in-flight save won't
			// commit later. Keep the resources in either case.
			return partialImport(err, applied, fmt.Sprintf("Save outcome is unknown. Inspect 'tld flow list --view %d' before retrying. Resources were retained to avoid deleting a possibly saved flow.", viewID))
		}
		return cleanupImport(c, e, applied, viewID, err)
	}
	return report(c, saved, map[string]any{"new_view": viewID}, nil)
}
func uncertain(err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodeUnavailable, connect.CodeDeadlineExceeded, connect.CodeCanceled, connect.CodeUnknown, connect.CodeInternal:
		return true
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
func partialImport(cause error, resp *diagv1.ApplyWorkspaceBatchResponse, hint string) error {
	elementIDs, viewIDs := []int32{}, []int32{}
	for _, m := range resp.ElementMetadata {
		elementIDs = append(elementIDs, m.Id)
	}
	for _, m := range resp.ViewMetadata {
		viewIDs = append(viewIDs, m.Id)
	}
	sort.Slice(elementIDs, func(i, j int) bool { return elementIDs[i] < elementIDs[j] })
	sort.Slice(viewIDs, func(i, j int) bool { return viewIDs[i] < viewIDs[j] })
	return &cmdutil.CommandError{Cause: fmt.Errorf("resource batch applied, but flow import did not complete (element IDs %v, view IDs %v): %w", elementIDs, viewIDs, cause), Code: "partial_import", Applied: true, Hint: hint}

}
func cleanupImport(c *cobra.Command, e *environment, resp *diagv1.ApplyWorkspaceBatchResponse, viewID int32, cause error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Context()), 15*time.Second)
	defer cancel()
	// Delete only IDs returned from our fresh batch. Retain the owner if its
	// view cannot be removed so recovery can still address the owned view.
	failures := []string{}
	ownerID := int32(0)
	if owner := resp.ElementMetadata["flow-owner"]; owner != nil {
		ownerID = owner.Id
	}
	for _, el := range resp.CreatedElements {
		if el.Id == ownerID {
			continue
		}
		if err := e.runner.DeleteElement(ctx, el.Id); err != nil && !exec.IsNotFound(err) {
			failures = append(failures, fmt.Sprintf("element %d: %v", el.Id, err))
		}
	}
	viewRemoved := true
	if err := e.runner.DeleteView(ctx, viewID); err != nil && !exec.IsNotFound(err) {
		viewRemoved = false
		failures = append(failures, fmt.Sprintf("view %d: %v", viewID, err))
	}
	if viewRemoved && ownerID != 0 {
		if err := e.runner.DeleteElement(ctx, ownerID); err != nil && !exec.IsNotFound(err) {
			failures = append(failures, fmt.Sprintf("element %d: %v", ownerID, err))
		}
	}
	if len(failures) > 0 {
		return partialImport(cause, resp, "Cleanup failed for "+strings.Join(failures, "; ")+". On the same target, remove remaining resource elements with 'tld remove element <id>', then remove the owner element (ID "+fmt.Sprint(ownerID)+") and its view.")
	}
	return fmt.Errorf("flow import failed; newly created diagram resources were removed: %w", cause)
}
