package workspace

import "time"

// TargetState owns the IDs and pull baseline for exactly one destination. The
// top-level lockfile fields mirror ActiveTarget for YAML-only editing helpers.
type TargetState struct {
	VersionID         string                       `yaml:"version_id,omitempty"`
	LastApply         time.Time                    `yaml:"last_apply,omitempty"`
	AppliedBy         string                       `yaml:"applied_by,omitempty"`
	Resources         *ResourceCounts              `yaml:"resources,omitempty"`
	WorkspaceHash     string                       `yaml:"workspace_hash,omitempty"`
	ParentVersion     *string                      `yaml:"parent_version,omitempty"`
	Metadata          *Meta                        `yaml:"metadata,omitempty"`
	CurrentElements   map[string]*ResourceMetadata `yaml:"current_elements,omitempty"`
	CurrentViews      map[string]*ResourceMetadata `yaml:"current_views,omitempty"`
	CurrentConnectors map[string]*ResourceMetadata `yaml:"current_connectors,omitempty"`
	WorkspaceSource   *WorkspaceSourceLock         `yaml:"workspace_source,omitempty"`
}

func (l *LockFile) rememberTarget() {
	if l.ActiveTarget == "" {
		return
	}
	if l.Targets == nil {
		l.Targets = make(map[string]*TargetState)
	}
	l.Targets[l.ActiveTarget] = &TargetState{
		VersionID: l.VersionID, LastApply: l.LastApply, AppliedBy: l.AppliedBy,
		Resources: cloneResourceCounts(l.Resources), WorkspaceHash: l.WorkspaceHash,
		ParentVersion: l.ParentVersion, Metadata: cloneMeta(l.Metadata),
		CurrentElements:   cloneResourceMetadataMap(l.CurrentElements),
		CurrentViews:      cloneResourceMetadataMap(l.CurrentViews),
		CurrentConnectors: cloneResourceMetadataMap(l.CurrentConnectors),
		WorkspaceSource:   l.WorkspaceSource,
	}
}

// SelectTarget changes only this in-memory lockfile. Unscoped legacy IDs cannot
// be attributed safely and are deliberately not adopted by the first target.
func (l *LockFile) SelectTarget(key string) {
	if key == l.ActiveTarget {
		return
	}
	l.rememberTarget()
	state := l.Targets[key]
	if state == nil {
		state = &TargetState{}
	}
	l.ActiveTarget = key
	l.VersionID, l.LastApply, l.AppliedBy = state.VersionID, state.LastApply, state.AppliedBy
	l.Resources, l.WorkspaceHash, l.ParentVersion = cloneResourceCounts(state.Resources), state.WorkspaceHash, state.ParentVersion
	l.Metadata = cloneMeta(state.Metadata)
	l.CurrentElements = cloneResourceMetadataMap(state.CurrentElements)
	l.CurrentViews = cloneResourceMetadataMap(state.CurrentViews)
	l.CurrentConnectors = cloneResourceMetadataMap(state.CurrentConnectors)
	l.WorkspaceSource = state.WorkspaceSource
}

func (l *LockFile) currentMetadata() *Meta {
	meta := &Meta{
		Elements:   make(map[string]*ResourceMetadata),
		Views:      make(map[string]*ResourceMetadata),
		Connectors: make(map[string]*ResourceMetadata),
	}
	copyResourceMetadataMap(meta.Elements, l.CurrentElements)
	copyResourceMetadataMap(meta.Views, l.CurrentViews)
	copyResourceMetadataMap(meta.Connectors, l.CurrentConnectors)
	return meta
}

// BindTarget selects IDs without writing files, including for dry runs. Inline
// connector IDs and legacy metadata must not leak into a different destination.
func BindTarget(ws *Workspace, key string) (*LockFile, error) {
	lock, err := LoadLockFile(ws.Dir)
	if err != nil {
		return nil, err
	}
	if lock == nil {
		lock = &LockFile{Version: "v1"}
	}
	lock.SelectTarget(key)
	ws.CacheTarget = key
	ws.Meta = lock.currentMetadata()
	for ref, connector := range ws.Connectors {
		if connector == nil {
			continue
		}
		connector.ID, connector.UpdatedAt = 0, time.Time{}
		if meta := ws.Meta.Connectors[ref]; meta != nil {
			connector.ID, connector.UpdatedAt = meta.ID, meta.UpdatedAt
		}
	}
	return lock, nil
}

// ActivateTarget persists the selected namespace before existing YAML editing
// helpers update its current metadata. Call only on a write path.
func ActivateTarget(dir, key string) error {
	if key == "" {
		return nil
	}
	lock, err := LoadLockFile(dir)
	if err != nil {
		return err
	}
	if lock == nil {
		lock = &LockFile{Version: "v1"}
	}
	if lock.ActiveTarget == key {
		return nil
	}
	lock.SelectTarget(key)
	return WriteLockFile(dir, lock)
}

// editAllTargets applies a YAML-only edit, such as a ref rename, to every
// destination's current metadata without changing its historical baseline.
func (l *LockFile) editAllTargets(edit func(*LockFile)) {
	active := l.ActiveTarget
	edit(l)
	for key := range l.Targets {
		if key == active {
			continue
		}
		l.SelectTarget(key)
		edit(l)
	}
	l.SelectTarget(active)
}
