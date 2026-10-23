package importcmd

import (
	"bytes"
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
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// importDocument is the accepted bulk-import file shape. It mirrors the
// workspace files: `elements` is a ref-keyed map and `connectors` is a list.
// JSON is accepted too because it is a subset of YAML.
type importDocument struct {
	Elements   map[string]*workspace.Element `yaml:"elements"`
	Connectors []*workspace.Connector        `yaml:"connectors"`
}

// NewImportCmd builds `tld import <file>`: a validated, atomic bulk upsert of
// elements and connectors.
func NewImportCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		target  string
		dataDir string
		dryRun  bool
		schema  bool
	)

	c := &cobra.Command{
		Use:   "import <file>",
		Short: "Bulk add or update elements and connectors from a file",
		Long: fmt.Sprintf(`Import many elements and connectors in one atomic operation.

The file is YAML or JSON shaped like the workspace files:

  elements:
    api:
      name: API
      kind: service
      placements:
        - parent: root
    db:
      name: Database
      kind: database
  connectors:
    - view: root
      source: api
      target: db
      label: reads

Existing elements/views/connectors are matched and updated, so re-running the
same file is safe. The whole batch is applied in a single transaction: if any
part fails, nothing is committed. Use "-" to read from stdin.

Use --schema to print the exact accepted import schema without network access.
Omitted resources, metadata, placements, and coordinates are preserved.
An import adds placements; it does not move elements or remove old placements.
Use 'tld view unplace <view> <element>' to remove a placement, or 'tld view place
<destination> <element> --from <old-view>' to move one. Shared placements can copy
existing relationships into the destination view; these appear in the refreshed
cache and live listings. Changing a connector's label,
view, or endpoints creates another connector; use 'update connector' to edit it.
Unapplied YAML edits must be synced before a write refreshes the cache; use
'import --dry-run' to preview without discarding them.
Connector tags require 'update connector'; the batch API cannot apply them.

Workspace schemas:
  elements:   %s
  connectors: %s`, workspace.ElementsSchemaURL, workspace.ConnectorsSchemaURL),
		Args: func(cmd *cobra.Command, args []string) error {
			if schema {
				return cobra.NoArgs(cmd, args)
			}
			return cobra.ExactArgs(1)(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if schema {
				data, err := assets.FS.ReadFile("schemas/import.schema.json")
				if err != nil {
					return err
				}
				_, err = cmd.OutOrStdout().Write(data)
				return err
			}
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				return err
			}
			defer func() { _ = sess.Close() }()
			return runImport(cmd, sess, *format, *compact, args[0], dryRun)
		},
	}

	c.Flags().BoolVar(&dryRun, "dry-run", false, "validate and report changes without applying them")
	c.Flags().BoolVar(&schema, "schema", false, "print the accepted JSON schema (no file or target needed)")
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, remote, or cloud")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}

func runImport(cmd *cobra.Command, sess *cmdutil.Session, format string, compact bool, file string, dryRun bool) error {
	fail := func(err error) error {
		if cmdutil.WantsJSON(format) {
			return cmdutil.WriteCommandError(cmd.OutOrStdout(), compact, "import", err)
		}
		return err
	}

	doc, err := readImportDocument(file, cmd.InOrStdin())
	if err != nil {
		return fail(err)
	}

	ws, err := sess.LoadWorkspace()
	if err != nil {
		return fail(err)
	}

	if !dryRun {
		if err := sess.RequireAppliedCache(); err != nil {
			return fail(err)
		}
	}
	runner, err := sess.Runner()
	if err != nil {
		return fail(err)
	}

	ctx := sess.Context(cmd.Context())
	batch, err := exec.BuildImportBatch(ctx, runner, ws, doc.Elements, doc.Connectors)
	if err != nil {
		return fail(fmt.Errorf("import aborted before any changes were made: %w", err))
	}

	setImportDiagnostics(batch, ws, doc.Elements)
	if dryRun {
		return reportImport(cmd, format, compact, "import", "dry-run", batch)
	}

	resp, err := runner.ApplyBatch(ctx, batch.Request)
	if err != nil {
		return fail(explainApplyError(err))
	}

	if sess.HasWorkspace() {
		actual, err := runner.ExportWorkspace(ctx)
		if err != nil {
			return fail(sess.CacheError(err))
		}
		if err := persistImportCache(sess.Wdir, ws, batch, resp, actual); err != nil {
			return fail(sess.CacheError(err))
		}
	}

	if sess.HasWorkspace() {
		batch.Views = ws.ViewDiagnostics()
	}
	return reportImport(cmd, format, compact, "import", "ok", batch, resp)
}

func readImportDocument(path string, stdin io.Reader) (*importDocument, error) {
	label := path
	var (
		data []byte
		err  error
	)
	if path == "-" {
		label = "stdin"
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read import file %s: %w", label, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, cmdutil.WithCode(fmt.Errorf("import file %s is empty", label), "invalid_document", "Add an elements map or connectors list; run 'tld import --schema' for accepted fields.")
	}

	var doc importDocument
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&doc); err != nil {
		return nil, cmdutil.WithCode(fmt.Errorf("parse import file %s: %w", label, err), "invalid_document", strings.TrimSpace(schemaHint())+" Use block YAML or quote descriptions containing commas in {flow mappings}; inspect the reported line for accidental keys.")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, cmdutil.WithCode(fmt.Errorf("import file %s must contain one document", label), "invalid_document", "Combine elements and connectors in one mapping.")
	}
	if len(doc.Elements) == 0 && len(doc.Connectors) == 0 {
		return nil, cmdutil.WithCode(fmt.Errorf("import file %s defines no elements or connectors", label), "invalid_document", "Add top-level elements and/or connectors; run 'tld import --schema' for their shape.")
	}
	// Workspace YAML carries cache-only fields that the batch API cannot apply.
	// Refuse them instead of reporting success and writing misleading cache data.
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return nil, err
	}
	if len(node.Content) > 0 {
		sections := importMappingEntries(node.Content[0])
		for _, section := range []string{"elements", "connectors"} {
			entries := sections[section]
			if entries == nil {
				continue
			}
			for entries.Kind == yaml.AliasNode {
				entries = entries.Alias
			}
			if section == "elements" {
				for ref, spec := range importMappingEntries(entries) {
					for _, field := range importMappingFields(spec) {
						if field == "density_level" {
							doc.Elements[ref].DensityLevelSet = true
						}
						if field == "owner" || field == "symbol" || field == "view_name" {
							hint := "Use file_path with a path#symbol anchor for source links."
							if field == "view_name" {
								hint = fmt.Sprintf("Use 'tld view rename %s <name>' after creating the view.", ref)
							}
							if field == "owner" {
								hint = "Keep repository ownership in workspace YAML; use repo or repository_id for target metadata."
							}
							return nil, cmdutil.WithCode(fmt.Errorf("element %q: import does not support %s", ref, field), "unsupported_field", hint)
						}
					}
				}
			}
			if section == "connectors" {
				for index, spec := range entries.Content {
					for _, field := range importMappingFields(spec) {
						if field == "visibility_delta" {
							doc.Connectors[index].VisibilityDeltaSet = true
						}
						if field == "tags" || field == "id" || field == "updated_at" {
							hint := "Omit cache IDs and timestamps; import resolves existing resources automatically."
							if field == "tags" {
								hint = "Omit connector tags from the import; set them with 'tld update connector <key-or-id> tags <comma-separated-tags>'."
							}
							return nil, cmdutil.WithCode(fmt.Errorf("connector import does not support %s", field), "unsupported_field", hint)
						}
					}
				}
			}
		}
	}
	return &doc, nil
}

// Resolve YAML aliases and merge keys before checking accepted fields.
func importMappingEntries(node *yaml.Node) map[string]*yaml.Node {
	if node.Kind == yaml.AliasNode {
		return importMappingEntries(node.Alias)
	}
	entries := map[string]*yaml.Node{}
	if node.Kind == yaml.SequenceNode {
		for _, child := range node.Content {
			for key, value := range importMappingEntries(child) {
				if entries[key] == nil {
					entries[key] = value
				}
			}
		}
		return entries
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == "<<" {
			for key, value := range importMappingEntries(node.Content[i+1]) {
				if entries[key] == nil {
					entries[key] = value
				}
			}
		}
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != "<<" {
			entries[node.Content[i].Value] = node.Content[i+1]
		}
	}
	return entries
}

func importMappingFields(node *yaml.Node) []string {
	var fields []string
	for key := range importMappingEntries(node) {
		fields = append(fields, key)
	}
	sort.Strings(fields)
	return fields
}

// schemaHint directs agents to the bundled import schema.
func schemaHint() string {
	return "\nRun 'tld import --schema' for the accepted fields and document shape."
}

// explainApplyError turns a failed apply into an actionable reason. The apply is
// transactional, so a failure means nothing was committed.
func explainApplyError(err error) error {
	wrapped := cmdutil.WithUnauthorizedHint("import failed", err)
	var connectErr *connect.Error
	if errors.As(wrapped, &connectErr) {
		switch connectErr.Code() {
		case connect.CodeInvalidArgument:
			return fmt.Errorf("import rejected by the target as invalid: %w\nNo changes were committed", connectErr)
		case connect.CodePermissionDenied:
			return fmt.Errorf("import denied by the target: %w\nNo changes were committed", connectErr)
		case connect.CodeFailedPrecondition:
			return fmt.Errorf("import cannot proceed on the target: %w\nNo changes were committed", connectErr)
		}
	}
	return fmt.Errorf("import failed and was rolled back; no changes were committed: %w", wrapped)
}

// persistImportCache writes the successful batch back into the local YAML cache,
// mirroring the write-through behavior of `add`/`connect`. The target is always
// the source of truth: if this fails, the caller reports that a `tld pull` is
// needed rather than treating the import as failed.
func persistImportCache(wdir string, ws *workspace.Workspace, batch *exec.ImportBatch, resp *diagv1.ApplyWorkspaceBatchResponse, exported *diagv1.ExportOrganizationResponse) error {
	if ws.Elements == nil {
		ws.Elements = map[string]*workspace.Element{}
	}
	if ws.Connectors == nil {
		ws.Connectors = map[string]*workspace.Connector{}
	}
	if ws.Meta == nil {
		ws.Meta = &workspace.Meta{
			Elements:   map[string]*workspace.ResourceMetadata{},
			Views:      map[string]*workspace.ResourceMetadata{},
			Connectors: map[string]*workspace.ResourceMetadata{},
		}
	}

	for ref, spec := range batch.ElementSpecs {
		ws.Elements[ref] = mergeImportElement(ws.Elements[ref], spec)
	}
	for _, spec := range batch.Connectors {
		ws.Connectors[workspace.ConnectorKey(spec)] = spec
	}

	if resp != nil {
		for ref, meta := range resp.GetElementMetadata() {
			ws.Meta.Elements[ref] = protoMetadata(meta)
		}
		for ref, meta := range resp.GetViewMetadata() {
			ws.Meta.Views[ref] = protoMetadata(meta)
			if el := ws.Elements[ref]; el != nil {
				el.HasView = true
			}
		}
		for ref, meta := range resp.GetConnectorMetadata() {
			ws.Meta.Connectors[ref] = protoMetadata(meta)
		}
		// Refresh the complete target snapshot: placement writes can copy related
		// connectors not explicitly declared in this batch.
		actual := cmdutil.ConvertExportResponse(ws, exported)
		cmdutil.PreservePendingResources(ws, actual)
		*ws = *actual
	}

	if err := workspace.Save(ws); err != nil {
		return err
	}
	return updateImportLockFile(wdir, ws)
}

func updateImportLockFile(wdir string, ws *workspace.Workspace) error {
	hash, err := workspace.CalculateWorkspaceHash(wdir)
	if err != nil {
		return fmt.Errorf("calculate workspace hash: %w", err)
	}
	lockFile, err := workspace.LoadLockFile(wdir)
	if err != nil {
		return fmt.Errorf("load lock file: %w", err)
	}
	if lockFile == nil {
		lockFile = &workspace.LockFile{Version: "v1"}
	}
	versionID := lockFile.VersionID
	if versionID == "" {
		versionID = fmt.Sprintf("import-%s", time.Now().UTC().Format(time.RFC3339))
	}
	workspace.UpdateLockFile(lockFile, versionID, "cli", &workspace.ResourceCounts{
		Elements:   len(ws.Elements),
		Views:      countWorkspaceViews(ws),
		Connectors: len(ws.Connectors),
	}, hash, nil, ws.Meta)
	return workspace.WriteLockFile(wdir, lockFile)
}

func countWorkspaceViews(ws *workspace.Workspace) int {
	count := 0
	for _, el := range ws.Elements {
		if el != nil && el.HasView {
			count++
		}
	}
	return count
}

// mergeImportElement layers an imported spec over an existing element. Imported
// non-zero fields win; unspecified fields keep the cached value so a partial
// import cannot silently erase metadata.
func mergeImportElement(existing, incoming *workspace.Element) *workspace.Element {
	if incoming == nil {
		return existing
	}
	if existing == nil {
		return incoming
	}
	merged := *existing
	if incoming.Name != "" {
		merged.Name = incoming.Name
	}
	if incoming.Kind != "" {
		merged.Kind = incoming.Kind
	}
	if incoming.Owner != "" {
		merged.Owner = incoming.Owner
	}
	if incoming.Description != "" {
		merged.Description = incoming.Description
	}
	if incoming.Technology != "" {
		merged.Technology = incoming.Technology
	}
	if incoming.URL != "" {
		merged.URL = incoming.URL
	}
	if incoming.LogoURL != "" {
		merged.LogoURL = incoming.LogoURL
	}
	if incoming.Repo != "" {
		merged.Repo = incoming.Repo
	}
	if incoming.Branch != "" {
		merged.Branch = incoming.Branch
	}
	if incoming.FilePath != "" {
		merged.FilePath = incoming.FilePath
	}
	if incoming.Symbol != "" {
		merged.Symbol = incoming.Symbol
	}
	if incoming.Tags != nil {
		merged.Tags = incoming.Tags
	}
	if incoming.ViewName != "" {
		merged.ViewName = incoming.ViewName
	}
	if incoming.ViewLabel != "" {
		merged.ViewLabel = incoming.ViewLabel
	}
	if incoming.DensityLevelSet || incoming.DensityLevel != 0 {
		merged.DensityLevel = incoming.DensityLevel
		merged.DensityLevelSet = incoming.DensityLevelSet
	}
	if incoming.BypassNoiseGate != nil {
		merged.BypassNoiseGate = incoming.BypassNoiseGate
	}
	if incoming.HasView {
		merged.HasView = true
	}
	merged.Placements = mergeImportPlacements(existing.Placements, incoming.Placements)
	return &merged
}

func mergeImportPlacements(existing, incoming []workspace.ViewPlacement) []workspace.ViewPlacement {
	if len(incoming) == 0 {
		return existing
	}
	out := append([]workspace.ViewPlacement(nil), existing...)
	for _, placement := range incoming {
		parent := placement.ParentRef
		if parent == "" {
			parent = workspace.RootRef
		}
		replaced := false
		for i := range out {
			current := out[i].ParentRef
			if current == "" {
				current = workspace.RootRef
			}
			if current == parent {
				if !placement.VisibilityDeltaSet && placement.VisibilityDelta == 0 {
					placement.VisibilityDelta = out[i].VisibilityDelta
				}
				if !placement.PositionXSet && placement.PositionX == 0 {
					placement.PositionX, placement.PositionXSet = out[i].PositionX, out[i].PositionXSet
				}
				if !placement.PositionYSet && placement.PositionY == 0 {
					placement.PositionY, placement.PositionYSet = out[i].PositionY, out[i].PositionYSet
				}
				out[i] = placement
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, placement)
		}
	}
	return out
}

func protoMetadata(meta *diagv1.ResourceMetadata) *workspace.ResourceMetadata {
	updatedAt := time.Now()
	if meta != nil && meta.UpdatedAt != nil {
		updatedAt = meta.UpdatedAt.AsTime()
	}
	id := int32(0)
	if meta != nil {
		id = meta.GetId()
	}
	return &workspace.ResourceMetadata{ID: workspace.ResourceID(id), UpdatedAt: updatedAt}
}

func reportImport(cmd *cobra.Command, format string, compact bool, command, status string, batch *exec.ImportBatch, responses ...*diagv1.ApplyWorkspaceBatchResponse) error {
	elements := batch.ElementsCreated + batch.ElementsUpdated
	connectors := batch.ConnectorsCreated + batch.ConnectorsUpdated
	omitted := 0
	for _, view := range batch.Views {
		omitted += len(view.OmittedConnectors)
	}

	if cmdutil.WantsJSON(format) {
		var response *diagv1.ApplyWorkspaceBatchResponse
		if len(responses) > 0 {
			response = responses[0]
		}
		items := make([]cmdutil.JSONItem, 0, elements+connectors)
		for _, el := range batch.Request.GetElements() {
			id, action := el.GetId(), "add"
			if id != 0 {
				action = "update"
			}
			if batch.Unchanged["element:"+el.GetRef()] {
				action = "unchanged"
			}
			if response != nil {
				id = response.GetElementMetadata()[el.GetRef()].GetId()
			}
			items = append(items, cmdutil.JSONItem{Ref: el.GetRef(), ID: id, ResourceType: "element", Action: action, Name: el.GetName()})
			if el.GetHasView() {
				viewID, viewAction := el.GetViewId(), "add"
				if viewID != 0 {
					viewAction = "update"
				}
				if batch.Unchanged["view:"+el.GetRef()] {
					viewAction = "unchanged"
				}
				if response != nil {
					viewID = response.GetViewMetadata()[el.GetRef()].GetId()
				}
				items = append(items, cmdutil.JSONItem{Ref: el.GetRef(), ID: viewID, ResourceType: "view", Action: viewAction})
			}
		}
		for _, connector := range batch.Request.GetConnectors() {
			id, action := connector.GetId(), "add"
			if id != 0 {
				action = "update"
			}
			if batch.Unchanged["connector:"+connector.GetRef()] {
				action = "unchanged"
			}
			if response != nil {
				id = response.GetConnectorMetadata()[connector.GetRef()].GetId()
			}
			items = append(items, cmdutil.JSONItem{Ref: connector.GetRef(), ID: id, ResourceType: "connector", Action: action})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].ResourceType != items[j].ResourceType {
				return items[i].ResourceType < items[j].ResourceType
			}
			return items[i].Ref < items[j].Ref
		})
		return cmdutil.WriteJSON(cmd.OutOrStdout(), compact, cmdutil.JSONOutput{
			Command: command,
			Status:  status,
			Items:   items,
			Extra:   map[string]any{"views": batch.Views, "preserved_placements": batch.PreservedPlacements},
			Summary: map[string]int{
				"omitted_connectors":   omitted,
				"elements_created":     batch.ElementsCreated,
				"elements_updated":     batch.ElementsUpdated,
				"elements_unchanged":   batch.ElementsUnchanged,
				"views_created":        batch.ViewsCreated,
				"connectors_created":   batch.ConnectorsCreated,
				"connectors_updated":   batch.ConnectorsUpdated,
				"connectors_unchanged": batch.ConnectorsUnchanged,
			},
		})
	}

	for _, view := range batch.Views {
		term.Infof(cmd.OutOrStdout(), "view %s: %d elements, %d connectors (%d direct, %d off-view, %d unresolved)", view.View, view.Elements, view.Connectors, view.RenderedConnectors, len(view.OffViewConnectors), len(view.UnresolvedConnectors))
		if len(view.OffViewConnectors) > 0 {
			term.Info(cmd.OutOrStdout(), workspace.OffViewHint)
		}
	}
	if len(batch.PreservedPlacements) > 0 {
		refs := make([]string, 0, len(batch.PreservedPlacements))
		for ref := range batch.PreservedPlacements {
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		for _, ref := range refs {
			term.Infof(cmd.OutOrStdout(), "kept existing placements for %s: %s", ref, strings.Join(batch.PreservedPlacements[ref], ", "))
		}
		term.Infof(cmd.OutOrStdout(), "Use 'tld view unplace <view> <element>' to remove a placement.")
	}
	if status == "dry-run" {
		term.Successf(cmd.OutOrStdout(),
			"dry-run: would %s %d element(s) (%d new, %d updated, %d unchanged) and %d connector(s) (%d new, %d updated, %d unchanged)",
			command, elements, batch.ElementsCreated, batch.ElementsUpdated,
			batch.ElementsUnchanged, connectors, batch.ConnectorsCreated, batch.ConnectorsUpdated, batch.ConnectorsUnchanged)
		if batch.ViewsCreated > 0 {
			term.Infof(cmd.OutOrStdout(), "would create %d view(s)", batch.ViewsCreated)
		}
		return nil
	}

	term.Successf(cmd.OutOrStdout(),
		"%s: %d element(s) (%d new, %d updated, %d unchanged) and %d connector(s) (%d new, %d updated, %d unchanged)",
		command, elements, batch.ElementsCreated, batch.ElementsUpdated,
		batch.ElementsUnchanged, connectors, batch.ConnectorsCreated, batch.ConnectorsUpdated, batch.ConnectorsUnchanged)
	if batch.ViewsCreated > 0 {
		term.Infof(cmd.OutOrStdout(), "created %d view(s)", batch.ViewsCreated)
	}
	return nil
}

// Describe the merged state, including placements an upsert deliberately keeps.
func setImportDiagnostics(batch *exec.ImportBatch, ws *workspace.Workspace, declared map[string]*workspace.Element) {
	projected := &workspace.Workspace{Elements: map[string]*workspace.Element{}, Connectors: map[string]*workspace.Connector{}, Meta: ws.Meta}
	for ref, el := range ws.Elements {
		projected.Elements[ref] = el
	}
	for key, c := range ws.Connectors {
		projected.Connectors[key] = c
	}
	batch.PreservedPlacements = map[string][]string{}
	for ref, el := range batch.ElementSpecs {
		merged := mergeImportElement(ws.Elements[ref], el)
		copy := *merged
		projected.Elements[ref] = &copy
	}
	for _, el := range batch.Request.Elements {
		if el.HasView {
			projected.Elements[el.Ref].HasView = true
		}
	}
	for ref, spec := range declared {
		if old := ws.Elements[ref]; old != nil {
			parents := map[string]bool{}
			for _, p := range spec.Placements {
				parents[cmdutil.NormalizeParentRef(p.ParentRef)] = true
			}
			for _, p := range old.Placements {
				parent := cmdutil.NormalizeParentRef(p.ParentRef)
				if !parents[parent] {
					batch.PreservedPlacements[ref] = append(batch.PreservedPlacements[ref], parent)
				}
			}
			sort.Strings(batch.PreservedPlacements[ref])
		}
	}
	for _, c := range batch.Connectors {
		projected.Connectors[workspace.ConnectorKey(c)] = c
	}
	// A newly shared endpoint causes the target to copy existing relationships
	// whose opposite endpoint is already placed in the destination view.
	for ref, spec := range declared {
		for _, p := range spec.Placements {
			view := cmdutil.NormalizeParentRef(p.ParentRef)
			if ws.ElementPlacedInView(ref, view) {
				continue
			}
			for _, old := range ws.Connectors {
				if old == nil || cmdutil.NormalizeParentRef(old.View) == view || (old.Source != ref && old.Target != ref) {
					continue
				}
				if !projected.ElementPlacedInView(old.Source, view) || !projected.ElementPlacedInView(old.Target, view) {
					continue
				}
				copy := *old
				copy.View = view
				key := workspace.ConnectorKey(&copy)
				if projected.Connectors[key] == nil {
					projected.Connectors[key] = &copy
				}
			}
		}
	}
	batch.Views = projected.ViewDiagnostics()
}
