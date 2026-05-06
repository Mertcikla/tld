package mcp

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/mertcikla/tld/cmd/apply"
	"github.com/mertcikla/tld/cmd/plan"
	"github.com/mertcikla/tld/cmd/pull"
	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/localserver"
	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/workspace"
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
	ViewLabel   string  `json:"view_label,omitempty"`
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

type planArgs struct {
	RecreateIDs bool   `json:"recreate_ids,omitempty"`
	Verbose     bool   `json:"verbose,omitempty"`
	Strictness  int    `json:"strictness,omitempty"`
	Output      string `json:"output,omitempty" jsonschema:"write plan to file instead of returned text"`
}

type applyArgs struct {
	Force       bool `json:"force,omitempty" jsonschema:"auto-confirm prompts"`
	RecreateIDs bool `json:"recreate_ids,omitempty"`
	Verbose     bool `json:"verbose,omitempty"`
	Debug       bool `json:"debug,omitempty"`
	ForceApply  bool `json:"force_apply,omitempty" jsonschema:"override server conflict checks"`
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

func registerTools(server *mcpsdk.Server, wdir *string) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_add",
		Description: "Add or update an element in elements.yaml.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a addArgs) (*mcpsdk.CallToolResult, result, error) {
		ref := a.Ref
		if ref == "" {
			ref = workspace.Slugify(a.Name)
		}
		kind := a.Kind
		if kind == "" {
			kind = "service"
		}
		parent := a.Parent
		if parent == "" {
			parent = "root"
		}
		spec := &workspace.Element{
			Name:        a.Name,
			Kind:        kind,
			Description: a.Description,
			Technology:  a.Technology,
			URL:         a.URL,
			HasView:     true,
			ViewLabel:   a.ViewLabel,
			Placements: []workspace.ViewPlacement{{
				ParentRef: parent,
				PositionX: a.PositionX,
				PositionY: a.PositionY,
			}},
		}
		if err := workspace.UpsertElement(*wdir, ref, spec); err != nil {
			return errResult(fmt.Errorf("upsert element: %w", err))
		}
		return textResult(fmt.Sprintf("upserted element %s", ref))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_connect",
		Description: "Add a connector between two elements in connectors.yaml.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a connectArgs) (*mcpsdk.CallToolResult, result, error) {
		view := a.View
		if view == "" {
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return errResult(fmt.Errorf("load workspace: %w", err))
			}
			view = inferView(ws, a.From, a.To)
		}
		direction := a.Direction
		if direction == "" {
			direction = "forward"
		}
		style := a.Style
		if style == "" {
			style = "bezier"
		}
		spec := &workspace.Connector{
			View:         view,
			Source:       a.From,
			Target:       a.To,
			Label:        a.Label,
			Description:  a.Description,
			Relationship: a.Relationship,
			Direction:    direction,
			Style:        style,
			URL:          a.URL,
		}
		if err := workspace.AppendConnector(*wdir, spec); err != nil {
			return errResult(fmt.Errorf("append connector: %w", err))
		}
		return textResult(fmt.Sprintf("connector %s -> %s in view %s", a.From, a.To, view))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_remove_element",
		Description: "Remove an element from elements.yaml.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a removeElementArgs) (*mcpsdk.CallToolResult, result, error) {
		if err := workspace.RemoveElement(*wdir, a.Ref); err != nil {
			return errResult(fmt.Errorf("remove element: %w", err))
		}
		return textResult(fmt.Sprintf("removed element %s", a.Ref))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_remove_connector",
		Description: "Remove matching connector(s) from connectors.yaml.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a removeConnectorArgs) (*mcpsdk.CallToolResult, result, error) {
		n, err := workspace.RemoveConnector(*wdir, a.View, a.From, a.To)
		if err != nil {
			return errResult(fmt.Errorf("remove connector: %w", err))
		}
		return textResult(fmt.Sprintf("removed %d connector(s)", n))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_rename",
		Description: "Rename an element; references in connectors and other diagrams are updated.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a renameArgs) (*mcpsdk.CallToolResult, result, error) {
		if err := workspace.RenameElement(*wdir, a.From, a.To); err != nil {
			return errResult(fmt.Errorf("rename element: %w", err))
		}
		return textResult(fmt.Sprintf("renamed %s -> %s", a.From, a.To))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_update_element",
		Description: "Update an element field.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a updateElementArgs) (*mcpsdk.CallToolResult, result, error) {
		if err := workspace.UpdateElementField(*wdir, a.Ref, a.Field, a.Value); err != nil {
			return errResult(fmt.Errorf("update element: %w", err))
		}
		return textResult(fmt.Sprintf("updated element %s: %s=%q", a.Ref, a.Field, a.Value))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_update_connector",
		Description: "Update a connector field.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a updateConnectorArgs) (*mcpsdk.CallToolResult, result, error) {
		if err := workspace.UpdateConnectorField(*wdir, a.Ref, a.Field, a.Value); err != nil {
			return errResult(fmt.Errorf("update connector: %w", err))
		}
		return textResult(fmt.Sprintf("updated connector %s: %s=%q", a.Ref, a.Field, a.Value))
	})

	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_validate",
		Description: "Validate workspace YAML files; returns errors and architectural warnings.",
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
		warnings := planner.AnalyzePlan(ws)
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

func addPlanTool(server *mcpsdk.Server, wdir *string) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_plan",
		Description: "Show what would be applied (server dry-run with conflict/drift detection).",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a planArgs) (*mcpsdk.CallToolResult, result, error) {
		c := plan.NewPlanCmd(wdir)
		args := []string{}
		if a.RecreateIDs {
			args = append(args, "--recreate-ids")
		}
		if a.Verbose {
			args = append(args, "--verbose")
		}
		if a.Strictness > 0 {
			args = append(args, "--strictness", strconv.Itoa(a.Strictness))
		}
		if a.Output != "" {
			args = append(args, "--output", a.Output)
		}
		return runSubcommand(ctx, c, args)
	})
}

func addApplyTool(server *mcpsdk.Server, wdir *string) {
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "tld_apply",
		Description: "Apply pending workspace changes to tldiagram.com.",
	}, func(ctx context.Context, _ *mcpsdk.CallToolRequest, a applyArgs) (*mcpsdk.CallToolResult, result, error) {
		c := apply.NewApplyCmd(wdir)
		args := []string{}
		if a.Force {
			args = append(args, "--force")
		}
		if a.RecreateIDs {
			args = append(args, "--recreate-ids")
		}
		if a.Verbose {
			args = append(args, "--verbose")
		}
		if a.Debug {
			args = append(args, "--debug")
		}
		if a.ForceApply {
			args = append(args, "--force-apply")
		}
		return runSubcommand(ctx, c, args)
	})
}

func inferView(ws *workspace.Workspace, from, to string) string {
	if ws == nil {
		return "root"
	}
	fromEl, okF := ws.Elements[from]
	toEl, okT := ws.Elements[to]
	if !okF || !okT {
		return "root"
	}
	parents := func(e *workspace.Element) []string {
		if e == nil || len(e.Placements) == 0 {
			return []string{"root"}
		}
		out := make([]string, 0, len(e.Placements))
		for _, p := range e.Placements {
			parent := p.ParentRef
			if parent == "" {
				parent = "root"
			}
			out = append(out, parent)
		}
		return out
	}
	fp := parents(fromEl)
	tp := parents(toEl)
	for _, f := range fp {
		for _, t := range tp {
			if f == t {
				return f
			}
		}
	}
	return "root"
}

// ensureServeRunning starts `tld serve` in the background if not already running.
func ensureServeRunning(cmd *cobra.Command, host, port, dataDir string) error {
	pid, err := localserver.ReadPID(localserver.PIDPath(dataDir))
	if err == nil && localserver.IsRunning(pid) {
		return nil
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
			registerTools(server, wdir)
			addPullTool(server, wdir)
			addPlanTool(server, wdir)
			addApplyTool(server, wdir)

			return server.Run(cmd.Context(), &mcpsdk.StdioTransport{})
		},
	}
	c.Flags().String("host", "", "host address to bind for tld serve (overrides config and env)")
	c.Flags().String("port", "", "port for tld serve (overrides config and env)")
	c.Flags().String("data-dir", "", "data directory for tld serve (overrides config and env)")
	return c
}
