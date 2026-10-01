package importcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
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

Schemas:
  elements:   %s
  connectors: %s`, workspace.ElementsSchemaURL, workspace.ConnectorsSchemaURL),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd, *wdir, *format, *compact, args[0], target, dataDir, dryRun)
		},
	}

	c.Flags().BoolVar(&dryRun, "dry-run", false, "validate and report changes without applying them")
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, remote, or cloud")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}

func runImport(cmd *cobra.Command, wdir, format string, compact bool, file, target, dataDir string, dryRun bool) error {
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

	ws, err := cmdutil.LoadWorkspace(wdir)
	if err != nil {
		return fail(err)
	}

	runner, err := exec.NewRunner(ws.Config, target, dataDir, false)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = runner.Close() }()
	if runner.Name() == exec.TargetRemote {
		if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
			return fail(err)
		}
	}

	ctx := cmd.Context()
	plan, err := exec.BuildImportPlan(ctx, runner, ws, doc.Elements, doc.Connectors)
	if err != nil {
		return fail(fmt.Errorf("import aborted before any changes were made: %w", err))
	}

	if dryRun {
		return reportImport(cmd, format, compact, "dry-run", plan)
	}

	resp, err := runner.ApplyPlan(ctx, plan.Request)
	if err != nil {
		return fail(explainApplyError(err))
	}

	if err := persistImportCache(wdir, ws, plan, resp); err != nil {
		return fail(fmt.Errorf(
			"the import was applied to the target, but updating the local cache failed: %w\n"+
				"The target state is correct; run `tld pull` to refresh the local files",
			err))
	}

	return reportImport(cmd, format, compact, "ok", plan)
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
		return nil, fmt.Errorf("import file %s is empty", label)
	}

	var doc importDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse import file %s: %w%s", label, err, schemaHint())
	}
	if len(doc.Elements) == 0 && len(doc.Connectors) == 0 {
		return nil, fmt.Errorf("import file %s defines no elements or connectors; expected top-level `elements` and/or `connectors`%s", label, schemaHint())
	}
	return &doc, nil
}

// schemaHint points at the published schemas so editors can validate and
// autocomplete the import file. Appended to parse/shape errors.
func schemaHint() string {
	return fmt.Sprintf("\nValidate the file against the schemas:\n  elements:   %s\n  connectors: %s",
		workspace.ElementsSchemaURL, workspace.ConnectorsSchemaURL)
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

// persistImportCache writes the successful plan back into the local YAML cache,
// mirroring the write-through behavior of `add`/`connect`. The target is always
// the source of truth: if this fails, the caller reports that a `tld pull` is
// needed rather than treating the import as failed.
func persistImportCache(wdir string, ws *workspace.Workspace, plan *exec.ImportPlan, resp *diagv1.ApplyPlanResponse) error {
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

	for ref, spec := range plan.ElementSpecs {
		ws.Elements[ref] = mergeImportElement(ws.Elements[ref], spec)
	}
	for _, spec := range plan.Connectors {
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
	if incoming.DensityLevel != 0 {
		merged.DensityLevel = incoming.DensityLevel
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

func reportImport(cmd *cobra.Command, format string, compact bool, status string, plan *exec.ImportPlan) error {
	elements := plan.ElementsCreated + plan.ElementsUpdated
	connectors := plan.ConnectorsCreated + plan.ConnectorsUpdated

	if cmdutil.WantsJSON(format) {
		return cmdutil.WriteJSON(cmd.OutOrStdout(), compact, cmdutil.JSONOutput{
			Command: "import",
			Status:  status,
			Summary: map[string]int{
				"elements_created":   plan.ElementsCreated,
				"elements_updated":   plan.ElementsUpdated,
				"views_created":      plan.ViewsCreated,
				"connectors_created": plan.ConnectorsCreated,
				"connectors_updated": plan.ConnectorsUpdated,
			},
		})
	}

	if status == "dry-run" {
		term.Successf(cmd.OutOrStdout(),
			"dry-run: would import %d element(s) (%d new, %d updated) and %d connector(s) (%d new, %d updated)",
			elements, plan.ElementsCreated, plan.ElementsUpdated,
			connectors, plan.ConnectorsCreated, plan.ConnectorsUpdated)
		if plan.ViewsCreated > 0 {
			term.Infof(cmd.OutOrStdout(), "would create %d view(s)", plan.ViewsCreated)
		}
		return nil
	}

	term.Successf(cmd.OutOrStdout(),
		"import: %d element(s) (%d new, %d updated) and %d connector(s) (%d new, %d updated)",
		elements, plan.ElementsCreated, plan.ElementsUpdated,
		connectors, plan.ConnectorsCreated, plan.ConnectorsUpdated)
	if plan.ViewsCreated > 0 {
		term.Infof(cmd.OutOrStdout(), "created %d view(s)", plan.ViewsCreated)
	}
	return nil
}
