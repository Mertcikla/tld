package cmd

import (
	"fmt"
	"os"

	"github.com/mertcikla/tld/cmd/add"
	"github.com/mertcikla/tld/cmd/analyze"
	"github.com/mertcikla/tld/cmd/apply"
	"github.com/mertcikla/tld/cmd/check"
	configcmd "github.com/mertcikla/tld/cmd/config"
	"github.com/mertcikla/tld/cmd/connect"
	"github.com/mertcikla/tld/cmd/diff"
	"github.com/mertcikla/tld/cmd/export"
	"github.com/mertcikla/tld/cmd/initialize"
	"github.com/mertcikla/tld/cmd/login"
	"github.com/mertcikla/tld/cmd/mcp"
	"github.com/mertcikla/tld/cmd/plan"
	"github.com/mertcikla/tld/cmd/pull"
	"github.com/mertcikla/tld/cmd/remove"
	"github.com/mertcikla/tld/cmd/rename"
	"github.com/mertcikla/tld/cmd/serve"
	"github.com/mertcikla/tld/cmd/status"
	"github.com/mertcikla/tld/cmd/stop"
	"github.com/mertcikla/tld/cmd/update"
	"github.com/mertcikla/tld/cmd/validate"
	"github.com/mertcikla/tld/cmd/version"
	"github.com/mertcikla/tld/cmd/views"
	watchcmd "github.com/mertcikla/tld/cmd/watch"
	"github.com/mertcikla/tld/internal/completion"
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
		Use:   "tld",
		Short: "tld -- tlDiagram CLI",
		Long: `tld manages software architecture diagrams as code.

Define your architecture in YAML, preview changes with 'tld plan',
and apply them atomically with 'tld apply'.`,
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

	// Define groups
	resourceGroup := &cobra.Group{
		ID:    "resource",
		Title: "CRUD actions on resources:",
	}
	secondaryGroup := &cobra.Group{
		ID:    "secondary",
		Title: "Secondary actions:",
	}
	root.AddGroup(resourceGroup, secondaryGroup)

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

	// Secondary Commands
	initCmd := initialize.NewInitCmd()
	initCmd.GroupID = secondaryGroup.ID

	loginCmd := login.NewLoginCmd(&wdir)
	loginCmd.GroupID = secondaryGroup.ID

	validateCmd := validate.NewValidateCmd(&wdir)
	validateCmd.GroupID = secondaryGroup.ID

	planCmd := plan.NewPlanCmd(&wdir)
	planCmd.GroupID = secondaryGroup.ID

	applyCmd := apply.NewApplyCmd(&wdir)
	applyCmd.GroupID = secondaryGroup.ID

	exportCmd := export.NewExportCmd(&wdir)
	exportCmd.GroupID = secondaryGroup.ID

	pullCmd := pull.NewPullCmd(&wdir)
	pullCmd.GroupID = secondaryGroup.ID

	statusCmd := status.NewStatusCmd(&wdir)
	statusCmd.GroupID = secondaryGroup.ID

	viewsCmd := views.NewViewsCmd(&wdir)
	viewsCmd.GroupID = secondaryGroup.ID

	diffCmd := diff.NewDiffCmd(&wdir)
	diffCmd.GroupID = secondaryGroup.ID

	versionCmd := version.NewVersionCmd()
	versionCmd.GroupID = secondaryGroup.ID

	analyzeCmd := analyze.NewAnalyzeCmd(&wdir)
	analyzeCmd.GroupID = secondaryGroup.ID

	checkCmd := check.NewCheckCmd(&wdir)
	checkCmd.GroupID = secondaryGroup.ID

	configCmd := configcmd.NewConfigCmd()
	configCmd.GroupID = secondaryGroup.ID

	watchCmd := watchcmd.NewWatchCmd()
	watchCmd.GroupID = secondaryGroup.ID

	serveCmd := serve.NewServeCmd(nil)
	serveCmd.GroupID = secondaryGroup.ID

	mcpCmd := mcp.NewMCPCmd(&wdir, &outputFormat, &compactJSON)
	mcpCmd.GroupID = secondaryGroup.ID

	stopCmd := stop.NewStopCmd()
	stopCmd.GroupID = secondaryGroup.ID

	root.AddCommand(
		initCmd,
		loginCmd,
		validateCmd,
		planCmd,
		applyCmd,
		exportCmd,
		pullCmd,
		statusCmd,
		viewsCmd,
		diffCmd,
		addCmd,
		connectCmd,
		removeCmd,
		updateCmd,
		renameCmd,
		analyzeCmd,
		checkCmd,
		configCmd,
		watchCmd,
		serveCmd,
		mcpCmd,
		stopCmd,
		versionCmd,
	)

	// Add completion and help explicitly to set their GroupID
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	for _, cmd := range root.Commands() {
		if cmd.Name() == "completion" {
			cmd.GroupID = secondaryGroup.ID
			// Intercept no-argument completion calls to launch the interactive install wizard
			cmd.RunE = func(c *cobra.Command, args []string) error {
				if len(args) == 0 {
					return completion.InstallWizard(c)
				}
				return nil
			}
		} else if cmd.Name() == "help" {
			cmd.GroupID = secondaryGroup.ID
		}
	}

	for _, option := range options {
		option(root)
	}

	return root
}
