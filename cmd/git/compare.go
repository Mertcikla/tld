package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/codeindex/configbridge"
	"github.com/mertcikla/tld/v2/internal/codeindex/identity"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/indexer"
	"github.com/mertcikla/tld/v2/internal/codeindex/remote"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/mermaid"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
)

type compareOptions struct {
	mermaid  bool
	markdown bool
	radius   uint32
	depth    uint32
	maxNodes int
	maxBytes int
	dataDir  string
}

// protoJSONOptions keeps zero-value fields explicit so downstream consumers can
// read distances and counts without knowing proto3 defaults.
var protoJSONOptions = protojson.MarshalOptions{Multiline: true, Indent: "  ", EmitUnpopulated: true}

func newCompareCmd() *cobra.Command {
	opts := compareOptions{}
	c := &cobra.Command{
		Use:   "compare [repository] <base> <head>",
		Short: "Compare two revisions of a repository",
		Long: `Compare two Git revisions and emit the repository impact diagram.

The repository may be omitted to use the current checkout, or given as a
repository id, a local path, or a remote URL (github.com/owner/repo,
owner/repo, or a Git URL). Missing snapshots are indexed on demand.

The default output is the protojson encoding of the impact diagram. Pass
--mermaid to emit the same Mermaid change diagram the web UI renders.

When the diagram exceeds the node budget the output is progressively narrowed
by blast radius and a warning is written to stderr.`,
		Args: cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 3 {
				target = args[0]
			}
			return runCompare(cmd, opts, target, args[len(args)-2], args[len(args)-1])
		},
	}
	c.Flags().BoolVar(&opts.mermaid, "mermaid", false, "emit the Mermaid change diagram instead of protojson")
	c.Flags().BoolVar(&opts.markdown, "markdown", false, "wrap the Mermaid diagram in a Markdown code fence")
	c.Flags().Uint32Var(&opts.radius, "radius", 0, "blast radius of unchanged context to include (0 = direct changes only)")
	c.Flags().Uint32Var(&opts.depth, "depth", impact.DefaultContextDepth, "maximum dependency hops of context to compute")
	c.Flags().IntVar(&opts.maxNodes, "max-nodes", impact.DefaultMaxNodes, "node budget; the blast radius is narrowed when exceeded (0 disables)")
	c.Flags().IntVar(&opts.maxBytes, "max-bytes", 2<<20, "protojson byte budget; the blast radius is narrowed when exceeded (0 disables)")
	c.Flags().StringVar(&opts.dataDir, "data-dir", "", "override the data directory")
	return c
}

func runCompare(cmd *cobra.Command, opts compareOptions, target, base, head string) error {
	ctx := cmd.Context()
	global, err := workspace.LoadGlobalConfig()
	if err != nil {
		return err
	}
	dataDir, err := workspace.ResolveDataDir(global, opts.dataDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}
	sq, err := localstore.OpenLocal(ctx, global, dataDir, assets.FS)
	if err != nil {
		return err
	}
	defer func() { _ = sq.Close() }()
	store := cstore.NewStore(sq.DB(), sq.BunDB(), sq.Dialect())

	repositoryID, err := resolveRepository(ctx, store, dataDir, target)
	if err != nil {
		return err
	}
	service := impact.Service{Workspace: sq, Index: store, Config: configbridge.FromGlobal(global)}
	diagram, err := service.Compare(ctx, impact.CompareRequest{
		RepositoryID: repositoryID,
		Base:         &pb.ComparisonTarget{GitRevision: base},
		Head:         &pb.ComparisonTarget{GitRevision: head},
		ContextDepth: opts.depth,
		Progress:     progressPrinter(cmd),
	})
	if err != nil {
		return err
	}
	requested := min(opts.radius, opts.depth, diagram.GetMaxRadius())
	scoped, radius, limited := scopeToBudget(diagram, requested, opts)
	if limited {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"warning: output limited to blast radius %d of %d (%d nodes); pass --radius %d or raise --max-nodes to include more\n",
			radius, requested, len(scoped.GetNodes()), requested)
	}
	out := cmd.OutOrStdout()
	if opts.mermaid || opts.markdown {
		code := mermaid.ExportImpactDiagram(scoped, mermaid.ImpactExportOptions{IncludeMetadata: true, Radius: radius})
		if opts.markdown {
			_, err = fmt.Fprint(out, mermaid.MermaidBlock(code))
			return err
		}
		_, err = fmt.Fprint(out, code)
		return err
	}
	payload, err := protoJSONOptions.Marshal(scoped)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(payload))
	return err
}

// scopeToBudget narrows the diagram by blast radius until it fits the node and
// byte budgets, returning the scoped diagram and the radius it used.
func scopeToBudget(diagram *pb.ImpactDiagram, requested uint32, opts compareOptions) (*pb.ImpactDiagram, uint32, bool) {
	radius, limited := impact.FitRadius(diagram, requested, opts.maxNodes)
	scoped := impact.Scope(diagram, radius)
	if opts.maxBytes <= 0 {
		return scoped, radius, limited
	}
	for radius > 0 {
		payload, err := protoJSONOptions.Marshal(scoped)
		if err != nil || len(payload) <= opts.maxBytes {
			break
		}
		radius--
		limited = true
		scoped = impact.Scope(diagram, radius)
	}
	return scoped, radius, limited
}

func progressPrinter(cmd *cobra.Command) indexer.ProgressFunc {
	last := ""
	return func(p indexer.Progress) {
		stage := strings.TrimSpace(p.Stage)
		if stage == "" || stage == last {
			return
		}
		last = stage
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), stage)
	}
}

// resolveRepository maps a repository id, local path, or remote URL to the
// stable repository id, cloning managed checkouts and registering identities
// the same way `tld index` does.
func resolveRepository(ctx context.Context, store *cstore.Store, dataDir, target string) (string, error) {
	cleaned := strings.TrimSpace(target)
	if cleaned == "" {
		return "", fmt.Errorf("repository is required")
	}
	if spec, isRemote := remoteTarget(cleaned); isRemote {
		root := remote.ManagedDir(dataDir, spec, uuid.Nil)
		_, _ = fmt.Fprintf(os.Stderr, "cloning %s into %s\n", spec.WebURL, root)
		if err := remote.Clone(ctx, spec, root); err != nil {
			return "", err
		}
		resolved, err := identity.Apply(ctx, store, root, "", spec.WebURL, true)
		if err != nil {
			return "", err
		}
		return resolved.ID, nil
	}
	if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
		root, err := filepath.Abs(cleaned)
		if err != nil {
			return "", err
		}
		if root, err = filepath.EvalSymlinks(root); err != nil {
			return "", err
		}
		resolved, err := identity.Apply(ctx, store, root, "", "", false)
		if err != nil {
			return "", err
		}
		return resolved.ID, nil
	}
	repo, err := store.Repository(ctx, cleaned)
	if err != nil {
		return "", fmt.Errorf("repository %q is neither a local path nor a known repository id: %w", cleaned, err)
	}
	if strings.TrimSpace(repo.GetRoot()) == "" {
		return "", fmt.Errorf("repository %q has no checkout; run `tld index <path|url>` first", cleaned)
	}
	if info, err := os.Stat(repo.GetRoot()); err != nil || !info.IsDir() {
		return "", fmt.Errorf("repository checkout %q is unavailable; run `tld index <path|url>` again", repo.GetRoot())
	}
	return repo.GetId(), nil
}

// remoteTarget mirrors the index command's remote detection: explicit remote
// syntax always wins, while owner/repo shorthand only counts when no local
// directory matches.
func remoteTarget(raw string) (remote.Spec, bool) {
	cleaned := strings.TrimSpace(raw)
	explicit := strings.Contains(cleaned, "://") || strings.HasPrefix(cleaned, "git@") ||
		strings.HasPrefix(cleaned, "github.com/") || strings.HasSuffix(cleaned, ".git")
	if !explicit {
		if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
			return remote.Spec{}, false
		}
	}
	spec, err := remote.Parse(cleaned)
	if err != nil {
		return remote.Spec{}, false
	}
	return spec, true
}
