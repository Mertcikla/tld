package serve

import (
	"fmt"
	"net/http"
	"os"

	"github.com/mertcikla/tld/internal/localserver"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func defaultServeRunE(cmd *cobra.Command, args []string) error {
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetString("port")

	opts := resolveServeOptions(host, port)

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	app, err := localserver.Bootstrap(cwd, opts)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Webapp available at http://%s\n", app.Addr)
	return http.ListenAndServe(app.Addr, app.Handler)
}

// resolveServeOptions merges CLI flags with global config.
// Priority: CLI flag > env var (handled in localserver) > global config > default.
func resolveServeOptions(flagHost, flagPort string) localserver.ServeOptions {
	cfg, _ := workspace.LoadGlobalConfig()

	host := cfg.Serve.Host
	port := cfg.Serve.Port

	if flagHost != "" {
		host = flagHost
	}
	if flagPort != "" {
		port = flagPort
	}

	return localserver.ServeOptions{Host: host, Port: port}
}

func NewServeCmd(runE func(*cobra.Command, []string) error) *cobra.Command {
	if runE == nil {
		runE = defaultServeRunE
	}

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the local tlDiagram web server",
		Long: `Start the tlDiagram web server locally.

The host and port can be set via flags, the global config file
(~/.config/tldiagram/tld.yaml under serve.host / serve.port),
or the TLD_ADDR / PORT environment variables.`,
		RunE: runE,
	}

	cmd.Flags().String("host", "", "host address to bind (overrides config and env)")
	cmd.Flags().String("port", "", "port to listen on (overrides config and env)")

	return cmd
}
