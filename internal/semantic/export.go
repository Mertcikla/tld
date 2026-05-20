package semantic

import (
	"bufio"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mertcikla/tld/v2/internal/semantic/semanticpb"
	"github.com/mertcikla/tld/v2/internal/watch"
	"google.golang.org/protobuf/encoding/protojson"
)

type ExportResult struct {
	RecordsWritten int `json:"records_written"`
}

type symbolRaw struct {
	Description  string `json:"description"`
	RawSignature string `json:"raw_signature"`
}

type factAttrs map[string]string

func ExportJSONL(ctx context.Context, store *watch.Store, repositoryID int64, outputPath string) (ExportResult, error) {
	if store == nil {
		return ExportResult{}, fmt.Errorf("semantic export requires a watch store")
	}
	if strings.TrimSpace(outputPath) == "" {
		return ExportResult{}, fmt.Errorf("semantic output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return ExportResult{}, fmt.Errorf("create semantic output directory: %w", err)
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return ExportResult{}, fmt.Errorf("create semantic output: %w", err)
	}
	defer func() { _ = file.Close() }()

	repo, err := store.Repository(ctx, repositoryID)
	if err != nil {
		return ExportResult{}, err
	}
	symbols, err := store.SymbolsForRepository(ctx, repositoryID)
	if err != nil {
		return ExportResult{}, err
	}
	facts, err := store.FactsForRepository(ctx, repositoryID)
	if err != nil {
		return ExportResult{}, err
	}
	refs, err := store.QueryReferences(ctx, repositoryID, watch.ReferenceQuery{Limit: -1})
	if err != nil {
		return ExportResult{}, err
	}
	identityKeys, err := store.SymbolIdentityKeys(ctx, repositoryID)
	if err != nil {
		return ExportResult{}, err
	}

	builder := newBuilder(repo, symbols, facts, refs, identityKeys)
	marshal := protojson.MarshalOptions{UseProtoNames: false}
	writer := bufio.NewWriter(file)
	count := 0
	for _, record := range builder.records() {
		data, err := marshal.Marshal(record)
		if err != nil {
			return ExportResult{}, fmt.Errorf("marshal semantic record %q: %w", record.GetUrn(), err)
		}
		if _, err := writer.Write(data); err != nil {
			return ExportResult{}, err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return ExportResult{}, err
		}
		count++
	}
	if err := writer.Flush(); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{RecordsWritten: count}, nil
}

type builder struct {
	repo         watch.Repository
	repoHash     string
	symbols      []watch.Symbol
	facts        []watch.Fact
	refs         []watch.Reference
	identityKeys map[string]string
	symbolByID   map[int64]watch.Symbol
	urnByStable  map[string]string
}

func newBuilder(repo watch.Repository, symbols []watch.Symbol, facts []watch.Fact, refs []watch.Reference, identityKeys map[string]string) *builder {
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].FilePath == symbols[j].FilePath {
			if symbols[i].StartLine == symbols[j].StartLine {
				return symbols[i].StableKey < symbols[j].StableKey
			}
			return symbols[i].StartLine < symbols[j].StartLine
		}
		return symbols[i].FilePath < symbols[j].FilePath
	})
	b := &builder{
		repo:         repo,
		repoHash:     shortHash(repo.RepoRoot),
		symbols:      symbols,
		facts:        facts,
		refs:         refs,
		identityKeys: identityKeys,
		symbolByID:   map[int64]watch.Symbol{},
		urnByStable:  map[string]string{},
	}
	for _, sym := range symbols {
		b.symbolByID[sym.ID] = sym
		b.urnByStable[sym.StableKey] = b.symbolURN(sym)
	}
	return b
}

func (b *builder) records() []*semanticpb.SymbolContext {
	out := make([]*semanticpb.SymbolContext, 0, len(b.symbols))
	for _, sym := range b.symbols {
		out = append(out, b.symbolRecord(sym))
	}
	out = append(out, b.syntheticFactRecords()...)
	return out
}

func (b *builder) symbolRecord(sym watch.Symbol) *semanticpb.SymbolContext {
	raw := decodeSymbolRaw(sym.RawJSON)
	facts := b.factsForSymbol(sym)
	properties := []*semanticpb.Property{
		prop("stable_key", sym.StableKey, "identity"),
		prop("file_path", sym.FilePath, "source"),
		prop("start_line", strconv.Itoa(sym.StartLine), "source"),
		prop("kind", sym.Kind, "symbol"),
	}
	if sym.EndLine != nil {
		properties = append(properties, prop("end_line", strconv.Itoa(*sym.EndLine), "source"))
	}
	if branch := nullStringValue(b.repo.Branch); branch != "" {
		properties = append(properties, prop("branch", branch, "repository"))
	}
	if head := nullStringValue(b.repo.HeadCommit); head != "" {
		properties = append(properties, prop("head_commit", head, "repository"))
	}
	for _, fact := range facts {
		for key, value := range parseAttrs(fact.AttributesJSON) {
			properties = append(properties, prop("fact."+fact.Type+"."+key, value, "fact"))
		}
	}
	return &semanticpb.SymbolContext{
		Urn:                 b.symbolURN(sym),
		SymbolName:          sym.QualifiedName,
		EntityType:          entityTypeForSymbol(sym, facts),
		SourceLanguage:      languageFromStableKey(sym.StableKey),
		DomainBoundary:      domainBoundary(sym, facts),
		RawSignature:        raw.RawSignature,
		NaturalLanguageDocs: cleanDoc(raw.Description),
		Properties:          dedupeProperties(properties),
		Relationships:       b.relationships(sym),
		SystemContext:       systemContextFromFacts(facts),
	}
}

func (b *builder) symbolURN(sym watch.Symbol) string {
	key := strings.TrimSpace(b.identityKeys[sym.StableKey])
	if key == "" {
		key = sym.StableKey
	}
	return "urn:tld:symbol:" + b.repoHash + ":" + sanitizeURNPart(key)
}

func (b *builder) relationships(sym watch.Symbol) []*semanticpb.Relationship {
	var out []*semanticpb.Relationship
	for _, ref := range b.refs {
		if ref.SourceSymbolID == sym.ID {
			target, ok := b.symbolByID[ref.TargetSymbolID]
			if !ok {
				continue
			}
			out = append(out, &semanticpb.Relationship{
				TargetUrn:        b.urnByStable[target.StableKey],
				RelationshipType: "depends_on",
				Description:      "Depends On: " + target.QualifiedName,
			})
		}
		if ref.TargetSymbolID == sym.ID {
			source, ok := b.symbolByID[ref.SourceSymbolID]
			if !ok {
				continue
			}
			out = append(out, &semanticpb.Relationship{
				TargetUrn:        b.urnByStable[source.StableKey],
				RelationshipType: "consumed_by",
				Description:      "Consumed By: " + source.QualifiedName,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].RelationshipType == out[j].RelationshipType {
			return out[i].TargetUrn < out[j].TargetUrn
		}
		return out[i].RelationshipType < out[j].RelationshipType
	})
	return dedupeRelationships(out)
}

func (b *builder) factsForSymbol(sym watch.Symbol) []watch.Fact {
	var out []watch.Fact
	for _, fact := range b.facts {
		if fact.SubjectKind == "symbol" && fact.SubjectStableKey == sym.StableKey {
			out = append(out, fact)
			continue
		}
		if fact.FilePath != sym.FilePath {
			continue
		}
		if fact.SubjectKind == "file" && fact.SubjectStableKey == "file:"+sym.FilePath {
			out = append(out, fact)
			continue
		}
		if fact.StartLine >= sym.StartLine && (sym.EndLine == nil || fact.StartLine <= *sym.EndLine) {
			out = append(out, fact)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			return out[i].StableKey < out[j].StableKey
		}
		return out[i].Type < out[j].Type
	})
	return out
}

func (b *builder) syntheticFactRecords() []*semanticpb.SymbolContext {
	seen := map[string]struct{}{}
	var out []*semanticpb.SymbolContext
	for _, fact := range b.facts {
		entityType := entityTypeForFact(fact)
		if entityType == semanticpb.EntityType_ENTITY_TYPE_UNSPECIFIED {
			continue
		}
		key := firstNonEmpty(fact.ObjectStableKey, fact.StableKey)
		if key == "" {
			continue
		}
		urn := "urn:tld:fact:" + b.repoHash + ":" + sanitizeURNPart(key)
		if _, ok := seen[urn]; ok {
			continue
		}
		seen[urn] = struct{}{}
		name := firstNonEmpty(fact.ObjectName, fact.Name, key)
		out = append(out, &semanticpb.SymbolContext{
			Urn:                 urn,
			SymbolName:          name,
			EntityType:          entityType,
			SourceLanguage:      languageFromFilePath(fact.FilePath),
			DomainBoundary:      normalizeBoundary(pathBoundary(fact.FilePath)),
			NaturalLanguageDocs: narrativeFactDoc(fact),
			Properties: []*semanticpb.Property{
				prop("stable_key", key, "identity"),
				prop("file_path", fact.FilePath, "source"),
				prop("fact_type", fact.Type, "fact"),
				prop("enricher", fact.Enricher, "fact"),
			},
			SystemContext: systemContextFromFacts([]watch.Fact{fact}),
		})
	}
	return out
}

func decodeSymbolRaw(rawJSON string) symbolRaw {
	var raw symbolRaw
	_ = json.Unmarshal([]byte(rawJSON), &raw)
	raw.Description = cleanDoc(raw.Description)
	raw.RawSignature = stripSignature(raw.RawSignature)
	return raw
}

func entityTypeForSymbol(sym watch.Symbol, facts []watch.Fact) semanticpb.EntityType {
	name := strings.ToLower(sym.Name + " " + sym.QualifiedName + " " + sym.FilePath)
	if strings.Contains(name, "component") || strings.Contains(name, "page") || strings.Contains(name, "button") || strings.Contains(name, ".tsx") || strings.Contains(name, ".jsx") {
		return semanticpb.EntityType_ENTITY_TYPE_UI_COMPONENT
	}
	switch strings.ToLower(sym.Kind) {
	case "function", "method", "constructor":
		return semanticpb.EntityType_ENTITY_TYPE_FUNCTION
	case "class", "struct", "interface", "enum", "record", "trait", "type":
		return semanticpb.EntityType_ENTITY_TYPE_CLASS
	case "module", "package":
		return semanticpb.EntityType_ENTITY_TYPE_MODULE
	}
	for _, fact := range facts {
		if entityType := entityTypeForFact(fact); entityType != semanticpb.EntityType_ENTITY_TYPE_UNSPECIFIED {
			return entityType
		}
	}
	return semanticpb.EntityType_ENTITY_TYPE_MODULE
}

func entityTypeForFact(fact watch.Fact) semanticpb.EntityType {
	kind := strings.ToLower(fact.Type + " " + fact.ObjectKind + " " + fact.Name)
	switch {
	case strings.Contains(kind, "queue"), strings.Contains(kind, "topic"), strings.Contains(kind, "messaging"):
		return semanticpb.EntityType_ENTITY_TYPE_QUEUE
	case strings.Contains(kind, "database"), strings.Contains(kind, "datastore"), strings.Contains(kind, "storage"), strings.Contains(kind, "orm"):
		return semanticpb.EntityType_ENTITY_TYPE_DATABASE
	case strings.Contains(kind, "pipeline"), strings.Contains(kind, "dataeng"), strings.Contains(kind, "job"), strings.Contains(kind, "cron"):
		return semanticpb.EntityType_ENTITY_TYPE_PIPELINE
	case strings.Contains(kind, "cloud"), strings.Contains(kind, "runtime"), strings.Contains(kind, "deployment"), strings.Contains(kind, "iac"), strings.Contains(kind, "terraform"):
		return semanticpb.EntityType_ENTITY_TYPE_INFRASTRUCTURE
	default:
		return semanticpb.EntityType_ENTITY_TYPE_UNSPECIFIED
	}
}

func systemContextFromFacts(facts []watch.Fact) *semanticpb.SystemContext {
	ctx := &semanticpb.SystemContext{CustomAttributes: map[string]string{}}
	for _, fact := range facts {
		attrs := parseAttrs(fact.AttributesJSON)
		value := firstNonEmpty(fact.ObjectName, fact.Name, attrs["name"], attrs["key"], attrs["topic"], attrs["queue"], attrs["route"], attrs["path"], attrs["service"])
		switch factFamily(fact.Type) {
		case "execution":
			ctx.ExecutionBoundaries = append(ctx.ExecutionBoundaries, executionLabel(fact, attrs, value))
		case "trigger":
			ctx.Triggers = append(ctx.Triggers, triggerLabel(fact, attrs, value))
		case "binding":
			ctx.ExternalBindings = append(ctx.ExternalBindings, bindingLabel(fact, attrs, value))
		}
		if fact.Confidence > 0 {
			ctx.CustomAttributes["fact."+fact.Type+".confidence"] = fmt.Sprintf("%.2f", fact.Confidence)
		}
		for _, tag := range fact.Tags {
			if strings.HasPrefix(tag, "owner:") || strings.HasPrefix(tag, "team:") || strings.HasPrefix(tag, "tier:") {
				parts := strings.SplitN(tag, ":", 2)
				ctx.CustomAttributes[parts[0]] = normalizeBoundary(parts[1])
			}
		}
	}
	ctx.ExecutionBoundaries = dedupeStrings(ctx.ExecutionBoundaries)
	ctx.Triggers = dedupeStrings(ctx.Triggers)
	ctx.ExternalBindings = dedupeStrings(ctx.ExternalBindings)
	if len(ctx.CustomAttributes) == 0 {
		ctx.CustomAttributes = nil
	}
	return ctx
}

func factFamily(factType string) string {
	t := strings.ToLower(factType)
	switch {
	case strings.Contains(t, "runtime"), strings.Contains(t, "deployment"), strings.Contains(t, "cloud"), strings.Contains(t, "iac"):
		return "execution"
	case strings.Contains(t, "route"), strings.Contains(t, "endpoint"), strings.Contains(t, "event"), strings.Contains(t, "job"), strings.Contains(t, "cron"), strings.Contains(t, "pipeline"):
		return "trigger"
	case strings.Contains(t, "config"), strings.Contains(t, "env"), strings.Contains(t, "secret"), strings.Contains(t, "storage"), strings.Contains(t, "datastore"), strings.Contains(t, "orm"), strings.Contains(t, "messaging"), strings.Contains(t, "queue"), strings.Contains(t, "topic"):
		return "binding"
	default:
		return ""
	}
}

func executionLabel(fact watch.Fact, attrs factAttrs, value string) string {
	switch {
	case attrs["platform"] != "":
		return attrs["platform"] + ":" + value
	case attrs["service"] != "":
		return "runtime:service/" + attrs["service"]
	case strings.Contains(fact.Type, "cloud"):
		return "cloud:" + value
	default:
		return "runtime:" + firstNonEmpty(value, fact.Type)
	}
}

func triggerLabel(fact watch.Fact, attrs factAttrs, value string) string {
	switch {
	case strings.Contains(fact.Type, "http") || strings.Contains(fact.Type, "route"):
		return "HTTP: " + firstNonEmpty(attrs["method"], "") + " " + value
	case strings.Contains(fact.Type, "frontend"):
		return "Router: " + value
	case strings.Contains(fact.Type, "job") || strings.Contains(fact.Type, "cron"):
		return "Cron: " + value
	default:
		return strings.ToUpper(fact.Type) + ": " + value
	}
}

func bindingLabel(fact watch.Fact, attrs factAttrs, value string) string {
	switch {
	case strings.Contains(fact.Type, "config") || strings.Contains(fact.Type, "env"):
		return "EnvKey: " + value
	case strings.Contains(fact.Type, "secret"):
		return "SecretKey: " + value
	case strings.Contains(fact.Type, "topic"):
		return "Kafka: topic/" + value
	case strings.Contains(fact.Type, "queue"):
		return "Queue: " + value
	case strings.Contains(fact.Type, "storage"):
		return "Storage: " + value
	case strings.Contains(fact.Type, "datastore") || strings.Contains(fact.Type, "orm"):
		return "Datastore: " + value
	default:
		return fact.Type + ": " + value
	}
}

func domainBoundary(sym watch.Symbol, facts []watch.Fact) string {
	for _, fact := range facts {
		for _, tag := range fact.Tags {
			if strings.HasPrefix(tag, "owner:") || strings.HasPrefix(tag, "team:") || strings.HasPrefix(tag, "domain:") {
				parts := strings.SplitN(tag, ":", 2)
				return normalizeBoundary(parts[1])
			}
		}
	}
	return normalizeBoundary(pathBoundary(sym.FilePath))
}

func pathBoundary(filePath string) string {
	for _, part := range strings.Split(filepath.ToSlash(filePath), "/") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch strings.ToLower(part) {
		case "cmd", "internal", "pkg", "src", "app", "apps", "lib", "services", "service", "frontend", "backend":
			continue
		default:
			return part
		}
	}
	return "unknown"
}

func normalizeBoundary(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer("-", "_", " ", "_", ".", "_").Replace(value)
	parts := strings.Split(value, "_")
	acronyms := map[string]string{"ord": "order", "proc": "processing", "svc": "service", "cfg": "config", "db": "database", "authn": "authentication", "authz": "authorization"}
	for i, part := range parts {
		if expanded := acronyms[part]; expanded != "" {
			parts[i] = expanded
		}
	}
	return strings.Join(parts, "_")
}

func languageFromStableKey(stableKey string) string {
	if idx := strings.Index(stableKey, ":"); idx > 0 {
		return stableKey[:idx]
	}
	return ""
}

func languageFromFilePath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".cc", ".cpp", ".cxx", ".hpp", ".h":
		return "cpp"
	case ".tf":
		return "terraform"
	case ".sql":
		return "sql"
	default:
		return ""
	}
}

func parseAttrs(raw string) factAttrs {
	out := factAttrs{}
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}

func prop(name, value, kind string) *semanticpb.Property {
	return &semanticpb.Property{Name: name, Value: value, Kind: kind}
}

func dedupeProperties(values []*semanticpb.Property) []*semanticpb.Property {
	seen := map[string]struct{}{}
	out := values[:0]
	for _, value := range values {
		key := value.Name + "\x00" + value.Value + "\x00" + value.Kind
		if _, ok := seen[key]; ok || strings.TrimSpace(value.Value) == "" {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func dedupeRelationships(values []*semanticpb.Relationship) []*semanticpb.Relationship {
	seen := map[string]struct{}{}
	out := values[:0]
	for _, value := range values {
		key := value.RelationshipType + "\x00" + value.TargetUrn
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func dedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := values[:0]
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func cleanDoc(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.Join(strings.Fields(value), " ")
}

func stripSignature(value string) string {
	value = strings.NewReplacer("{", "", "}", "", ";", "").Replace(value)
	for _, token := range []string{"public ", "private ", "protected ", "static ", "final ", "abstract "} {
		value = strings.ReplaceAll(value, token, "")
	}
	return strings.Join(strings.Fields(value), " ")
}

func narrativeFactDoc(fact watch.Fact) string {
	if entityTypeForFact(fact) == semanticpb.EntityType_ENTITY_TYPE_DATABASE {
		return "Database Entity: " + firstNonEmpty(fact.ObjectName, fact.Name, fact.StableKey) + ". Source fact: " + fact.Type + "."
	}
	return "System context fact: " + firstNonEmpty(fact.Name, fact.Type) + "."
}

func shortHash(value string) string {
	sum := sha1.Sum([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

func sanitizeURNPart(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer(" ", "_", "/", ".", "\\", ".", ":", ".").Replace(value)
	return strings.Trim(value, ".")
}

func nullStringValue(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
