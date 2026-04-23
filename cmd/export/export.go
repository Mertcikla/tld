package export

import (
	"fmt"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/diag/tld/internal/client"
	"github.com/mertcikla/diag/tld/internal/cmdutil"
	"github.com/mertcikla/diag/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func NewExportCmd(wdir *string) *cobra.Command {
	c := &cobra.Command{
		Use:   "export [org-id]",
		Short: "Export all diagrams from an organization to the local workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w (did you run 'tld init'?)", err)
			}

			targetOrg := ws.Config.WorkspaceID
			if len(args) > 0 {
				targetOrg = args[0]
			}
			if targetOrg == "" {
				return fmt.Errorf("org-id required (either as argument or in .tld.yaml)")
			}

			c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
			resp, err := c.ExportWorkspace(cmd.Context(), connect.NewRequest(&diagv1.ExportOrganizationRequest{
				OrgId: targetOrg,
			}))
			if err != nil {
				return fmt.Errorf("export failed: %w", err)
			}

			newWS := cmdutil.ConvertExportResponse(ws, resp.Msg)

			if err := workspace.Save(newWS); err != nil {
				return fmt.Errorf("save workspace: %w", err)
			}

			// Update lock file so version tracking stays consistent
			hash, err := workspace.CalculateWorkspaceHash(*wdir)
			if err != nil {
				return fmt.Errorf("calculate hash: %w", err)
			}
			lockFile, err := workspace.LoadLockFile(*wdir)
			if err != nil {
				return fmt.Errorf("load lock file: %w", err)
			}
			if lockFile == nil {
				lockFile = &workspace.LockFile{Version: "v1"}
			}
			versionID := fmt.Sprintf("pull-%s", time.Now().UTC().Format(time.RFC3339))
			workspace.UpdateLockFile(lockFile, versionID, "pull", &workspace.ResourceCounts{
				Elements:   len(newWS.Elements),
				Views:      cmdutil.CountElementDiagrams(newWS),
				Connectors: len(newWS.Connectors),
			}, hash, nil, newWS.Meta)
			lockFile.Resources.Elements = len(newWS.Elements)
			lockFile.Resources.Views = cmdutil.CountElementDiagrams(newWS)
			lockFile.Resources.Connectors = len(newWS.Connectors)
			if err := workspace.WriteLockFile(*wdir, lockFile); err != nil {
				return fmt.Errorf("write lock file: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Exported %d elements, %d diagrams, %d connectors to %s\n",
				len(newWS.Elements), cmdutil.CountElementDiagrams(newWS), len(newWS.Connectors), *wdir)

			return nil
		},
	}

	return c
}
