package initialize

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mertcikla/tld/internal/git"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var defaultWorkspaceExclude = []string{
	"vendor/",
	"node_modules/",
	".venv/",
	".git/",
	"**/*_test.go",
	"**/*.pb.go",
}

func generateDefaultWorkspaceConfig(dir string) ([]byte, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	parentDir := filepath.Dir(absDir)
	defaultProjectName := filepath.Base(parentDir)

	config := workspace.WorkspaceConfig{
		ProjectName: defaultProjectName,
		Exclude:     append([]string{}, defaultWorkspaceExclude...),
	}

	// Attempt to detect git remote
	if remoteURL, err := git.DetectRemoteURL(parentDir); err == nil {
		config.Repositories = map[string]workspace.Repository{
			defaultProjectName: {
				URL:      remoteURL,
				LocalDir: "", // Assumes parent directory
				Config: &workspace.RepositoryConfig{
					Mode: "upsert",
				},
			},
		}
	}

	return yaml.Marshal(&config)
}

func NewInitCmd() *cobra.Command {
	var wizard bool
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Initialize a new tld workspace",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := ".tld"
			if len(args) > 0 {
				dir = args[0]
			}

			if err := os.MkdirAll(dir, 0750); err != nil {
				return fmt.Errorf("create %s: %w", dir, err)
			}

			// Create empty YAML files if they don't exist
			files := map[string]string{
				"elements.yaml":   "{}\n",
				"connectors.yaml": "{}\n",
			}
			for f, content := range files {
				path := filepath.Join(dir, f)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					if err := os.WriteFile(path, []byte(content), 0600); err != nil {
						return fmt.Errorf("create %s: %w", f, err)
					}
				}
			}

			workspaceConfigPath := filepath.Join(dir, ".tld.yaml")
			if wizard {
				if err := runInitWizard(cmd, dir); err != nil {
					return err
				}
			} else if _, err := os.Stat(workspaceConfigPath); os.IsNotExist(err) {
				data, err := generateDefaultWorkspaceConfig(dir)
				if err != nil {
					return fmt.Errorf("generate default config: %w", err)
				}
				if err := os.WriteFile(workspaceConfigPath, data, 0600); err != nil {
					return fmt.Errorf("create .tld.yaml: %w", err)
				}
			} else if err != nil {
				return fmt.Errorf("stat .tld.yaml: %w", err)
			}

			cfgPath, err := workspace.ConfigPath()
			if err != nil {
				return fmt.Errorf("get config path: %w", err)
			}

			if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
				return fmt.Errorf("create config dir: %w", err)
			}

			if _, err := os.Stat(cfgPath); err == nil {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized workspace at %s (global config already exists at %s)\n", dir, cfgPath)
			} else {
				cfg := `# tld global configuration
server_url: https://tldiagram.com
api_key: ""        # or set TLD_API_KEY env var
org_id: ""         # UUID of your organisation
`
				if err := os.WriteFile(cfgPath, []byte(cfg), 0600); err != nil {
					return fmt.Errorf("write tld.yaml: %w", err)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Initialized workspace at %s\n", dir)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Global configuration file created: %s\n", cfgPath)
			}

			if !wizard {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Run `tld login` to authenticate with tldiagram.com")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&wizard, "wizard", false, "run interactive setup wizard")
	return cmd
}

func runInitWizard(cmd *cobra.Command, dir string) error {
	scanner := bufio.NewScanner(cmd.InOrStdin())

	absDir, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	parentDir := filepath.Dir(absDir)
	defaultProjectName := filepath.Base(parentDir)

	projectName, err := promptWithDefault(scanner, cmd.OutOrStdout(), "Project name", defaultProjectName)
	if err != nil {
		return err
	}

	repositories := make(map[string]workspace.Repository)

	// Attempt to detect git remote
	defaultRepoURL := ""
	defaultRepoName := defaultProjectName
	if remoteURL, err := git.DetectRemoteURL(parentDir); err == nil {
		defaultRepoURL = remoteURL
	} else {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Warning: current folder is not a git repository. You can still initialize tld, but automatic source code linking will require manual configuration of repository URL and localDir in .tld.yaml.")
	}

	for {
		repoKey, err := promptWithDefault(scanner, cmd.OutOrStdout(), "Repository key", defaultRepoName)
		if err != nil {
			return err
		}
		url, err := promptWithDefault(scanner, cmd.OutOrStdout(), "Repository URL", defaultRepoURL)
		if err != nil {
			return err
		}
		localDir, err := promptWithDefault(scanner, cmd.OutOrStdout(), "Repository local dir (empty for workspace parent)", "")
		if err != nil {
			return err
		}
		mode, err := promptRepositoryMode(scanner, cmd.OutOrStdout())
		if err != nil {
			return err
		}

		repositories[repoKey] = workspace.Repository{
			URL:      url,
			LocalDir: localDir,
			Config: &workspace.RepositoryConfig{
				Mode: mode,
			},
		}

		addAnother, err := promptYesNo(scanner, cmd.OutOrStdout(), "Add another repository? [y/N]", false)
		if err != nil {
			return err
		}
		if !addAnother {
			break
		}
	}

	config := workspace.WorkspaceConfig{
		ProjectName:  projectName,
		Exclude:      append([]string{}, defaultWorkspaceExclude...),
		Repositories: repositories,
	}
	data, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("marshal .tld.yaml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tld.yaml"), data, 0600); err != nil {
		return fmt.Errorf("write .tld.yaml: %w", err)
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Next steps:")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  1. tld login          - authenticate with tlDiagram.com")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  2. tld analyze .      - extract symbols from your repo")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  3. tld plan           - preview what will be created")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  4. tld apply          - push to tlDiagram.com")
	return nil
}

func promptWithDefault(scanner *bufio.Scanner, out interface{ Write([]byte) (int, error) }, label, defaultValue string) (string, error) {
	_, _ = fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return defaultValue, nil
	}
	value := strings.TrimSpace(scanner.Text())
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func promptRepositoryMode(scanner *bufio.Scanner, out interface{ Write([]byte) (int, error) }) (string, error) {
	_, _ = fmt.Fprintln(out, "Select repository mode:")
	_, _ = fmt.Fprintln(out, "  1. upsert - add new symbols, update existing - never delete (recommended)")
	_, _ = fmt.Fprintln(out, "  2. manual - no automatic changes - full manual control")
	_, _ = fmt.Fprintln(out, "  3. auto   - create, update, and delete based on source analysis")

	for {
		if _, err := fmt.Fprint(out, "Mode [1]: "); err != nil {
			return "", err
		}
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", err
			}
			return "", fmt.Errorf("repository mode is required")
		}
		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
		case "", "1", "upsert":
			return "upsert", nil
		case "2", "manual":
			return "manual", nil
		case "3", "auto":
			return "auto", nil
		}
	}
}

func promptYesNo(scanner *bufio.Scanner, out interface{ Write([]byte) (int, error) }, label string, defaultYes bool) (bool, error) {
	for {
		_, _ = fmt.Fprintf(out, "%s ", label)
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return false, err
			}
			return defaultYes, nil
		}
		switch strings.ToLower(strings.TrimSpace(scanner.Text())) {
		case "":
			return defaultYes, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		}
	}
}
