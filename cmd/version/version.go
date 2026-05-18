package version

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/mertcikla/tld/v2/internal/selfupdate"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// Version is the current version of the CLI.
// This is overridden by ldflags during build.
var Version = "2.1.0"

func NewVersionCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "version",
		Short: "Print the version number of tld",
		Run: func(cmd *cobra.Command, _ []string) {
			term.Infof(cmd.OutOrStdout(), "tld version %s", Version)
		},
	}

	c.AddCommand(newUpdateCmd())

	return c
}

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Update the tld CLI binary from GitHub releases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := workspace.LoadGlobalConfig()
			if err != nil {
				return err
			}
			interval, err := time.ParseDuration(cfg.Updates.CheckInterval)
			if err != nil || interval <= 0 {
				interval = selfupdate.DefaultCheckInterval
			}
			statePath := ""
			if dir, err := workspace.ConfigDir(); err == nil {
				statePath = filepath.Join(dir, "update-check.json")
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Minute)
			defer cancel()
			status, err := selfupdate.Install(ctx, selfupdate.Options{
				Current:       Version,
				CheckInterval: interval,
				StatePath:     statePath,
				Force:         true,
			})
			if err != nil {
				return fmt.Errorf("update tld: %w", err)
			}
			if !status.UpdateAvailable {
				term.Successf(cmd.OutOrStdout(), "tld %s is already up to date.", Version)
				return nil
			}
			term.Successf(cmd.OutOrStdout(), "Updated tld from %s to %s.", Version, status.Latest)
			term.Hint(cmd.OutOrStdout(), "Restart any running tld server to use the new version.")
			return nil
		},
	}
}
