package workspace

import (
	"fmt"
	"time"

	hashidlib "github.com/mertcikla/tld/internal/hashids"
	"github.com/mertcikla/tld/internal/ignore"
)

// WorkspaceConfig is parsed from the workspace-local .tld.yaml.
type WorkspaceConfig struct {
	ProjectName  string                `yaml:"project_name,omitempty"`
	Exclude      []string              `yaml:"exclude,omitempty"`
	Repositories map[string]Repository `yaml:"repositories,omitempty"`
}

// RepositoryConfig holds per-repository behavior flags.
type RepositoryConfig struct {
	Mode string `yaml:"mode,omitempty"`
}

// Repository describes one repository in a multi-repo workspace.
type Repository struct {
	URL      string            `yaml:"url,omitempty"`
	LocalDir string            `yaml:"localDir,omitempty"`
	Root     string            `yaml:"root,omitempty"`
	Config   *RepositoryConfig `yaml:"config,omitempty"`
	Exclude  []string          `yaml:"exclude,omitempty"`
}

// ViewPlacement is an element placement within another element's internal view.
// Parent "root" means the synthetic workspace root.
type ViewPlacement struct {
	ParentRef string  `yaml:"parent"`
	PositionX float64 `yaml:"position_x,omitempty"`
	PositionY float64 `yaml:"position_y,omitempty"`
}

// Element is the primary workspace resource.
// It combines reusable identity with optional internal view metadata.
type Element struct {
	Name        string          `yaml:"name"`
	Kind        string          `yaml:"kind"`
	Owner       string          `yaml:"owner,omitempty"`
	Description string          `yaml:"description,omitempty"`
	Technology  string          `yaml:"technology,omitempty"`
	URL         string          `yaml:"url,omitempty"`
	LogoURL     string          `yaml:"logo_url,omitempty"`
	Repo        string          `yaml:"repo,omitempty"`
	Branch      string          `yaml:"branch,omitempty"`
	Language    string          `yaml:"language,omitempty"`
	FilePath    string          `yaml:"file_path,omitempty"`
	Symbol      string          `yaml:"symbol,omitempty"` // Named code symbol within FilePath (e.g. "MyFunc")
	Tags        []string        `yaml:"tags,omitempty"`
	HasView     bool            `yaml:"has_view,omitempty"`
	ViewLabel   string          `yaml:"view_label,omitempty"`
	Placements  []ViewPlacement `yaml:"placements,omitempty"`
}

// Connector is one entry in connectors.yaml.
type Connector struct {
	View         string     `yaml:"view"`
	Source       string     `yaml:"source"`
	Target       string     `yaml:"target"`
	Label        string     `yaml:"label,omitempty"`
	Description  string     `yaml:"description,omitempty"`
	Relationship string     `yaml:"relationship,omitempty"`
	Direction    string     `yaml:"direction,omitempty"`
	Style        string     `yaml:"style,omitempty"`
	URL          string     `yaml:"url,omitempty"`
	SourceHandle string     `yaml:"source_handle,omitempty"`
	TargetHandle string     `yaml:"target_handle,omitempty"`
	ID           ResourceID `yaml:"id,omitempty"`
	UpdatedAt    time.Time  `yaml:"updated_at,omitempty"`
}

// ResourceID is an int32 that serializes to Hashids in YAML
type ResourceID int32

// MarshalYAML serialises a ResourceID to its Hashid string representation.
func (r ResourceID) MarshalYAML() (any, error) {
	if r == 0 {
		return nil, nil
	}
	return hashidlib.Encode(int32(r)), nil
}

// UnmarshalYAML deserialises a Hashid string back to a ResourceID.
func (r *ResourceID) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	if s == "" {
		*r = 0
		return nil
	}
	id, err := hashidlib.Decode(s)
	if err != nil {
		return fmt.Errorf("decode resource id: %w", err)
	}
	*r = ResourceID(id)
	return nil
}

// ResourceMetadata tracks system IDs and timestamps for resources
type ResourceMetadata struct {
	ID        ResourceID `yaml:"id"`
	UpdatedAt time.Time  `yaml:"updated_at"`
	Conflict  bool       `yaml:"conflict,omitempty"` // True if both local and server changed since last sync
}

// LockFile tracks workspace versioning and change history
type LockFile struct {
	Version           string                       `yaml:"version"`    // "v1"
	VersionID         string                       `yaml:"version_id"` // Workspace version UUID
	LastApply         time.Time                    `yaml:"last_apply"`
	AppliedBy         string                       `yaml:"applied_by"` // "cli" or "frontend"
	Resources         *ResourceCounts              `yaml:"resources"`
	WorkspaceHash     string                       `yaml:"workspace_hash"`               // Hash of all YAML files
	ParentVersion     *string                      `yaml:"parent_version,omitempty"`     // Previous version
	Metadata          *Meta                        `yaml:"metadata,omitempty"`           // Metadata at time of last sync
	CurrentElements   map[string]*ResourceMetadata `yaml:"current_elements,omitempty"`   // Current local element metadata, migrated from _meta_elements
	CurrentViews      map[string]*ResourceMetadata `yaml:"current_views,omitempty"`      // Current local view metadata, migrated from _meta_views
	CurrentConnectors map[string]*ResourceMetadata `yaml:"current_connectors,omitempty"` // Current local connector metadata; connector timestamps now live here
}

// ResourceCounts holds current model counts plus legacy fields retained for lockfile compatibility.
type ResourceCounts struct {
	Elements   int `yaml:"elements,omitempty"`
	Views      int `yaml:"views,omitempty"`
	Connectors int `yaml:"connectors,omitempty"`
}

// Workspace holds the fully loaded workspace state
type Workspace struct {
	Dir             string
	Config          Config
	WorkspaceConfig *WorkspaceConfig
	Elements        map[string]*Element // key = ref
	Connectors      map[string]*Connector
	Meta            *Meta         // Loaded from separate _meta sections
	IgnoreRules     *ignore.Rules // Loaded from workspace config; nil if file absent
	ActiveRepo      string        // active repository scope for plan/apply operations
}

// Meta contains current-model metadata plus legacy buckets retained for compatibility with older lockfiles and exports.
type Meta struct {
	Elements   map[string]*ResourceMetadata
	Views      map[string]*ResourceMetadata
	Connectors map[string]*ResourceMetadata
}
