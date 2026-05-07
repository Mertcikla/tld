package enrich

import (
	"context"

	"github.com/mertcikla/tld/internal/analyzer"
)

type ActivationMode string

const (
	ActivationAlways             ActivationMode = "always"
	ActivationImportOrDependency ActivationMode = "import_or_dependency"
)

const (
	SignalDependency = "dependency"
	SignalImport     = "import"
)

type ActivationSignal struct {
	Kind   string
	Value  string
	Source string
}

type Metadata struct {
	ID       string
	Name     string
	Mode     ActivationMode
	Triggers []ActivationSignal
}

type SourceSpan struct {
	FilePath    string
	StartLine   int
	EndLine     int
	StartColumn int
	EndColumn   int
}

type SubjectRef struct {
	Kind      string
	StableKey string
	FilePath  string
	Name      string
}

type Fact struct {
	Type            string
	StableKey       string
	Enricher        string
	Subject         SubjectRef
	Object          SubjectRef
	Relationship    string
	Source          SourceSpan
	Confidence      float64
	Name            string
	Tags            []string
	Attributes      map[string]string
	VisibilityHints map[string]float64
}

type FactEmitter interface {
	EmitFact(Fact) error
	Warn(Warning)
}

type Warning struct {
	Enricher string
	FilePath string
	Message  string
}

type FileInput struct {
	RepoRoot string
	AbsPath  string
	RelPath  string
	Language string
	Source   []byte
	Parsed   *analyzer.Result
	Signals  []ActivationSignal
}

type Enricher interface {
	Metadata() Metadata
	MatchFile(FileInput) bool
	EnrichFile(context.Context, FileInput, FactEmitter) error
}
