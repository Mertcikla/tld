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
	// IncludeExternalImports materializes external imports under a single
	// External container. Import usages are rolled up per top-level component so
	// enabling this stays bounded by the connector budget rather than emitting
	// one connector per (file, import) pair.
	IncludeExternalImports bool
	// AnnotateConnectors writes relationship labels and tags on connectors.
	AnnotateConnectors bool
	// AnnotateTags writes language and test tags on elements.
	AnnotateTags bool
	// AnnotateTechnology writes catalog technology onto elements.
	AnnotateTechnology bool
	// GroupLayers creates a view layer per community group.
	GroupLayers bool
}

// FromGlobal resolves global config into effective map options. Zero-valued
// fields fall back to production defaults so partial or nil configs are safe.
func FromGlobal(cfg *workspace.Config) Options {
	defaults := community.DefaultOptions()
	out := Options{
		Grouping:                 defaults,
		MaxConnectorsPerView:     materialize.DefaultMaxConnectorsPerView,
		MaxLeafConnectorsPerView: materialize.DefaultMaxLeafConnectorsPerView,
		AnnotateConnectors:       true,
		AnnotateTags:             true,
		AnnotateTechnology:       true,
		GroupLayers:              true,
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
	applyBool := func(value *bool, target *bool) {
		if value != nil {
			*target = *value
		}
	}
	applyBool(cfg.Map.Annotate.Connectors, &out.AnnotateConnectors)
	applyBool(cfg.Map.Annotate.Tags, &out.AnnotateTags)
	applyBool(cfg.Map.Annotate.Technology, &out.AnnotateTechnology)
	applyBool(cfg.Map.Annotate.GroupLayers, &out.GroupLayers)

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
		"annotate_connectors=" + strconv.FormatBool(o.AnnotateConnectors),
		"annotate_tags=" + strconv.FormatBool(o.AnnotateTags),
		"annotate_technology=" + strconv.FormatBool(o.AnnotateTechnology),
		"group_layers=" + strconv.FormatBool(o.GroupLayers),
	}, ",")
}

// ConfigHash identifies a completed map produced with these options. It must
// change whenever a setting that affects map output changes so cached maps are
// not reused across configurations.
func (o Options) ConfigHash() string {
	return cgraph.ID("group-v7", strconv.FormatBool(o.IncludeExternalImports), o.Fingerprint())
}

// MaterializeOptions builds the materialization options for these settings.
func (o Options) MaterializeOptions(progress func(current, total int, detail string)) materialize.MapOptions {
	return materialize.MapOptions{
		MaxConnectorsPerView:     o.MaxConnectorsPerView,
		MaxLeafConnectorsPerView: o.MaxLeafConnectorsPerView,
		IncludeExternalImports:   o.IncludeExternalImports,
		AnnotateConnectors:       o.AnnotateConnectors,
		AnnotateTags:             o.AnnotateTags,
		AnnotateTechnology:       o.AnnotateTechnology,
		GroupLayers:              o.GroupLayers,
		Progress:                 progress,
	}
}
