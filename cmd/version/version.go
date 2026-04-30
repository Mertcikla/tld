package version

import (
	"github.com/mertcikla/tld/internal/term"
	"github.com/spf13/cobra"
)

// Version is the current version of the CLI.
// This is overridden by ldflags during build.
var Version = "1.95.1"

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version number of tld",
		Run: func(cmd *cobra.Command, _ []string) {
			term.Infof(cmd.OutOrStdout(), "tld version %s", Version)
		},
	}
}
