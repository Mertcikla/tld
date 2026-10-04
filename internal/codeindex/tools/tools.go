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
	// InstallHint is a copy-pasteable command that installs the tool, or a
	// release page when no package manager install exists.
	InstallHint string
	path        func(config.Config) string
}

// All returns every indexer the pipeline knows how to invoke.
func All() []Tool {
	return []Tool{
		{Family: "go", Name: "scip-go", VersionArgs: []string{"--version"}, InstallHint: "go install github.com/scip-code/scip-go/cmd/scip-go@latest", path: func(c config.Config) string { return c.Tools.SCIPGo }},
		{Family: "web", Name: "scip-typescript", VersionArgs: []string{"--version"}, InstallHint: "npm install -g @sourcegraph/scip-typescript", path: func(c config.Config) string { return c.Tools.SCIPTypeScript }},
		{Family: "python", Name: "scip-python", VersionArgs: []string{"--version"}, InstallHint: "npm install -g @sourcegraph/scip-python", path: func(c config.Config) string { return c.Tools.SCIPPython }},
		{Family: "dotnet", Name: "scip-dotnet", VersionArgs: []string{"--version"}, InstallHint: "dotnet tool install --global scip-dotnet", path: func(c config.Config) string { return c.Tools.SCIPDotnet }},
		{Family: "clang", Name: "scip-clang", VersionArgs: []string{"--version"}, InstallHint: "download a release from https://github.com/sourcegraph/scip-clang/releases", path: func(c config.Config) string { return c.Tools.SCIPClang }},
		{Family: "jvm", Name: "scip-java", VersionArgs: []string{"--version"}, InstallHint: "cs install scip-java", path: func(c config.Config) string { return c.Tools.SCIPJava }},
		{Family: "dart", Name: "scip-dart", VersionArgs: []string{"--version"}, InstallHint: "dart pub global activate scip_dart", path: func(c config.Config) string { return c.Tools.SCIPDart }},
		{Family: "php", Name: "scip-php", VersionArgs: []string{"--help"}, InstallHint: "composer global require davidrjenni/scip-php", path: func(c config.Config) string { return c.Tools.SCIPPhp }},
		{Family: "ruby", Name: "scip-ruby", VersionArgs: []string{"--version"}, InstallHint: "gem install scip-ruby", path: func(c config.Config) string { return c.Tools.SCIPRuby }},
		{Family: "rust", Name: "rust-analyzer", VersionArgs: []string{"--version"}, InstallHint: "rustup component add rust-analyzer", path: func(c config.Config) string { return c.Tools.RustAnalyzer }},
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
	Family      string
	Name        string
	Path        string
	Found       bool
	Version     string
	Error       string
	InstallHint string
}

// Resolve returns the executable path for a tool. Paths containing a separator
// are checked directly; bare names are resolved on PATH.
func Resolve(t Tool, cfg config.Config) (string, error) {
	name := t.path(cfg)
	if name == "" {
		name = t.Name
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
	return exec.LookPath(name)
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
	s := Status{Family: t.Family, Name: t.Name, InstallHint: t.InstallHint}
	path, err := Resolve(t, cfg)
	if err != nil {
		s.Path = path
		s.Error = err.Error()
		return s
	}
	s.Path = path
	s.Found = true
	s.Version = probeVersion(ctx, path, t.VersionArgs)
	return s
}

func probeVersion(ctx context.Context, path string, args []string) string {
	probeCtx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, path, args...)
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if text == "" && err != nil {
		return ""
	}
	if line, _, found := strings.Cut(text, "\n"); found {
		return strings.TrimSpace(line)
	}
	return text
}
