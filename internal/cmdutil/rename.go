package cmdutil

import (
	"fmt"

	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// RenameWorkspaceElement changes a YAML alias and cascades its references and
// metadata. Both update element ref and the deprecated rename command use it.
func RenameWorkspaceElement(cmd *cobra.Command, wdir, from, to string, dryRun bool) error {
	if !WorkspaceConfigured(cmd, wdir) {
		return WithHint(fmt.Errorf("renaming an element ref requires a YAML workspace"), "Run 'tld init' first or pass --yaml to rename a workspace-local ref. To change a display name, use 'tld update element <ref-or-id> name <new-name>'.")
	}
	if dryRun {
		if err := WithWorkspaceDryRun(wdir, func(cloneDir string) error {
			return workspace.RenameElement(cloneDir, from, to)
		}); err != nil {
			return fmt.Errorf("dry-run rename element: %w", err)
		}
		return nil
	}
	lock, err := workspace.LoadLockFile(wdir)
	if err != nil {
		return err
	}
	clean := false
	if lock != nil && lock.WorkspaceHash != "" {
		hash, err := workspace.CalculateWorkspaceHash(wdir)
		if err != nil {
			return err
		}
		clean = hash == lock.WorkspaceHash
	}
	if err := workspace.RenameElement(wdir, from, to); err != nil {
		return fmt.Errorf("rename element: %w", err)
	}
	if clean {
		// Reload the lock to retain the metadata cascade performed by the rename.
		lock, err := workspace.LoadLockFile(wdir)
		if err != nil {
			return err
		}
		lock.WorkspaceHash, err = workspace.CalculateWorkspaceHash(wdir)
		if err != nil {
			return err
		}
		return workspace.WriteLockFile(wdir, lock)
	}
	return nil
}
