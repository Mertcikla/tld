package workspacesource

import (
	"time"

	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/api"
)

const ViewFileName = "view.md"

type Options struct {
	WorkspaceDir   string
	RepositoryName string
	WorkspaceID    uuid.UUID
}

type Status struct {
	Available bool   `json:"available"`
	RootPath  string `json:"root_path,omitempty"`
	ViewsDir  string `json:"views_dir,omitempty"`
	Message   string `json:"message,omitempty"`
}

type ChangeCounts struct {
	Planned int `json:"planned"`
	Applied int `json:"applied"`
	Created int `json:"created"`
	Updated int `json:"updated"`
	Deleted int `json:"deleted"`
}

type Result struct {
	Available  bool         `json:"available"`
	DryRun     bool         `json:"dry_run,omitempty"`
	RootPath   string       `json:"root_path,omitempty"`
	ViewsDir   string       `json:"views_dir,omitempty"`
	Hash       string       `json:"hash,omitempty"`
	Views      ChangeCounts `json:"views"`
	Elements   ChangeCounts `json:"elements"`
	Connectors ChangeCounts `json:"connectors"`
	Warnings   []string     `json:"warnings,omitempty"`
	Message    string       `json:"message,omitempty"`
}

type Store interface {
	api.Store
}

type desiredWorkspace struct {
	Views          map[string]*desiredView
	Elements       map[string]*desiredElement
	Connectors     map[string]*desiredConnector
	Placements     []desiredPlacement
	ViewOrder      []string
	ElementOrder   []string
	ConnectorOrder []string
	Warnings       []string
	SourceHash     string
	RootPath       string
}

type desiredView struct {
	Ref       string
	ParentRef string
	Name      string
	OwnerRef  string
	Path      string
	UpdatedAt time.Time
}

type desiredElement struct {
	Ref             string
	Name            string
	Kind            string
	Description     string
	Technology      string
	URL             string
	LogoURL         string
	Tags            []string
	Repo            string
	Branch          string
	Language        string
	FilePath        string
	BypassNoiseGate *bool
	HasView         bool
	ViewLabel       string
}

type desiredPlacement struct {
	ViewRef    string
	ElementRef string
	X          float64
	Y          float64
}

type desiredConnector struct {
	Ref          string
	ViewRef      string
	SourceRef    string
	TargetRef    string
	Label        string
	Description  string
	Relationship string
	Direction    string
	Style        string
	URL          string
	SourceHandle string
	TargetHandle string
}
