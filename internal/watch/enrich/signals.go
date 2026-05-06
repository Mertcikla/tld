package enrich

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mertcikla/tld/internal/analyzer"
)

var goRequireLineRE = regexp.MustCompile(`^\s*([A-Za-z0-9_./~-]+)\s+v[0-9]`)

func DiscoverRepositorySignals(repoRoot string) []ActivationSignal {
	var signals []ActivationSignal
	signals = append(signals, discoverGoModSignals(filepath.Join(repoRoot, "go.mod"))...)
	signals = append(signals, discoverPackageJSONSignals(repoRoot)...)
	return uniqueSignals(signals)
}

func DiscoverRepositorySignalsFromFiles(repoRoot string, files []string) []ActivationSignal {
	var signals []ActivationSignal
	for _, file := range files {
		rel, err := filepath.Rel(repoRoot, file)
		if err != nil {
			rel = file
		}
		rel = filepath.ToSlash(rel)
		switch filepath.Base(file) {
		case "go.mod":
			signals = append(signals, discoverGoModSignals(file)...)
		case "package.json":
			signals = append(signals, packageJSONSignals(file, rel)...)
		}
	}
	return uniqueSignals(signals)
}

func ImportSignals(refs []analyzer.Ref) []ActivationSignal {
	signals := make([]ActivationSignal, 0, len(refs))
	for _, ref := range refs {
		if ref.Kind != "import" || strings.TrimSpace(ref.TargetPath) == "" {
			continue
		}
		signals = append(signals, ActivationSignal{Kind: SignalImport, Value: ref.TargetPath, Source: ref.FilePath})
	}
	return uniqueSignals(signals)
}

func discoverGoModSignals(path string) []ActivationSignal {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var signals []ActivationSignal
	for _, line := range strings.Split(string(data), "\n") {
		match := goRequireLineRE.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		signals = append(signals, ActivationSignal{Kind: SignalDependency, Value: match[1], Source: "go.mod"})
	}
	return signals
}

func discoverPackageJSONSignals(repoRoot string) []ActivationSignal {
	var signals []ActivationSignal
	_ = filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if isSignalScanIgnoredDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "package.json" {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			rel = path
		}
		signals = append(signals, packageJSONSignals(path, filepath.ToSlash(rel))...)
		return nil
	})
	return signals
}

func isSignalScanIgnoredDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", ".hg", ".svn", "node_modules", "dist", "build", ".next", ".turbo", "coverage", "vendor":
		return true
	default:
		return strings.HasPrefix(name, ".")
	}
}

func packageJSONSignals(path, rel string) []ActivationSignal {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pkg struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil
	}
	var signals []ActivationSignal
	add := func(values map[string]string) {
		for name := range values {
			signals = append(signals, ActivationSignal{Kind: SignalDependency, Value: name, Source: rel})
		}
	}
	add(pkg.Dependencies)
	add(pkg.DevDependencies)
	add(pkg.PeerDependencies)
	add(pkg.OptionalDependencies)
	return signals
}

func uniqueSignals(signals []ActivationSignal) []ActivationSignal {
	seen := map[string]struct{}{}
	out := make([]ActivationSignal, 0, len(signals))
	for _, signal := range signals {
		signal.Kind = strings.TrimSpace(signal.Kind)
		signal.Value = strings.TrimSpace(signal.Value)
		if signal.Kind == "" || signal.Value == "" {
			continue
		}
		key := signal.Kind + "\x00" + signal.Value + "\x00" + signal.Source
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, signal)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			if out[i].Value == out[j].Value {
				return out[i].Source < out[j].Source
			}
			return out[i].Value < out[j].Value
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}
