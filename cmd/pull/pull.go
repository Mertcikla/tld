package pull

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/client"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
)

func NewPullCmd(wdir *string) *cobra.Command {
	var force bool
	var dryRun bool
	var target string
	var dataDir string

	c := &cobra.Command{
		Use:   "pull",
		Short: "Pull the current server state into local YAML files",
		Long: `Pull downloads the current diagram state from the server and refreshes
the local YAML cache. Use this after making changes in the frontend UI.

If you have local hand-edits, tld pull will warn you before overwriting
them. Use --force to skip the prompt.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				return err
			}

		resolvedTarget, err := exec.ResolveTarget(ws.Config, target)
		if err != nil {
			return err
		}

		if resolvedTarget == exec.TargetRemote {
				if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
					return err
				}

				targetOrg := ws.Config.WorkspaceID
				if targetOrg == "" {
					return cmdutil.WorkspaceIDRequired("org-id required in .tld.yaml")
				}
			}

			lockFile, err := workspace.LoadLockFile(*wdir)
			if err != nil {
				return fmt.Errorf("load lock file: %w", err)
			}

			// Detect local changes if we have a previous hash to compare against
			if !force && lockFile != nil && lockFile.WorkspaceHash != "" {
				currentHash, err := workspace.CalculateWorkspaceHash(*wdir)
				if err != nil {
					return fmt.Errorf("calculate hash: %w", err)
				}
				if currentHash != lockFile.WorkspaceHash {
					term.Warn(cmd.OutOrStdout(), "Local workspace has uncommitted changes. Pull will overwrite them.")
					_, _ = fmt.Fprint(cmd.OutOrStdout(), "  Continue? [yes/no]: ")
					scanner := bufio.NewScanner(cmd.InOrStdin())
					if !scanner.Scan() {
						return errors.New("aborted")
					}
					answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
					if answer != "yes" && answer != "y" {
						term.Infof(cmd.OutOrStdout(), "Pull cancelled.")
						return nil
					}
				}
			}

		var exportResp *diagv1.ExportOrganizationResponse
		if resolvedTarget == exec.TargetRemote {
				c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
				resp, err := c.ExportWorkspace(cmd.Context(), connect.NewRequest(&diagv1.ExportOrganizationRequest{
					OrgId: ws.Config.WorkspaceID,
				}))
				if err != nil {
					return cmdutil.WithUnauthorizedHint("pull failed", err)
				}
				exportResp = resp.Msg
			} else {
				resolvedDataDir, err := workspace.ResolveDataDir(&ws.Config, dataDir)
				if err != nil {
					return err
				}
				dbPath := localserver.DatabasePath(resolvedDataDir)
				if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
					return fmt.Errorf("create data dir: %w", err)
				}
				sqliteStore, err := store.Open(dbPath, assets.FS)
				if err != nil {
					return err
				}
				defer func() { _ = sqliteStore.Legacy().Close() }()
				adapter := store.NewAPIAdapter(sqliteStore)
				exportResp, err = exportLocalWorkspace(cmd.Context(), adapter)
				if err != nil {
					return fmt.Errorf("pull local failed: %w", err)
				}
			}

			newWS := cmdutil.ConvertExportResponse(ws, exportResp)

			if dryRun {
				term.Infof(cmd.OutOrStdout(), "Would pull: %d elements, %d diagrams, %d connectors",
					len(newWS.Elements), cmdutil.CountViews(newWS), len(newWS.Connectors))
				return nil
			}

			// Perform surgical merge
			lastSyncMeta := &workspace.Meta{
				Elements:   make(map[string]*workspace.ResourceMetadata),
				Views:      make(map[string]*workspace.ResourceMetadata),
				Connectors: make(map[string]*workspace.ResourceMetadata),
			}
			if lockFile != nil && lockFile.Metadata != nil {
				lastSyncMeta = lockFile.Metadata
			}

			if force {
				if err := workspace.Save(newWS); err != nil {
					return fmt.Errorf("force save workspace: %w", err)
				}
			} else {
				if err := workspace.MergeWorkspace(*wdir, newWS, lastSyncMeta, ws.Meta); err != nil {
					return fmt.Errorf("merge workspace: %w", err)
				}
			}

			hash, err := workspace.CalculateWorkspaceHash(*wdir)
			if err != nil {
				return fmt.Errorf("calculate hash: %w", err)
			}
			if lockFile == nil {
				lockFile = &workspace.LockFile{Version: "v1"}
			}
			versionID := fmt.Sprintf("pull-%s", time.Now().UTC().Format(time.RFC3339))
			workspace.UpdateLockFile(lockFile, versionID, "pull", &workspace.ResourceCounts{
				Elements:   len(newWS.Elements),
				Views:      cmdutil.CountViews(newWS),
				Connectors: len(newWS.Connectors),
			}, hash, nil, newWS.Meta)
			lockFile.Resources.Elements = len(newWS.Elements)
			lockFile.Resources.Views = cmdutil.CountViews(newWS)
			lockFile.Resources.Connectors = len(newWS.Connectors)
			if err := workspace.WriteLockFile(*wdir, lockFile); err != nil {
				return fmt.Errorf("write lock file: %w", err)
			}

			term.Successf(cmd.OutOrStdout(), "Pulled %d elements, %d diagrams, %d connectors",
				len(newWS.Elements), cmdutil.CountViews(newWS), len(newWS.Connectors))

			return nil
		},
	}

	c.Flags().BoolVar(&force, "force", false, "overwrite local changes without prompting")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be pulled without writing")
	c.Flags().StringVar(&target, "target", "", "pull target: auto, local, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}

func exportLocalWorkspace(ctx context.Context, adapter *store.APIAdapter) (*diagv1.ExportOrganizationResponse, error) {
	var (
		views      []*diagv1.View
		elements   []*diagv1.Element
		placements []*diagv1.PlacedElement
		connectors []*diagv1.Connector
		layers     []*diagv1.ViewLayer
	)

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { var e error; views, e = adapter.ListViews(gctx, uuid.Nil); return e })
	g.Go(func() error {
		var e error
		elements, _, e = adapter.ListElements(gctx, uuid.Nil, 0, 0, "")
		return e
	})
	g.Go(func() error { var e error; placements, e = adapter.ListAllPlacements(gctx, uuid.Nil); return e })
	g.Go(func() error { var e error; connectors, e = adapter.ListAllConnectors(gctx, uuid.Nil); return e })
	g.Go(func() error { var e error; layers, e = adapter.ListAllViewLayers(gctx, uuid.Nil); return e })

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("local export query: %w", err)
	}

	// Convert PlacedElement → ElementPlacement for export format
	exportPlacements := make([]*diagv1.ElementPlacement, 0, len(placements))
	for _, p := range placements {
		exportPlacements = append(exportPlacements, &diagv1.ElementPlacement{
			Id:        p.Id,
			ViewId:    p.ViewId,
			ElementId: p.ElementId,
			PositionX: p.PositionX,
			PositionY: p.PositionY,
		})
	}

	elementToChildView := make(map[int32]*diagv1.View, len(views))
	for _, v := range views {
		if v.OwnerElementId != nil {
			elementToChildView[*v.OwnerElementId] = v
		}
	}
	navigations := make([]*diagv1.ElementNavigation, 0)
	for _, p := range placements {
		childView, ok := elementToChildView[p.ElementId]
		if !ok {
			continue
		}
		navigations = append(navigations, &diagv1.ElementNavigation{
			Id:         p.Id,
			ElementId:  p.ElementId,
			FromViewId: p.ViewId,
			ToViewId:   childView.Id,
		})
	}

	return &diagv1.ExportOrganizationResponse{
		Views:       views,
		Elements:    elements,
		Navigations: navigations,
		Placements:  exportPlacements,
		Connectors:  connectors,
		Layers:      layers,
	}, nil
}
