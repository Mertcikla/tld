package rename

import (
	"fmt"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/spf13/cobra"
)

func NewRenameCmd(wdir *string) *cobra.Command {
	var from string
	var to string
	var dryRun bool

	c := &cobra.Command{
		Use:   "rename",
		Short: "Rename an element (YAML ref or database name)",
		Long: `Rename an element.

In a workspace, --from is an element ref and the ref key in elements.yaml is
changed. Without a workspace the element name is renamed in the database; pass
the element's ref, name, or numeric ID as --from.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if from == "" || to == "" {
				return fmt.Errorf("--from and --to are required")
			}
			sess, err := cmdutil.OpenSession(cmd, *wdir, "", "")
			if err != nil {
				return fail(cmd, err)
			}
			defer func() { _ = sess.Close() }()
			if sess.HasWorkspace() {
				return runWorkspaceRename(cmd, *wdir, from, to, dryRun)
			}
			return runDBRename(cmd, sess, from, to, dryRun)
		},
	}

	c.Flags().StringVar(&from, "from", "", "current element ref, name, or ID (required)")
	c.Flags().StringVar(&to, "to", "", "new element ref or name (required)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing")
	_ = c.MarkFlagRequired("from")
	_ = c.MarkFlagRequired("to")

	_ = c.RegisterFlagCompletionFunc("from", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementRefs(wdir)
	})
	return c
}

func fail(cmd *cobra.Command, err error) error {
	if cmdutil.WantsJSONFromCmd(cmd) {
		return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", err)
	}
	return err
}

func runWorkspaceRename(cmd *cobra.Command, wdir, from, to string, dryRun bool) error {
	if dryRun {
		if err := cmdutil.WithWorkspaceDryRun(wdir, func(cloneDir string) error {
			return workspace.RenameElement(cloneDir, from, to)
		}); err != nil {
			return fail(cmd, fmt.Errorf("dry-run rename element: %w", err))
		}
		if cmdutil.WantsJSONFromCmd(cmd) {
			return cmdutil.WriteMutation(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", "dry-run", fmt.Sprintf("%s -> %s", from, to))
		}
		term.Successf(cmd.OutOrStdout(), "dry-run: renamed %s → %s", from, to)
		return nil
	}
	if err := workspace.RenameElement(wdir, from, to); err != nil {
		return fail(cmd, fmt.Errorf("rename element: %w", err))
	}
	if cmdutil.WantsJSONFromCmd(cmd) {
		return cmdutil.WriteMutation(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", "rename", fmt.Sprintf("%s -> %s", from, to))
	}
	term.Successf(cmd.OutOrStdout(), "renamed %s → %s", from, to)
	return nil
}

func runDBRename(cmd *cobra.Command, sess *cmdutil.Session, from, to string, dryRun bool) error {
	to = strings.TrimSpace(to)
	if to == "" {
		return fail(cmd, fmt.Errorf("new name is required"))
	}
	ws, err := sess.LoadWorkspace()
	if err != nil {
		return fail(cmd, err)
	}
	ref, err := cmdutil.ResolveElementArg(ws, from)
	if err != nil {
		return fail(cmd, err)
	}
	elementID, err := elementIDForRef(ws, ref)
	if err != nil {
		return fail(cmd, err)
	}
	if dryRun {
		if cmdutil.WantsJSONFromCmd(cmd) {
			return cmdutil.WriteMutation(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", "dry-run", fmt.Sprintf("%s -> %s", ref, to))
		}
		term.Successf(cmd.OutOrStdout(), "dry-run: rename %s → %s", ref, to)
		return nil
	}
	runner, err := sess.Runner()
	if err != nil {
		return fail(cmd, err)
	}
	ctx := sess.Context(cmd.Context())
	existing, err := runner.GetElement(ctx, elementID)
	if err != nil {
		return fail(cmd, cmdutil.WithUnauthorizedHint("read element failed", err))
	}
	input := elementInputFromProto(existing)
	input.Name = to
	updated, err := runner.UpdateElement(ctx, elementID, input)
	if err != nil {
		return fail(cmd, cmdutil.WithUnauthorizedHint("rename element failed", err))
	}
	_ = updated
	if cmdutil.WantsJSONFromCmd(cmd) {
		return cmdutil.WriteMutation(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", "rename", fmt.Sprintf("%s -> %s", ref, to))
	}
	term.Successf(cmd.OutOrStdout(), "renamed %s → %s (id=%d)", ref, to, elementID)
	return nil
}

func elementIDForRef(ws *workspace.Workspace, ref string) (int32, error) {
	if ws != nil && ws.Meta != nil {
		if m, ok := ws.Meta.Elements[ref]; ok && m != nil && m.ID != 0 {
			return int32(m.ID), nil
		}
	}
	return 0, fmt.Errorf("element %q has no database ID; run 'tld pull' or recreate it", ref)
}

func elementInputFromProto(e *diagv1.Element) api.ElementInput {
	bypass := e.GetBypassNoiseGate()
	return api.ElementInput{
		Name:            e.GetName(),
		Description:     optStr(e.Description),
		Kind:            optStr(e.Kind),
		Technology:      optStr(e.Technology),
		URL:             optStr(e.Url),
		LogoURL:         optStr(e.LogoUrl),
		TechLinks:       e.TechnologyLinks,
		Tags:            e.Tags,
		Repo:            optStr(e.Repo),
		RepositoryID:    optStr(e.RepositoryId),
		Branch:          optStr(e.Branch),
		Language:        optStr(e.Language),
		FilePath:        optStr(e.FilePath),
		BypassNoiseGate: &bypass,
		HasView:         e.GetHasView(),
		ViewLabel:       optStr(e.ViewLabel),
	}
}

func optStr(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}
