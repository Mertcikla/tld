package serve

import (
	"log"
	"net/http"
	"os"

	"github.com/mertcikla/tld/internal/localserver"
	"github.com/spf13/cobra"
)

func defaultServeRunE(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	app, err := localserver.Bootstrap(cwd)
	if err != nil {
		return err
	}

	log.Printf("tld listening at http://%s\n", app.Addr)
	return http.ListenAndServe(app.Addr, app.Handler)
}

func NewServeCmd(runE func(*cobra.Command, []string) error) *cobra.Command {
	if runE == nil {
		runE = defaultServeRunE
	}

	return &cobra.Command{
		Use:   "serve",
		Short: "A brief description of your command",
		Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
		RunE: runE,
	}
}
