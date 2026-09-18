package initialize

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/mertcikla/tld/v2/internal/git"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var defaultWorkspaceExcludeProfiles = map[string][]string{
	"go": {
		".git/",
		"vendor/",
		"node_modules/",
		"**/*_test.go",
		"**/*.pb.go",
	},
	"python": {
		".git/",
		".venv/",
		"venv/",
		"__pycache__/",
		"**/*.pyc",
		".pytest_cache/",
		".mypy_cache/",
	},
	"rust": {
		".git/",
		"target/",
	},
	"cpp": {
		".git/",
		"build/",
		"cmake-build-*/",
		"CMakeFiles/",
		"out/",
		"bin/",
	},
	"default": {
		".git/",
		"node_modules/",
		".venv/",
		"build/",
		"dist/",
	},
}

var dominantLanguagePriority = []string{"go", "python", "rust", "cpp"}

var languageExtensions = map[string]map[string]bool{
	"go": {
		".go": true,
	},
	"python": {
		".py": true,
	},
	"rust": {
		".rs": true,
	},
	"cpp": {
		".c":   true,
		".cc":  true,
		".cpp": true,
		".cxx": true,
		".h":   true,
		".hh":  true,
		".hpp": true,
		".hxx": true,
	},
}

var initLanguageScanSkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	".venv":        true,
	"venv":         true,
	"target":       true,
	"build":        true,
	"dist":         true,
	"__pycache__":  true,
}

type workspaceInitDefaults struct {
	projectName string
	exclude     []string
	repoRoot    string
}

func detectWorkspaceInitDefaults(dir string) (*workspaceInitDefaults, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	scanRoot := workspaceContentRoot(absDir)
	repoRoot := scanRoot
	if detectedRepoRoot, err := git.RepoRoot(scanRoot); err == nil {
		repoRoot = detectedRepoRoot
	}
	projectName := filepath.Base(repoRoot)
	if projectName == "." || projectName == string(filepath.Separator) || projectName == "" {
		projectName = filepath.Base(scanRoot)
	}
	language := detectDominantLanguage(repoRoot)
	return &workspaceInitDefaults{
		projectName: projectName,
		exclude:     defaultWorkspaceExclude(language),
		repoRoot:    repoRoot,
	}, nil
}

func workspaceContentRoot(absDir string) string {
	base := filepath.Base(absDir)
	if base == ".tld" || base == "tld" {
		return filepath.Dir(absDir)
	}
	return absDir
}

func defaultWorkspaceExclude(language string) []string {
	profile := defaultWorkspaceExcludeProfiles[language]
	if len(profile) == 0 {
		profile = defaultWorkspaceExcludeProfiles["default"]
	}
	return append([]string{}, profile...)
}

func detectDominantLanguage(root string) string {
	counts := map[string]int{}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if initLanguageScanSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if ext == "" {
			return nil
		}
		for language, extensions := range languageExtensions {
			if extensions[ext] {
				counts[language]++
				break
			}
		}
		return nil
	})

	dominant := ""
	maxCount := 0
	for _, language := range dominantLanguagePriority {
		if count := counts[language]; count > maxCount {
			dominant = language
			maxCount = count
		}
	}
	return dominant
}

func generateDefaultWorkspaceConfig(dir string) ([]byte, error) {
	defaults, err := detectWorkspaceInitDefaults(dir)
	if err != nil {
		return nil, err
	}

	config := workspace.WorkspaceConfig{
		ProjectName: defaults.projectName,
		Exclude:     defaults.exclude,
	}

	// Attempt to detect git remote
	if remoteURL, err := git.DetectRemoteURL(defaults.repoRoot); err == nil {
		config.Repositories = map[string]workspace.Repository{
			defaults.projectName: {
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
				term.Successf(cmd.OutOrStdout(), "Workspace initialized at %s", term.Path(cmd.OutOrStdout(), dir))
				term.Infof(cmd.OutOrStdout(), "Global config already exists at %s", term.Path(cmd.OutOrStdout(), cfgPath))
			} else {
				if err := workspace.EnsureGlobalConfig(); err != nil {
					return fmt.Errorf("ensure global config: %w", err)
				}
				term.Successf(cmd.OutOrStdout(), "Workspace initialized at %s", term.Path(cmd.OutOrStdout(), dir))
				term.Infof(cmd.OutOrStdout(), "Global config created at %s", term.Path(cmd.OutOrStdout(), cfgPath))
			}

			return nil
		},
	}
	cmd.Flags().BoolVar(&wizard, "wizard", false, "run interactive setup wizard")
	return cmd
}

func runInitWizard(cmd *cobra.Command, dir string) error {
	scanner := bufio.NewScanner(cmd.InOrStdin())

	defaults, err := detectWorkspaceInitDefaults(dir)
	if err != nil {
		return err
	}

	projectName, err := promptWithDefault(scanner, cmd.OutOrStdout(), "Project name", defaults.projectName)
	if err != nil {
		return err
	}

	repositories := make(map[string]workspace.Repository)

	// Attempt to detect git remote
	defaultRepoURL := ""
	defaultRepoName := defaults.projectName
	if remoteURL, err := git.DetectRemoteURL(defaults.repoRoot); err == nil {
		defaultRepoURL = remoteURL
	} else {
		term.Warn(cmd.OutOrStdout(), "Current folder is not a git repository. Automatic source linking requires manual configuration of repository URL and localDir in .tld.yaml.")
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
		Exclude:      defaults.exclude,
		Repositories: repositories,
	}
	data, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("marshal .tld.yaml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".tld.yaml"), data, 0600); err != nil {
		return fmt.Errorf("write .tld.yaml: %w", err)
	}

	term.Separator(cmd.OutOrStdout())
	term.Info(cmd.OutOrStdout(), "Next steps:")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "    1. tld analyze .      - extract symbols from your repo")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "    2. tld add <name>     - add elements (applies instantly)")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "    3. tld connect        - connect elements (applies instantly)")
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
