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
	signals = append(signals, discoverComposeSignals(repoRoot)...)
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
		case "requirements.txt":
			signals = append(signals, lineDependencySignals(file, rel, requirementSignalName)...)
		case "pyproject.toml", "poetry.lock":
			signals = append(signals, lineDependencySignals(file, rel, tomlSignalName)...)
		case "Cargo.toml":
			signals = append(signals, lineDependencySignals(file, rel, cargoSignalName)...)
		case "pom.xml":
			signals = append(signals, pomSignals(file, rel)...)
		case "build.gradle", "build.gradle.kts":
			signals = append(signals, lineDependencySignals(file, rel, gradleSignalName)...)
		case "CMakeLists.txt", "conanfile.txt", "conanfile.py", "vcpkg.json":
			signals = append(signals, lineDependencySignals(file, rel, cppSignalName)...)
		}
		if isComposeFile(rel) {
			signals = append(signals, ActivationSignal{Kind: SignalDependency, Value: "docker-compose", Source: rel})
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
	for line := range strings.SplitSeq(string(data), "\n") {
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

func discoverComposeSignals(repoRoot string) []ActivationSignal {
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
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			rel = path
		}
		if isComposeFile(filepath.ToSlash(rel)) {
			signals = append(signals, ActivationSignal{Kind: SignalDependency, Value: "docker-compose", Source: rel})
		}
		return nil
	})
	return signals
}

func isComposeFile(rel string) bool {
	base := strings.ToLower(filepath.Base(rel))
	switch base {
	case "docker-compose.yml", "docker-compose.yaml", "compose.yaml", "compose.yml":
		return true
	default:
		return strings.HasPrefix(base, "docker-compose.") && (strings.HasSuffix(base, ".yml") || strings.HasSuffix(base, ".yaml"))
	}
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

func lineDependencySignals(path, rel string, parse func(string) string) []ActivationSignal {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var signals []ActivationSignal
	for line := range strings.SplitSeq(string(data), "\n") {
		name := parse(line)
		if name == "" {
			continue
		}
		signals = append(signals, ActivationSignal{Kind: SignalDependency, Value: name, Source: rel})
	}
	return signals
}

func pomSignals(path, rel string) []ActivationSignal {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	re := regexp.MustCompile(`(?s)<dependency>.*?<groupId>\s*([^<\s]+)\s*</groupId>.*?<artifactId>\s*([^<\s]+)\s*</artifactId>.*?</dependency>`)
	var signals []ActivationSignal
	for _, match := range re.FindAllStringSubmatch(string(data), -1) {
		if len(match) == 3 {
			signals = append(signals, ActivationSignal{Kind: SignalDependency, Value: match[1] + ":" + match[2], Source: rel})
			signals = append(signals, ActivationSignal{Kind: SignalDependency, Value: match[2], Source: rel})
		}
	}
	return signals
}

func requirementSignalName(line string) string {
	line = strings.TrimSpace(strings.Split(line, "#")[0])
	if line == "" || strings.HasPrefix(line, "-") {
		return ""
	}
	return signalPrefix(line, "=", "<", ">", "~", "!", "[", ";")
}

func tomlSignalName(line string) string {
	line = strings.TrimSpace(strings.Split(line, "#")[0])
	if line == "" || strings.HasPrefix(line, "[") {
		return ""
	}
	if strings.HasPrefix(line, "\"") || strings.HasPrefix(line, "'") {
		trimmed := strings.Trim(line, " ,")
		trimmed = strings.Trim(trimmed, `"'`)
		if trimmed != "" && !strings.Contains(trimmed, "=") {
			return requirementSignalName(trimmed)
		}
	}
	if idx := strings.Index(line, "="); idx > 0 {
		return strings.Trim(strings.TrimSpace(line[:idx]), `"'`)
	}
	return ""
}

func cargoSignalName(line string) string {
	name := tomlSignalName(line)
	switch name {
	case "package", "dependencies", "dev-dependencies", "build-dependencies", "workspace":
		return ""
	default:
		return name
	}
}

func gradleSignalName(line string) string {
	line = strings.TrimSpace(line)
	for _, quote := range []string{"\"", "'"} {
		start := strings.Index(line, quote)
		if start < 0 {
			continue
		}
		rest := line[start+1:]
		before, _, ok := strings.Cut(rest, quote)
		if !ok {
			continue
		}
		value := before
		if strings.Count(value, ":") >= 1 {
			parts := strings.Split(value, ":")
			return parts[len(parts)-2]
		}
	}
	return ""
}

func cppSignalName(line string) string {
	line = strings.TrimSpace(strings.Split(line, "#")[0])
	if line == "" {
		return ""
	}
	for _, prefix := range []string{"find_package(", "target_link_libraries(", "requires =", "self.requires(", "\"name\":"} {
		if _, after, ok := strings.Cut(line, prefix); ok {
			value := strings.TrimSpace(after)
			value = strings.Trim(value, ` "'),[]`)
			return signalPrefix(value, " ", "/", ")", ",", "\"")
		}
	}
	return ""
}

func signalPrefix(value string, stops ...string) string {
	value = strings.TrimSpace(value)
	end := len(value)
	for _, stop := range stops {
		if idx := strings.Index(value, stop); idx >= 0 && idx < end {
			end = idx
		}
	}
	return strings.TrimSpace(value[:end])
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
