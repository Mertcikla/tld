package status

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/mertcikla/tld/v2/cmd/version"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/runtimeinfo"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

const readyRequestTimeout = 500 * time.Millisecond

type runtimeStatusOutput struct {
	Command   string              `json:"command"`
	Status    string              `json:"status"`
	Processes []runtimeStatusItem `json:"processes"`
}

type runtimeStatusItem struct {
	Kind         string `json:"kind"`
	PID          int    `json:"pid"`
	URL          string `json:"url,omitempty"`
	Ready        *bool  `json:"ready,omitempty"`
	DataDir      string `json:"data_dir,omitempty"`
	DBPath       string `json:"db_path,omitempty"`
	DBSize       int64  `json:"db_size,omitempty"`
	DBModifiedAt string `json:"db_modified_at,omitempty"`
	RepoRoot     string `json:"repo_root,omitempty"`
	RepositoryID int64  `json:"repository_id,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	Resources    *struct {
		Views      int `json:"views"`
		Elements   int `json:"elements"`
		Connectors int `json:"connectors"`
	} `json:"resources,omitempty"`
}

func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show running local tlDiagram processes",
		Long: `Show running local tlDiagram processes registered by 'tld serve'.

To refresh the workspace YAML cache from the server, use 'tld pull'.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, err := localserver.PruneProcessRegistry()
			if err != nil {
				return err
			}
			items := buildRuntimeStatus(reg.Processes)
			if cmdutil.WantsJSONFromCmd(cmd) {
				return writeRuntimeStatusJSON(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), items)
			}
			printRuntimeStatus(cmd.OutOrStdout(), items)
			return nil
		},
	}
}

func buildRuntimeStatus(processes []localserver.ProcessRecord) []runtimeStatusItem {
	items := make([]runtimeStatusItem, 0, len(processes))
	for _, proc := range processes {
		item := runtimeStatusItem{
			Kind:         proc.Kind,
			PID:          proc.PID,
			DataDir:      proc.DataDir,
			RepoRoot:     proc.RepoRoot,
			RepositoryID: proc.RepositoryID,
			StartedAt:    proc.StartedAt,
			UpdatedAt:    proc.UpdatedAt,
		}
		if proc.Addr != "" {
			item.URL = "http://" + proc.Addr
			if ready, err := getReady(item.URL + "/api/ready"); err == nil {
				ok := ready.OK
				item.Ready = &ok
				item.Resources = &struct {
					Views      int `json:"views"`
					Elements   int `json:"elements"`
					Connectors int `json:"connectors"`
				}{
					Views:      ready.Resources.Views,
					Elements:   ready.Resources.Elements,
					Connectors: ready.Resources.Connectors,
				}
			} else {
				ok := false
				item.Ready = &ok
			}
		}
		if proc.DataDir != "" {
			item.DBPath = localserver.DatabasePath(proc.DataDir)
			if info, err := os.Stat(item.DBPath); err == nil {
				item.DBSize = info.Size()
				item.DBModifiedAt = info.ModTime().Format(time.RFC3339)
			}
		}
		items = append(items, item)
	}
	return items
}

func printRuntimeStatus(out io.Writer, items []runtimeStatusItem) {
	term.Label(out, term.DefaultLabelWidth, "Version", version.Version)
	term.Separator(out)
	if len(items) == 0 {
		term.Info(out, "No tld processes running.")
		term.Hint(out, "Run 'tld serve' to start the local server")
		term.Separator(out)
		printDataEnvironment(out)
		term.Separator(out)
		term.Label(out, term.DefaultLabelWidth, "Config path", term.Path(out, configPath()))
		return
	}
	for i, item := range items {
		if i > 0 {
			term.Separator(out)
		}
		term.Label(out, term.DefaultLabelWidth, printableKind(item.Kind), "running")
		term.Label(out, term.DefaultLabelWidth, "PID", fmt.Sprintf("%d", item.PID))
		if item.URL != "" {
			term.Label(out, term.DefaultLabelWidth, "URL", term.URL(out, item.URL))
		}
		if item.Ready != nil {
			term.Label(out, term.DefaultLabelWidth, "Ready", printableBool(*item.Ready))
		}
		if item.Resources != nil {
			term.Label(out, term.DefaultLabelWidth, "Resources", runtimeinfo.ResourceCounts{
				Views:      item.Resources.Views,
				Elements:   item.Resources.Elements,
				Connectors: item.Resources.Connectors,
			}.Format())
		}
		if item.RepoRoot != "" {
			term.Label(out, term.DefaultLabelWidth, "Repo", term.Path(out, item.RepoRoot))
		}
		if item.RepositoryID != 0 {
			term.Label(out, term.DefaultLabelWidth, "Repository ID", fmt.Sprintf("%d", item.RepositoryID))
		}
		if item.StartedAt != "" {
			term.Label(out, term.DefaultLabelWidth, "Started", item.StartedAt)
		}
		runtimeinfo.PrintStorage(out, runtimeinfo.Storage{
			DataDir:      item.DataDir,
			DBPath:       item.DBPath,
			DBSize:       item.DBSize,
			DBModifiedAt: item.DBModifiedAt,
		})
	}
	term.Separator(out)
	term.Label(out, term.DefaultLabelWidth, "Config path", term.Path(out, configPath()))
	term.Hint(out, "Run 'tld stop' to shut down registered processes")
}

func configPath() string {
	path, _ := workspace.ExistingGlobalConfigPath()
	return path
}

func printDataEnvironment(out io.Writer) {
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		return
	}
	dataDir, err := workspace.ResolveDataDir(cfg, "")
	if err != nil {
		return
	}
	storage := runtimeinfo.Storage{DataDir: dataDir, DBDriver: cfg.Database.Driver}
	if runtimeinfo.NormalizeDBDriver(cfg.Database.Driver) == "sqlite" {
		dbPath := localserver.DatabasePath(dataDir)
		storage.DBPath = dbPath
		if info, err := os.Stat(dbPath); err == nil {
			storage.DBSize = info.Size()
			storage.DBModifiedAt = info.ModTime().Format(time.RFC3339)
		}
	}
	runtimeinfo.PrintStorage(out, storage)
}

func writeRuntimeStatusJSON(out io.Writer, compact bool, items []runtimeStatusItem) error {
	status := "stopped"
	if len(items) > 0 {
		status = "running"
	}
	enc := json.NewEncoder(out)
	if !compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(runtimeStatusOutput{
		Command:   "status",
		Status:    status,
		Processes: items,
	})
}

type readyInfo struct {
	OK        bool `json:"ok"`
	Resources struct {
		Views      int `json:"views"`
		Elements   int `json:"elements"`
		Connectors int `json:"connectors"`
	} `json:"resources"`
}

func getReady(url string) (*readyInfo, error) {
	client := &http.Client{Timeout: readyRequestTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ready status %d", resp.StatusCode)
	}
	var ready readyInfo
	if err := json.NewDecoder(resp.Body).Decode(&ready); err != nil {
		return nil, err
	}
	return &ready, nil
}

func printableKind(kind string) string {
	switch kind {
	case localserver.ProcessKindServer:
		return "Server"
	default:
		return "Process"
	}
}

func printableBool(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
