package cmd

import (
	"fmt"
	"os"

	"github.com/mertcikla/tld/v2/cmd/add"
	configcmd "github.com/mertcikla/tld/v2/cmd/config"
	"github.com/mertcikla/tld/v2/cmd/connect"
	doctorcmd "github.com/mertcikla/tld/v2/cmd/doctor"
	"github.com/mertcikla/tld/v2/cmd/export"
	indexcmd "github.com/mertcikla/tld/v2/cmd/index"
	"github.com/mertcikla/tld/v2/cmd/initialize"
	inspectcmd "github.com/mertcikla/tld/v2/cmd/inspect"
	listcmd "github.com/mertcikla/tld/v2/cmd/list"
	"github.com/mertcikla/tld/v2/cmd/login"
	"github.com/mertcikla/tld/v2/cmd/mcp"
	"github.com/mertcikla/tld/v2/cmd/pull"
	"github.com/mertcikla/tld/v2/cmd/remove"
	"github.com/mertcikla/tld/v2/cmd/rename"
	"github.com/mertcikla/tld/v2/cmd/render"
	"github.com/mertcikla/tld/v2/cmd/serve"
	"github.com/mertcikla/tld/v2/cmd/status"
	"github.com/mertcikla/tld/v2/cmd/stop"
	techcmd "github.com/mertcikla/tld/v2/cmd/tech"
	"github.com/mertcikla/tld/v2/cmd/update"
	"github.com/mertcikla/tld/v2/cmd/validate"
	"github.com/mertcikla/tld/v2/cmd/version"
	viewcmd "github.com/mertcikla/tld/v2/cmd/view"
	"github.com/mertcikla/tld/v2/cmd/views"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

var rootCmd = NewRootCmd()
var outputFormat string
var compactJSON bool

type RootOption func(*cobra.Command)

// WithServeCommand replaces the default serve handler for tests.
func WithServeCommand(runE func(*cobra.Command, []string) error) RootOption {
	return func(root *cobra.Command) {
		cmd, _, err := root.Find([]string{"serve"})
		if err != nil || cmd == nil {
			return
		}
		cmd.RunE = runE
	}
}

// Execute runs the root command and exits on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// NewRootCmd creates a fresh, isolated root command. Called by Execute() for the
// binary and by tests to get a clean instance with no shared state.
func NewRootCmd(options ...RootOption) *cobra.Command {
	root := &cobra.Command{
		Use:           "tld",
		Short:         "tld -- tlDiagram CLI",
		Long:          `tld`,
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version.Version,
	}

	var wdir string
	defaultWdir := ""
	if _, err := os.Stat(".tld"); err == nil {
		defaultWdir = ".tld"
	} else if _, err := os.Stat("tld"); err == nil {
		defaultWdir = "tld"
	}
	root.PersistentFlags().StringVarP(&wdir, "workspace", "w", defaultWdir, "workspace directory")
	root.PersistentFlags().StringVar(&outputFormat, "format", "text", "output format: text or json")
	root.PersistentFlags().BoolVar(&compactJSON, "compact", false, "compact JSON output (no whitespace)")

	// Accept either a content root or a workspace directory for --workspace by
	// resolving to the nested ".tld" directory when the root itself does not
	// hold workspace files.
	root.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		wdir = workspace.ResolveDir(wdir)
		return nil
	}

	// Define groups
	resourceGroup := &cobra.Group{
		ID:    "resource",
		Title: "CRUD actions on resources:",
	}
	workspaceGroup := &cobra.Group{
		ID:    "workspace",
		Title: "Workspace:",
	}
	syncGroup := &cobra.Group{
		ID:    "sync",
		Title: "Sync & authentication:",
	}
	queryGroup := &cobra.Group{
		ID:    "query",
		Title: "Inspect & query:",
	}
	serverGroup := &cobra.Group{
		ID:    "server",
		Title: "Server & integrations:",
	}
	systemGroup := &cobra.Group{
		ID:    "system",
		Title: "Configuration & system:",
	}
	root.AddGroup(resourceGroup, workspaceGroup, syncGroup, queryGroup, serverGroup, systemGroup)

	// CRUD Commands
	addCmd := add.NewAddCmd(&wdir, &outputFormat, &compactJSON)
	addCmd.GroupID = resourceGroup.ID

	connectCmd := connect.NewConnectCmd(&wdir, &outputFormat, &compactJSON)
	connectCmd.GroupID = resourceGroup.ID

	removeCmd := remove.NewRemoveCmd(&wdir, &outputFormat, &compactJSON)
	removeCmd.GroupID = resourceGroup.ID

	updateCmd := update.NewUpdateCmd(&wdir, &outputFormat, &compactJSON)
	updateCmd.GroupID = resourceGroup.ID

	renameCmd := rename.NewRenameCmd(&wdir)
	renameCmd.GroupID = resourceGroup.ID

	viewCmd := viewcmd.NewViewCmd(&wdir, &outputFormat, &compactJSON)
	viewCmd.GroupID = resourceGroup.ID

	// Workspace commands
	initCmd := initialize.NewInitCmd()
	initCmd.GroupID = workspaceGroup.ID

	validateCmd := validate.NewValidateCmd(&wdir)
	validateCmd.GroupID = workspaceGroup.ID

	indexCmd := indexcmd.NewIndexCmd()
	indexCmd.GroupID = workspaceGroup.ID

	// Sync & authentication commands
	loginCmd := login.NewLoginCmd(&wdir)
	loginCmd.GroupID = syncGroup.ID

	exportCmd := export.NewExportCmd(&wdir)
	exportCmd.GroupID = syncGroup.ID

	pullCmd := pull.NewPullCmd(&wdir)
	pullCmd.GroupID = syncGroup.ID

	// Inspect & query commands
	viewsCmd := views.NewViewsCmd(&wdir)
	viewsCmd.GroupID = queryGroup.ID

	renderCmd := render.NewRenderCmd(&wdir)
	renderCmd.GroupID = queryGroup.ID

	inspectCmd := inspectcmd.NewInspectCmd(&wdir, &outputFormat, &compactJSON)
	inspectCmd.GroupID = queryGroup.ID

	listCmd := listcmd.NewListCmd(&wdir, &outputFormat, &compactJSON)
	listCmd.GroupID = queryGroup.ID

	techCmd := techcmd.NewTechCmd()
	techCmd.GroupID = queryGroup.ID

	// Server & integration commands
	serveCmd := serve.NewServeCmd(nil)
	serveCmd.GroupID = serverGroup.ID

	mcpCmd := mcp.NewMCPCmd(&wdir, &outputFormat, &compactJSON)
	mcpCmd.GroupID = serverGroup.ID

	stopCmd := stop.NewStopCmd()
	stopCmd.GroupID = serverGroup.ID

	statusCmd := status.NewStatusCmd()
	statusCmd.GroupID = serverGroup.ID

	// Configuration & system commands
	versionCmd := version.NewVersionCmd()
	versionCmd.GroupID = systemGroup.ID

	configCmd := configcmd.NewConfigCmd()
	configCmd.GroupID = systemGroup.ID

	doctorCmd := doctorcmd.NewDoctorCmd()
	doctorCmd.GroupID = systemGroup.ID

	root.AddCommand(
		addCmd,
		connectCmd,
		removeCmd,
		updateCmd,
		renameCmd,
		viewCmd,
		initCmd,
		validateCmd,
		loginCmd,
		exportCmd,
		pullCmd,
		viewsCmd,
		renderCmd,
		inspectCmd,
		listCmd,
		techCmd,
		doctorCmd,
		indexCmd,
		serveCmd,
		stopCmd,
		statusCmd,
		mcpCmd,
		configCmd,
		versionCmd,
	)

	// Add completion and help explicitly to set their GroupID
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	for _, cmd := range root.Commands() {
		if cmd.Name() == "completion" {
			cmd.GroupID = systemGroup.ID
			// Intercept no-argument completion calls to launch the interactive install wizard
			cmd.RunE = func(c *cobra.Command, args []string) error {
				if len(args) == 0 {
					return completion.InstallWizard(c)
				}
				return nil
			}
		} else if cmd.Name() == "help" {
			cmd.GroupID = systemGroup.ID
		}
	}

	for _, option := range options {
		option(root)
	}

	return root
}
