// Package mapconfig resolves the graph map pipeline settings from global
// configuration into the effective grouping and materialization options.
package mapconfig

import (
	"strconv"
	"strings"

	"github.com/mertcikla/tld/v2/internal/codeindex/community"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// Options is the effective graph map pipeline configuration.
type Options struct {
	Grouping                 community.Options
	MaxConnectorsPerView     int
	MaxLeafConnectorsPerView int
}

// FromGlobal resolves global config into effective map options. Zero-valued
// fields fall back to production defaults so partial or nil configs are safe.
func FromGlobal(cfg *workspace.Config) Options {
	defaults := community.DefaultOptions()
	out := Options{
		Grouping:                 defaults,
		MaxConnectorsPerView:     materialize.DefaultMaxConnectorsPerView,
		MaxLeafConnectorsPerView: materialize.DefaultMaxLeafConnectorsPerView,
	}
	if cfg == nil {
		return out
	}
	grouping := cfg.Map.Grouping
	if grouping.Resolution > 0 {
		out.Grouping.Resolution = grouping.Resolution
	}
	if grouping.MinGroupSize > 0 {
		out.Grouping.MinGroupSize = grouping.MinGroupSize
	}
	if grouping.MinRootGroups > 0 {
		out.Grouping.MinRootGroups = grouping.MinRootGroups
	}
	if grouping.MaxRootGroups > 0 {
		out.Grouping.MaxRootGroups = grouping.MaxRootGroups
	}
	if grouping.MaxChildren > 0 {
		out.Grouping.MaxChildren = grouping.MaxChildren
	}
	if grouping.MaxDepth > 0 {
		out.Grouping.MaxDepth = grouping.MaxDepth
	}
	if grouping.MaxLeafFiles > 0 {
		out.Grouping.MaxLeafFiles = grouping.MaxLeafFiles
	}
	if out.Grouping.MaxRootGroups < out.Grouping.MinRootGroups {
		out.Grouping.MaxRootGroups = out.Grouping.MinRootGroups
	}
	if cfg.Map.Budget.MaxConnectorsPerView > 0 {
		out.MaxConnectorsPerView = cfg.Map.Budget.MaxConnectorsPerView
	}
	if cfg.Map.Budget.MaxLeafConnectorsPerView > 0 {
		out.MaxLeafConnectorsPerView = cfg.Map.Budget.MaxLeafConnectorsPerView
	}
	return out
}

// Fingerprint renders the effective options as a stable cache key component.
func (o Options) Fingerprint() string {
	grouping := o.Grouping
	return strings.Join([]string{
		"resolution=" + strconv.FormatFloat(grouping.Resolution, 'g', -1, 64),
		"min_group_size=" + strconv.Itoa(grouping.MinGroupSize),
		"min_root_groups=" + strconv.Itoa(grouping.MinRootGroups),
		"max_root_groups=" + strconv.Itoa(grouping.MaxRootGroups),
		"max_children=" + strconv.Itoa(grouping.MaxChildren),
		"max_depth=" + strconv.Itoa(grouping.MaxDepth),
		"max_leaf_files=" + strconv.Itoa(grouping.MaxLeafFiles),
		"max_connectors_per_view=" + strconv.Itoa(o.MaxConnectorsPerView),
		"max_leaf_connectors_per_view=" + strconv.Itoa(o.MaxLeafConnectorsPerView),
	}, ",")
}

// ConfigHash identifies a completed map produced with these options. It must
// change whenever a setting that affects map output changes so cached maps are
// not reused across configurations. External imports are always part of a map.
func (o Options) ConfigHash() string {
	return cgraph.ID("group-v4", o.Fingerprint())
}

// MaterializeOptions builds the materialization options for these settings.
func (o Options) MaterializeOptions(progress func(current, total int, detail string)) materialize.MapOptions {
	return materialize.MapOptions{
		MaxConnectorsPerView:     o.MaxConnectorsPerView,
		MaxLeafConnectorsPerView: o.MaxLeafConnectorsPerView,
		Progress:                 progress,
	}
}
