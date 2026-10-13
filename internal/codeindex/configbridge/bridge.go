// Package configbridge maps tld's global configuration onto the codeindex
// engine config. The index.* section is authoritative.
package configbridge

import (
	"strings"

	ci "github.com/mertcikla/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// FromGlobal builds a codeindex config from tld's global config. CODEINDEX_*
// environment variables take precedence over the global index.* settings.
func FromGlobal(global *workspace.Config) ci.Config {
	cfg := ci.Default()
	if global != nil {
		tools := global.Index.Tools
		cfg.Tools.SCIPGo = firstNonEmpty(tools.SCIPGo, cfg.Tools.SCIPGo)
		cfg.Tools.SCIPTypeScript = firstNonEmpty(tools.SCIPTypeScript, cfg.Tools.SCIPTypeScript)
		cfg.Tools.SCIPPython = firstNonEmpty(tools.SCIPPython, cfg.Tools.SCIPPython)
		cfg.Tools.SCIPDotnet = firstNonEmpty(tools.SCIPDotnet, cfg.Tools.SCIPDotnet)
		cfg.Tools.SCIPClang = firstNonEmpty(tools.SCIPClang, cfg.Tools.SCIPClang)
		cfg.Tools.SCIPJava = firstNonEmpty(tools.SCIPJava, cfg.Tools.SCIPJava)
		cfg.Tools.SCIPDart = firstNonEmpty(tools.SCIPDart, cfg.Tools.SCIPDart)
		cfg.Tools.SCIPPhp = firstNonEmpty(tools.SCIPPhp, cfg.Tools.SCIPPhp)
		cfg.Tools.SCIPRuby = firstNonEmpty(tools.SCIPRuby, cfg.Tools.SCIPRuby)
		cfg.Tools.RustAnalyzer = firstNonEmpty(tools.RustAnalyzer, cfg.Tools.RustAnalyzer)
		if tools.TimeoutSeconds > 0 {
			cfg.Tools.TimeoutSeconds = tools.TimeoutSeconds
		}
	}
	cfg.ApplyEnv()
	return cfg
}

func firstNonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}
