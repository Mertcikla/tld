package mcp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/mertcikla/tld/v2/cmd/add"
	"github.com/mertcikla/tld/v2/cmd/connect"
	"github.com/mertcikla/tld/v2/cmd/pull"
	"github.com/mertcikla/tld/v2/cmd/remove"
	"github.com/mertcikla/tld/v2/cmd/rename"
	"github.com/mertcikla/tld/v2/cmd/update"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/localserver"
	archwarnings "github.com/mertcikla/tld/v2/internal/warnings"
	"github.com/mertcikla/tld/v2/internal/workspace"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

type addArgs struct {
	Name        string  `json:"name" jsonschema:"element display name (required)"`
	Ref         string  `json:"ref,omitempty" jsonschema:"override generated ref (default: slugified name)"`
	Kind        string  `json:"kind,omitempty" jsonschema:"element kind (default: service)"`
	Description string  `json:"description,omitempty"`
	Technology  string  `json:"technology,omitempty"`
	URL         string  `json:"url,omitempty"`
	Parent      string  `json:"parent,omitempty" jsonschema:"parent element ref (default: root)"`
	PositionX   float64 `json:"position_x,omitempty"`
	PositionY   float64 `json:"position_y,omitempty"`
	ViewLabel   string  `json:"view_label,omitempty" jsonschema:"label for the diagram created when this element becomes a parent"`
}

type connectArgs struct {
	From         string `json:"from" jsonschema:"source element ref (required)"`
	To           string `json:"to" jsonschema:"target element ref (required)"`
	View         string `json:"view,omitempty" jsonschema:"view ref; inferred if empty"`
	Label        string `json:"label,omitempty"`
	Description  string `json:"description,omitempty"`
	Relationship string `json:"relationship,omitempty"`
	Direction    string `json:"direction,omitempty" jsonschema:"forward|backward|both|none"`
	Style        string `json:"style,omitempty"`
	URL          string `json:"url,omitempty"`
}

type removeElementArgs struct {
	Ref string `json:"ref" jsonschema:"element ref to remove"`
}

type removeConnectorArgs struct {
	View string `json:"view"`
	From string `json:"from"`
	To   string `json:"to"`
}

type renameArgs struct {
	From string `json:"from" jsonschema:"current element ref"`
	To   string `json:"to" jsonschema:"new element ref"`
}

type updateElementArgs struct {
	Ref   string `json:"ref"`
	Field string `json:"field"`
	Value string `json:"value"`
}

type updateConnectorArgs struct {
	Ref   string `json:"ref" jsonschema:"connector key e.g. view:source:target[:label]"`
	Field string `json:"field"`
	Value string `json:"value"`
}

type validateArgs struct {
	Strictness int  `json:"strictness,omitempty" jsonschema:"override validation level [1-3]"`
	Verbose    bool `json:"verbose,omitempty"`
}

type pullArgs struct {
	Force  bool `json:"force,omitempty" jsonschema:"overwrite local changes without prompting"`
	DryRun bool `json:"dry_run,omitempty"`
}

type result struct {
	Message string `json:"message"`
}

func textResult(msg string) (*mcpsdk.CallToolResult, result, error) {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: msg}},
	}, result{Message: msg}, nil
}

func errResult(err error) (*mcpsdk.CallToolResult, result, error) {
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: err.Error()}},
	}, result{Message: err.Error()}, nil
}

func registerTools(server *mcpsdk.Server, _ *cobra.Command, wdir, format *string, compact *bool, dataDir string) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_add",
		Description: "Add or update an element (applies immediately to the server). Adding another element with parent=<ref> opens a drill-down diagram (view) for it.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a addArgs) (*mcpsdk.CallToolResult, result, error) {
		c := add.NewAddCmd(wdir, format, compact)
		args := []string{a.Name}
		if a.Ref != "" {
			args = append(args, "--ref", a.Ref)
		}
		if a.Kind != "" {
			args = append(args, "--kind", a.Kind)
		}
		if a.Description != "" {
			args = append(args, "--description", a.Description)
		}
		if a.Technology != "" {
			args = append(args, "--technology", a.Technology)
		}
		if a.URL != "" {
			args = append(args, "--url", a.URL)
		}
		if a.Parent != "" {
			args = append(args, "--parent", a.Parent)
		}
		if a.PositionX != 0 {
			args = append(args, "--position-x", fmt.Sprintf("%v", a.PositionX))
		}
		if a.PositionY != 0 {
			args = append(args, "--position-y", fmt.Sprintf("%v", a.PositionY))
		}
		if a.ViewLabel != "" {
			args = append(args, "--diagram-label", a.ViewLabel)
		}
		if dataDir != "" {
			args = append(args, "--data-dir", dataDir)
		}
		return runSubcommand(ctx, c, args)
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_connect",
		Description: "Add a connector between two elements (applies immediately to the server).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a connectArgs) (*mcpsdk.CallToolResult, result, error) {
		c := connect.NewConnectCmd(wdir, format, compact)
		args := []string{"--from", a.From, "--to", a.To}
		if a.View != "" {
			args = append(args, "--view", a.View)
		}
		if a.Label != "" {
			args = append(args, "--label", a.Label)
		}
		if a.Description != "" {
			args = append(args, "--description", a.Description)
		}
		if a.Relationship != "" {
			args = append(args, "--relationship", a.Relationship)
		}
		if a.Direction != "" {
			args = append(args, "--direction", a.Direction)
		}
		if a.Style != "" {
			args = append(args, "--style", a.Style)
		}
		if a.URL != "" {
			args = append(args, "--url", a.URL)
		}
		if dataDir != "" {
			args = append(args, "--data-dir", dataDir)
		}
		return runSubcommand(ctx, c, args)
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_remove_element",
		Description: "Remove an element (applies immediately to the server).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a removeElementArgs) (*mcpsdk.CallToolResult, result, error) {
		c := remove.NewRemoveCmd(wdir, format, compact)
		args := []string{"element", a.Ref}
		if dataDir != "" {
			args = append(args, "--data-dir", dataDir)
		}
		return runSubcommand(ctx, c, args)
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_remove_connector",
		Description: "Remove matching connector(s) (applies immediately to the server).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a removeConnectorArgs) (*mcpsdk.CallToolResult, result, error) {
		c := remove.NewRemoveCmd(wdir, format, compact)
		args := []string{"connector", "--view", a.View, "--from", a.From, "--to", a.To}
		if dataDir != "" {
			args = append(args, "--data-dir", dataDir)
		}
		return runSubcommand(ctx, c, args)
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_rename",
		Description: "Rename an element; references in connectors and other diagrams are updated.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a renameArgs) (*mcpsdk.CallToolResult, result, error) {
		c := rename.NewRenameCmd(wdir)
		return runSubcommand(ctx, c, []string{"--from", a.From, "--to", a.To})
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_update_element",
		Description: "Update an element field (applies immediately to the server).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a updateElementArgs) (*mcpsdk.CallToolResult, result, error) {
		c := update.NewUpdateCmd(wdir, format, compact)
		args := []string{"element", a.Ref, a.Field, a.Value}
		if dataDir != "" {
			args = append(args, "--data-dir", dataDir)
		}
		return runSubcommand(ctx, c, args)
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_update_connector",
		Description: "Update a connector field (applies immediately to the server).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a updateConnectorArgs) (*mcpsdk.CallToolResult, result, error) {
		c := update.NewUpdateCmd(wdir, format, compact)
		args := []string{"connector", a.Ref, a.Field, a.Value}
		if dataDir != "" {
			args = append(args, "--data-dir", dataDir)
		}
		return runSubcommand(ctx, c, args)
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_validate",
		Description: "Validate workspace YAML files; returns errors, outdated diagrams, and architectural warnings.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a validateArgs) (*mcpsdk.CallToolResult, result, error) {
		ws, err := workspace.Load(*wdir)
		if err != nil {
			return errResult(fmt.Errorf("load workspace: %w", err))
		}
		repoCtx := cmdutil.DetectRepoScope(cmdutil.GetWorkingDir(), *wdir)
		rules := ws.IgnoreRulesForRepository(repoCtx.Name)
		if a.Strictness > 0 {
			ws.Config.Validation.Level = a.Strictness
		}
		out := ""
		if errs := ws.Validate(); len(errs) > 0 {
			out += "Validation errors:\n"
			for _, e := range errs {
				out += "  - " + e.Error() + "\n"
			}
			return errResult(fmt.Errorf("%s%d validation error(s)", out, len(errs)))
		}
		broken := cmdutil.CheckSymbols(ctx, ws, repoCtx, rules)
		if len(broken) > 0 {
			out += "Symbol verification errors:\n"
			for _, m := range broken {
				out += "  - " + m + "\n"
			}
			return errResult(fmt.Errorf("%s%d symbol error(s)", out, len(broken)))
		}
		out += fmt.Sprintf("Workspace valid: %d elements, %d connectors\n", len(ws.Elements), len(ws.Connectors))
		validationWarnings := ws.ValidateWarnings()
		if len(validationWarnings) > 0 {
			out += "\nValidation warnings:\n"
			for _, warning := range validationWarnings {
				out += "  - " + warning.Error() + "\n"
			}
		}
		outdated := cmdutil.CheckOutdated(ws, repoCtx, rules)
		if len(outdated) > 0 {
			out += "\nOutdated diagrams:\n"
			for _, m := range outdated {
				out += "  - " + m + "\n"
			}
		}
		warnings := archwarnings.Analyze(ws)
		if len(warnings) > 0 {
			out += "\nArchitectural warnings:\n"
			for _, w := range warnings {
				if a.Verbose {
					out += fmt.Sprintf("[%s] %s\n%s\n", w.RuleCode, w.RuleName, w.Mediation)
					for _, v := range w.Violations {
						out += "  * " + v + "\n"
					}
				} else {
					out += fmt.Sprintf("[%s] %s (%d violations)\n", w.RuleCode, w.RuleName, len(w.Violations))
				}
			}
		}
		return textResult(out)
	})
}

func runSubcommand(ctx context.Context, c *cobra.Command, args []string) (*mcpsdk.CallToolResult, result, error) {
	var buf bytes.Buffer
	c.SetOut(&buf)
	c.SetErr(&buf)
	c.SetIn(bytes.NewReader(nil))
	c.SetArgs(args)
	err := c.ExecuteContext(ctx)
	out := buf.String()
	if err != nil {
		msg := out
		if msg != "" {
			msg += "\n"
		}
		msg += err.Error()
		return errResult(fmt.Errorf("%s", msg))
	}
	return textResult(out)
}

func addPullTool(server *mcpsdk.Server, wdir *string) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_pull",
		Description: "Pull current server state into local YAML files.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a pullArgs) (*mcpsdk.CallToolResult, result, error) {
		c := pull.NewPullCmd(wdir)
		args := []string{}
		if a.Force {
			args = append(args, "--force")
		}
		if a.DryRun {
			args = append(args, "--dry-run")
		}
		return runSubcommand(ctx, c, args)
	})
}

// ensureServeRunning starts `tld serve` in the background if not already running.
func ensureServeRunning(cmd *cobra.Command, host, port, dataDir string) error {
	addr := localserver.ResolveAddr(localserver.ServeOptions{Host: host, Port: port})
	reg, err := localserver.PruneProcessRegistry()
	if err == nil {
		for _, proc := range reg.Processes {
			if proc.Addr == addr || (proc.Kind == localserver.ProcessKindServer && proc.DataDir == dataDir) {
				return nil
			}
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{"serve"}
	if host != "" {
		args = append(args, "--host", host)
	}
	if port != "" {
		args = append(args, "--port", port)
	}
	if dataDir != "" {
		args = append(args, "--data-dir", dataDir)
	}
	child := exec.Command(exe, args...)
	// Inherit stderr so startup errors surface on the caller; stdout discarded.
	child.Stdout = os.Stderr
	child.Stderr = os.Stderr
	return child.Run()
}

func NewMCPCmd(wdir, format *string, compact *bool) *cobra.Command {
	c := &cobra.Command{
		Use:   "mcp",
		Short: "Run an MCP server over stdio exposing tld CRUD + validate tools",
		Long: `Start a Model Context Protocol server on stdio.

Exposes tld's CRUD commands (add, connect, remove, rename, update) and validate as MCP tools.

If a 'tld serve' instance is already running, only the MCP server is started.
Otherwise, 'tld serve' is launched in the background first, then the MCP server starts on stdio.

Accepts the same --host, --port, --data-dir flags as 'tld serve'.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = workspace.EnsureGlobalConfig()

			host, _ := cmd.Flags().GetString("host")
			port, _ := cmd.Flags().GetString("port")
			dataDirFlag, _ := cmd.Flags().GetString("data-dir")

			cfg, err := workspace.LoadGlobalConfig()
			if err != nil {
				return err
			}
			dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
			if err != nil {
				return err
			}
			serveCfg := workspace.ResolveServeOptions(cfg, host, port)

			if err := ensureServeRunning(cmd, serveCfg.Host, serveCfg.Port, dataDir); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to start tld serve in background: %v\n", err)
			}

			server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "tld", Version: "0.1.0"}, nil)
			registerTools(server, cmd, wdir, format, compact, dataDir)
			addPullTool(server, wdir)

			return server.Run(cmd.Context(), &mcpsdk.StdioTransport{})
		},
	}
	c.Flags().String("host", "", "host address to bind for tld serve (overrides config and env)")
	c.Flags().String("port", "", "port for tld serve (overrides config and env)")
	c.Flags().String("data-dir", "", "data directory for tld serve (overrides config and env)")
	return c
}
