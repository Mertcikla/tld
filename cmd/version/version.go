package version

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the current version of the CLI.
// This is overridden by ldflags during build.
var Version = "1.89.1"

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number of tld",
		Run: func(cmd *cobra.Command, _ []string) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "tld version %s\n", Version)
		},
	}
}
