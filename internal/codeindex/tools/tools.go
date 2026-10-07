// Package tools resolves and probes the external SCIP indexers the codeindex
// pipeline shells out to. Every family is invoked as a separate binary on PATH;
// a missing tool fails the project that needs it.
package tools

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mertcikla/tld/v2/internal/codeindex/config"
)

// versionTimeout bounds the version/help probe so doctor cannot hang.
const versionTimeout = 5 * time.Second

// Tool describes one external indexer binary.
type Tool struct {
	Family      string
	Name        string
	VersionArgs []string
	// MinVersion is the lowest version the pipeline is tested against. It is
	// advisory: older tools still run, but may emit less symbol metadata.
	MinVersion string
	// InstallHint is a copy-pasteable command that installs the tool, or a
	// release page when no package manager install exists.
	InstallHint string
	// DownloadURL is the release or package page a user can download the tool
	// from. It backs the UI's download action for missing or outdated tools.
	DownloadURL string
	path        func(config.Config) string
}

// All returns every indexer the pipeline knows how to invoke.
func All() []Tool {
	return []Tool{
		{Family: "go", Name: "scip-go", VersionArgs: []string{"--version"}, MinVersion: "0.1.26", InstallHint: "go install github.com/scip-code/scip-go/cmd/scip-go@latest", DownloadURL: "https://github.com/scip-code/scip-go/releases", path: func(c config.Config) string { return c.Tools.SCIPGo }},
		{Family: "web", Name: "scip-typescript", VersionArgs: []string{"--version"}, MinVersion: "0.4.0", InstallHint: "npm install -g @sourcegraph/scip-typescript", DownloadURL: "https://github.com/sourcegraph/scip-typescript/releases", path: func(c config.Config) string { return c.Tools.SCIPTypeScript }},
		{Family: "python", Name: "scip-python", VersionArgs: []string{"--version"}, MinVersion: "0.6.6", InstallHint: "npm install -g @sourcegraph/scip-python", DownloadURL: "https://github.com/sourcegraph/scip-python/releases", path: func(c config.Config) string { return c.Tools.SCIPPython }},
		{Family: "dotnet", Name: "scip-dotnet", VersionArgs: []string{"--version"}, MinVersion: "0.2.14", InstallHint: "dotnet tool install --global scip-dotnet", DownloadURL: "https://github.com/sourcegraph/scip-dotnet/releases", path: func(c config.Config) string { return c.Tools.SCIPDotnet }},
		{Family: "clang", Name: "scip-clang", VersionArgs: []string{"--version"}, MinVersion: "0.4.0", InstallHint: "download a release from https://github.com/sourcegraph/scip-clang/releases", DownloadURL: "https://github.com/sourcegraph/scip-clang/releases", path: func(c config.Config) string { return c.Tools.SCIPClang }},
		{Family: "jvm", Name: "scip-java", VersionArgs: []string{"--version"}, InstallHint: "cs install scip-java", DownloadURL: "https://github.com/sourcegraph/scip-java/releases", path: func(c config.Config) string { return c.Tools.SCIPJava }},
		{Family: "dart", Name: "scip-dart", VersionArgs: []string{"--version"}, MinVersion: "1.6.2", InstallHint: "dart pub global activate scip_dart", DownloadURL: "https://pub.dev/packages/scip_dart", path: func(c config.Config) string { return c.Tools.SCIPDart }},
		{Family: "php", Name: "scip-php", VersionArgs: []string{"--help"}, InstallHint: "composer global require davidrjenni/scip-php", DownloadURL: "https://github.com/davidrjenni/scip-php", path: func(c config.Config) string { return c.Tools.SCIPPhp }},
		{Family: "ruby", Name: "scip-ruby", VersionArgs: []string{"--version"}, MinVersion: "0.5.0", InstallHint: "gem install scip-ruby", DownloadURL: "https://github.com/sourcegraph/scip-ruby/releases", path: func(c config.Config) string { return c.Tools.SCIPRuby }},
		{Family: "rust", Name: "rust-analyzer", VersionArgs: []string{"--version"}, InstallHint: "rustup component add rust-analyzer", DownloadURL: "https://rust-analyzer.github.io/", path: func(c config.Config) string { return c.Tools.RustAnalyzer }},
	}
}

// ForFamilies returns the tools for the given indexer families in the stable
// order of All. Unknown families are skipped.
func ForFamilies(families []string) []Tool {
	if len(families) == 0 {
		return nil
	}
	wanted := make(map[string]bool, len(families))
	for _, family := range families {
		wanted[family] = true
	}
	out := make([]Tool, 0, len(families))
	for _, tool := range All() {
		if wanted[tool.Family] {
			out = append(out, tool)
		}
	}
	return out
}

// Status reports the resolution and version of one tool.
type Status struct {
	Family  string
	Name    string
	Path    string
	Found   bool
	Version string
	// BelowMinimum is set when the detected version is older than the tested
	// minimum. It is advisory: the tool still runs.
	BelowMinimum bool
	Minimum      string
	Error        string
	InstallHint  string
	DownloadURL  string
}

// Resolve returns the executable path for a tool. Paths containing a separator
// are checked directly; bare names are resolved on PATH.
func Resolve(t Tool, cfg config.Config) (string, error) {
	name := t.path(cfg)
	if name == "" {
		name = t.Name
	}
	return ResolveName(name)
}

// ResolveName resolves an indexer binary by configured path or name. Paths
// containing a separator or absolute paths are checked directly. Bare names are
// searched on PATH, then under well-known tool install directories, then via the
// hyphen/underscore alias some installers produce (for example the pub tool
// installs scip_dart while the tool is named scip-dart).
func ResolveName(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return name, exec.ErrNotFound
	}
	if strings.ContainsRune(name, filepath.Separator) || filepath.IsAbs(name) {
		info, err := os.Stat(name)
		if err != nil {
			return name, err
		}
		if info.IsDir() {
			return name, errors.New("is a directory")
		}
		return name, nil
	}
	path, lookErr := exec.LookPath(name)
	if lookErr == nil {
		return path, nil
	}
	for _, alias := range nameAliases(name) {
		if aliasPath, err := exec.LookPath(alias); err == nil {
			return aliasPath, nil
		}
	}
	for _, dir := range indexerSearchDirs() {
		for _, candidate := range append([]string{name}, nameAliases(name)...) {
			full := filepath.Join(dir, candidate)
			if info, err := os.Stat(full); err == nil && !info.IsDir() {
				return full, nil
			}
		}
	}
	return name, lookErr
}

// nameAliases returns the alternate hyphen/underscore spelling of a bare tool
// name, since some package managers install one and the pipeline defaults to the
// other.
func nameAliases(name string) []string {
	if strings.Contains(name, "-") {
		return []string{strings.ReplaceAll(name, "-", "_")}
	}
	if strings.Contains(name, "_") {
		return []string{strings.ReplaceAll(name, "_", "-")}
	}
	return nil
}

// indexerSearchDirs lists the well-known install locations for the SCIP indexers
// and language servers, which are frequently not on a GUI or non-login PATH.
func indexerSearchDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, "go", "bin"),
		filepath.Join(home, ".dotnet", "tools"),
		filepath.Join(home, ".pub-cache", "bin"),
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".cargo", "bin"),
		filepath.Join(home, ".local", "scip-php", "vendor", "bin"),
	}
}

// Check probes every indexer with its version/help flag and returns the results
// in stable family order. A probe that cannot run still yields a Status.
func Check(ctx context.Context, cfg config.Config) []Status {
	return CheckTools(ctx, cfg, All())
}

// CheckTools probes the selected tools, preserving their order. A probe that
// cannot run still yields a Status.
func CheckTools(ctx context.Context, cfg config.Config, selected []Tool) []Status {
	out := make([]Status, 0, len(selected))
	for _, t := range selected {
		out = append(out, checkOne(ctx, t, cfg))
	}
	return out
}

// Missing returns the statuses for indexers that cannot be resolved on PATH.
func Missing(ctx context.Context, cfg config.Config) []Status {
	var out []Status
	for _, s := range Check(ctx, cfg) {
		if !s.Found {
			out = append(out, s)
		}
	}
	return out
}

func checkOne(ctx context.Context, t Tool, cfg config.Config) Status {
	s := Status{Family: t.Family, Name: t.Name, InstallHint: t.InstallHint, DownloadURL: t.DownloadURL, Minimum: t.MinVersion}
	path, err := Resolve(t, cfg)
	if err != nil {
		s.Path = path
		s.Error = err.Error()
		return s
	}
	s.Path = path
	s.Found = true
	s.Version = probeVersion(ctx, path, t.VersionArgs)
	if t.MinVersion != "" {
		if ok, comparable := versionAtLeast(s.Version, t.MinVersion); comparable && !ok {
			s.BelowMinimum = true
		}
	}
	return s
}

// versionAtLeast reports whether version >= minimum using the first numeric
// dotted tuple found in each string. comparable is false when either string has
// no parseable version, in which case the caller should not warn.
func versionAtLeast(version, minimum string) (atLeast, comparable bool) {
	v, vok := versionTuple(version)
	m, mok := versionTuple(minimum)
	if !vok || !mok {
		return false, false
	}
	for i := 0; i < len(m); i++ {
		vi := 0
		if i < len(v) {
			vi = v[i]
		}
		if vi != m[i] {
			return vi > m[i], true
		}
	}
	return true, true
}

// versionTuple extracts the first dotted numeric tuple (for example "0.1.26"
// from "scip-go 0.1.26").
func versionTuple(s string) ([]int, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			continue
		}
		j := i
		for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == '.') {
			j++
		}
		field := strings.Trim(s[i:j], ".")
		if field != "" {
			parts := strings.Split(field, ".")
			nums := make([]int, 0, len(parts))
			valid := true
			for _, part := range parts {
				n, err := strconv.Atoi(part)
				if err != nil {
					valid = false
					break
				}
				nums = append(nums, n)
			}
			if valid && len(nums) > 0 {
				return nums, true
			}
		}
		i = j
	}
	return nil, false
}

func probeVersion(ctx context.Context, path string, args []string) string {
	probeCtx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	out, err := Run(probeCtx, path, args, "")
	text := strings.TrimSpace(string(out))
	if probeCtx.Err() != nil || strings.HasPrefix(text, truncatedOutput) || (text == "" && err != nil) {
		return ""
	}
	if line, _, found := strings.Cut(text, "\n"); found {
		return strings.TrimSpace(line)
	}
	return text
}
