package status

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/term"
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
		Long: `Show running local tlDiagram processes registered by 'tld serve' and 'tld watch'.

For workspace YAML sync state, use 'tld sync status'.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, err := localserver.PruneProcessRegistry()
			if err != nil {
				return err
			}
			items := buildRuntimeStatus(reg.Processes)
			if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
				return writeRuntimeStatusJSON(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", items)
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

func printRuntimeStatus(out interface{ Write([]byte) (int, error) }, items []runtimeStatusItem) {
	if len(items) == 0 {
		term.Info(out, "No tld processes running.")
		term.Hint(out, "Run 'tld serve' to start the local server")
		return
	}
	for i, item := range items {
		if i > 0 {
			term.Separator(out)
		}
		term.Label(out, 16, printableKind(item.Kind), "running")
		term.Label(out, 16, "PID", fmt.Sprintf("%d", item.PID))
		if item.URL != "" {
			term.Label(out, 16, "URL", term.URL(out, item.URL))
		}
		if item.Ready != nil {
			term.Label(out, 16, "Ready", printableBool(*item.Ready))
		}
		if item.Resources != nil {
			term.Label(out, 16, "Resources", fmt.Sprintf("%d views, %d elements, %d connectors", item.Resources.Views, item.Resources.Elements, item.Resources.Connectors))
		}
		if item.RepoRoot != "" {
			term.Label(out, 16, "Repo", term.Path(out, item.RepoRoot))
		}
		if item.RepositoryID != 0 {
			term.Label(out, 16, "Repository ID", fmt.Sprintf("%d", item.RepositoryID))
		}
		if item.DataDir != "" {
			term.Label(out, 16, "Data dir", term.Path(out, item.DataDir))
		}
		if item.DBPath != "" {
			term.Label(out, 16, "DB", term.Path(out, item.DBPath))
		}
		if item.DBSize > 0 {
			term.Label(out, 16, "DB size", humanBytes(item.DBSize))
		}
		if item.DBModifiedAt != "" {
			term.Label(out, 16, "DB modified", item.DBModifiedAt)
		}
		if item.StartedAt != "" {
			term.Label(out, 16, "Started", item.StartedAt)
		}
	}
	term.Separator(out)
	term.Hint(out, "Run 'tld stop' to shut down registered processes")
}

func writeRuntimeStatusJSON(out interface{ Write([]byte) (int, error) }, compact bool, items []runtimeStatusItem) error {
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
	case localserver.ProcessKindWatch:
		return "Watch"
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

func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
