package cmd

import (
	"fmt"
	"net/url"
	"path/filepath"
	"sort"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func newContextCmd(wdir *string) *cobra.Command {
	c := &cobra.Command{
		Use: "context", Short: "Show the effective workspace, target, destination, and read source",
		GroupID: "workspace", Args: cobra.NoArgs,
		Example: "  tld context --format json\n  tld context --target local --data-dir ./data",
		RunE: func(cmd *cobra.Command, _ []string) error {
			sess, err := cmdutil.OpenSession(cmd, *wdir, "", "")
			if err != nil {
				return err
			}
			defer func() { _ = sess.Close() }()
			target, dataDir := cmdutil.TargetOverrides(cmd, "", "")
			target, err = exec.ResolveTarget(sess.Config, target)
			if err != nil {
				return err
			}
			source, err := cmdutil.ReadSource(cmd, sess.HasWorkspace())
			if err != nil {
				return err
			}
			info := map[string]any{"target": target, "read_source": source, "mode": "database"}
			if sess.HasWorkspace() {
				path, err := filepath.Abs(sess.Wdir)
				if err != nil {
					return err
				}
				info["workspace"], info["mode"] = path, "workspace"
			}
			if target == exec.TargetRemote {
				info["destination"] = publicDestination(sess.Config.ServerURL)
				info["org_id"] = sess.Config.WorkspaceID
			} else {
				dataDir, err = workspace.ResolveDataDir(&sess.Config, dataDir)
				if err != nil {
					return err
				}
				dataDir, err = filepath.Abs(dataDir)
				if err != nil {
					return err
				}
				info["data_dir"], info["database_driver"] = dataDir, sess.Config.Database.Driver
				info["destination"] = localserver.DatabasePath(dataDir)
				if sess.Config.Database.Driver == "postgres" {
					info["destination"] = publicDestination(sess.Config.Database.DatabaseURL)
				}
			}
			if cmdutil.WantsJSONFromCmd(cmd) {
				return cmdutil.WriteJSON(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), cmdutil.JSONOutput{Command: "context", Status: "ok", Extra: info})
			}
			keys := make([]string, 0, len(info))
			for key := range info {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %v\n", key, info[key])
			}
			return nil
		},
	}
	cmdutil.AddReadSourceFlag(c)
	return c
}

// Never display credentials or connection-string options in a context result.
func publicDestination(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" {
		return "configured server"
	}
	u.User, u.RawQuery, u.Fragment = nil, "", ""
	return u.String()
}
