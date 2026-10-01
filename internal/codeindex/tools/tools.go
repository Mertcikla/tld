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
	path        func(config.Config) string
}

// All returns every indexer the pipeline knows how to invoke.
func All() []Tool {
	return []Tool{
		{Family: "go", Name: "scip-go", VersionArgs: []string{"--version"}, path: func(c config.Config) string { return c.Tools.SCIPGo }},
		{Family: "web", Name: "scip-typescript", VersionArgs: []string{"--version"}, path: func(c config.Config) string { return c.Tools.SCIPTypeScript }},
		{Family: "python", Name: "scip-python", VersionArgs: []string{"--version"}, path: func(c config.Config) string { return c.Tools.SCIPPython }},
		{Family: "dotnet", Name: "scip-dotnet", VersionArgs: []string{"--version"}, path: func(c config.Config) string { return c.Tools.SCIPDotnet }},
		{Family: "clang", Name: "scip-clang", VersionArgs: []string{"--version"}, path: func(c config.Config) string { return c.Tools.SCIPClang }},
		{Family: "jvm", Name: "scip-java", VersionArgs: []string{"--version"}, path: func(c config.Config) string { return c.Tools.SCIPJava }},
		{Family: "dart", Name: "scip-dart", VersionArgs: []string{"--version"}, path: func(c config.Config) string { return c.Tools.SCIPDart }},
		{Family: "php", Name: "scip-php", VersionArgs: []string{"--help"}, path: func(c config.Config) string { return c.Tools.SCIPPhp }},
		{Family: "ruby", Name: "scip-ruby", VersionArgs: []string{"--version"}, path: func(c config.Config) string { return c.Tools.SCIPRuby }},
		{Family: "rust", Name: "rust-analyzer", VersionArgs: []string{"--version"}, path: func(c config.Config) string { return c.Tools.RustAnalyzer }},
	}
}

// Status reports the resolution and version of one tool.
type Status struct {
	Family  string
	Name    string
	Path    string
	Found   bool
	Version string
	Error   string
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
	all := All()
	out := make([]Status, 0, len(all))
	for _, t := range all {
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
	s := Status{Family: t.Family, Name: t.Name}
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
