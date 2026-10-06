// Package git contains Git-aware repository commands that run against the
// local tld data directory, independent of a running server.
package git

import "github.com/spf13/cobra"

// NewGitCmd builds the `tld git` command group.
func NewGitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "git",
		Short: "Git-aware repository operations",
	}
	cmd.AddCommand(newCompareCmd())
	return cmd
}
