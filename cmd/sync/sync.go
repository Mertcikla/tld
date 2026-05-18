package synccmd

import "github.com/spf13/cobra"

func NewSyncCmd(wdir *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Inspect and reconcile workspace sync state",
		Long: `Inspect and reconcile sync state between local YAML and the diagram server.

Use 'tld sync status' to compare the local workspace against the last
known sync point, optionally checking the live server for drift.`,
	}

	cmd.AddCommand(NewStatusCmd(wdir))
	return cmd
}
