package pull

import (
	"bufio"
	"errors"
	"fmt"
	"strings"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/internal/client"
	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/term"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func NewPullCmd(wdir *string) *cobra.Command {
	var force bool
	var dryRun bool

	c := &cobra.Command{
		Use:   "pull",
		Short: "Pull the current server state into local YAML files",
		Long: `Pull downloads the current diagram state from the server and overwrites
local YAML files. Use this after making changes in the frontend UI.

If you have local changes that haven't been applied yet, tld pull will warn
you before overwriting them. Use --force to skip the prompt.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				return err
			}
			if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
				return err
			}

			targetOrg := ws.Config.WorkspaceID
			if targetOrg == "" {
				return cmdutil.WorkspaceIDRequired("org-id required in .tld.yaml")
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

			c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
			resp, err := c.ExportWorkspace(cmd.Context(), connect.NewRequest(&diagv1.ExportOrganizationRequest{
				OrgId: targetOrg,
			}))
			if err != nil {
				return cmdutil.WithUnauthorizedHint("pull failed", err)
			}

			newWS := cmdutil.ConvertExportResponse(ws, resp.Msg)

			if dryRun {
				term.Infof(cmd.OutOrStdout(), "Would pull: %d elements, %d diagrams, %d connectors",
					len(newWS.Elements), cmdutil.CountElementDiagrams(newWS), len(newWS.Connectors))
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
				Views:      cmdutil.CountElementDiagrams(newWS),
				Connectors: len(newWS.Connectors),
			}, hash, nil, newWS.Meta)
			lockFile.Resources.Elements = len(newWS.Elements)
			lockFile.Resources.Views = cmdutil.CountElementDiagrams(newWS)
			lockFile.Resources.Connectors = len(newWS.Connectors)
			if err := workspace.WriteLockFile(*wdir, lockFile); err != nil {
				return fmt.Errorf("write lock file: %w", err)
			}

			term.Successf(cmd.OutOrStdout(), "Pulled %d elements, %d diagrams, %d connectors",
				len(newWS.Elements), cmdutil.CountElementDiagrams(newWS), len(newWS.Connectors))

			return nil
		},
	}

	c.Flags().BoolVar(&force, "force", false, "overwrite local changes without prompting")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be pulled without writing")
	return c
}
