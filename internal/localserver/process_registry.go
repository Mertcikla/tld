package localserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

const (
	ProcessKindServer = "server"
	ProcessKindWatch  = "watch"
)

type ProcessRecord struct {
	Kind         string `json:"kind"`
	PID          int    `json:"pid"`
	DataDir      string `json:"data_dir,omitempty"`
	Addr         string `json:"addr,omitempty"`
	RepoRoot     string `json:"repo_root,omitempty"`
	RepositoryID int64  `json:"repository_id,omitempty"`
	StartedAt    string `json:"started_at"`
	UpdatedAt    string `json:"updated_at"`
}

type ProcessRegistry struct {
	Processes []ProcessRecord `json:"processes"`
}

func ProcessRegistryPath() (string, error) {
	dir, err := workspace.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "processes.json"), nil
}

func LoadProcessRegistry() (ProcessRegistry, error) {
	path, err := ProcessRegistryPath()
	if err != nil {
		return ProcessRegistry{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ProcessRegistry{}, nil
		}
		return ProcessRegistry{}, err
	}
	if len(data) == 0 {
		return ProcessRegistry{}, nil
	}
	var reg ProcessRegistry
	if err := json.Unmarshal(data, &reg); err != nil {
		return ProcessRegistry{}, fmt.Errorf("read process registry: %w", err)
	}
	return reg, nil
}

func RegisterProcess(record ProcessRecord) error {
	if record.PID <= 0 {
		return fmt.Errorf("register process: invalid pid %d", record.PID)
	}
	if record.Kind == "" {
		return fmt.Errorf("register process: kind is required")
	}
	reg, err := LoadProcessRegistry()
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	record.UpdatedAt = now
	replaced := false
	next := reg.Processes[:0]
	for _, existing := range reg.Processes {
		if existing.PID == record.PID {
			if record.StartedAt == "" {
				record.StartedAt = existing.StartedAt
			}
			next = append(next, record)
			replaced = true
			continue
		}
		if IsRunning(existing.PID) {
			next = append(next, existing)
		}
	}
	if !replaced {
		if record.StartedAt == "" {
			record.StartedAt = now
		}
		next = append(next, record)
	}
	reg.Processes = next
	return SaveProcessRegistry(reg)
}

func RemoveProcess(pid int) error {
	reg, err := LoadProcessRegistry()
	if err != nil {
		return err
	}
	next := reg.Processes[:0]
	for _, existing := range reg.Processes {
		if existing.PID != pid {
			next = append(next, existing)
		}
	}
	reg.Processes = next
	return SaveProcessRegistry(reg)
}

func PruneProcessRegistry() (ProcessRegistry, error) {
	reg, err := LoadProcessRegistry()
	if err != nil {
		return ProcessRegistry{}, err
	}
	next := reg.Processes[:0]
	for _, existing := range reg.Processes {
		if IsRunning(existing.PID) {
			next = append(next, existing)
		}
	}
	reg.Processes = next
	return reg, SaveProcessRegistry(reg)
}

func SaveProcessRegistry(reg ProcessRegistry) error {
	path, err := ProcessRegistryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".processes-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}
