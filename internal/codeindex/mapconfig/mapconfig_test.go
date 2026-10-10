package mapconfig_test

import (
	"reflect"
	"testing"

	"github.com/mertcikla/codeindex/community"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	"github.com/mertcikla/tld/v2/internal/codeindex/materialize"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestFromGlobalFallsBackToEngineDefaults(t *testing.T) {
	for _, cfg := range []*workspace.Config{nil, {}} {
		opts := mapconfig.FromGlobal(cfg)
		if !reflect.DeepEqual(opts.Grouping, community.DefaultOptions()) {
			t.Fatalf("grouping = %+v, want engine defaults", opts.Grouping)
		}
		if opts.MaxConnectorsPerView != materialize.DefaultMaxConnectorsPerView {
			t.Fatalf("max connectors = %d, want %d", opts.MaxConnectorsPerView, materialize.DefaultMaxConnectorsPerView)
		}
		if opts.MaxLeafConnectorsPerView != materialize.DefaultMaxLeafConnectorsPerView {
			t.Fatalf("max leaf connectors = %d, want %d", opts.MaxLeafConnectorsPerView, materialize.DefaultMaxLeafConnectorsPerView)
		}
		if !opts.AnnotateConnectors || !opts.AnnotateTags || !opts.AnnotateTechnology || !opts.GroupLayers {
			t.Fatalf("annotation must default on: %+v", opts)
		}
		if !opts.CrossViewConnectors {
			t.Fatalf("cross-view connectors must default on: %+v", opts)
		}
		if opts.CrossViewMaxViews != materialize.DefaultCrossViewMaxViews ||
			opts.CrossViewMaxElementsPerView != materialize.DefaultCrossViewMaxElementsPerView ||
			opts.CrossViewMaxConnectorsPerView != materialize.DefaultCrossViewMaxConnectorsPerView ||
			opts.CrossViewMaxConnectorsPerElement != materialize.DefaultCrossViewMaxConnectorsPerElement {
			t.Fatalf("cross-view limits drifted: %+v", opts)
		}
	}
}

func TestFromGlobalHonorsAnnotationOptOut(t *testing.T) {
	disabled := false
	cfg := workspace.DefaultConfig()
	cfg.Map.Annotate = workspace.MapAnnotateConfig{Connectors: &disabled, GroupLayers: &disabled}
	opts := mapconfig.FromGlobal(cfg)
	if opts.AnnotateConnectors || opts.GroupLayers {
		t.Fatalf("annotation opt-out ignored: %+v", opts)
	}
	if !opts.AnnotateTags || !opts.AnnotateTechnology {
		t.Fatalf("unset annotation flags must stay on: %+v", opts)
	}
}

func TestDefaultConfigMatchesEngineDefaults(t *testing.T) {
	opts := mapconfig.FromGlobal(workspace.DefaultConfig())
	if !reflect.DeepEqual(opts.Grouping, community.DefaultOptions()) {
		t.Fatalf("workspace defaults drifted from community defaults: %+v", opts.Grouping)
	}
	if opts.MaxConnectorsPerView != materialize.DefaultMaxConnectorsPerView || opts.MaxLeafConnectorsPerView != materialize.DefaultMaxLeafConnectorsPerView {
		t.Fatalf("workspace budget defaults drifted: %+v", opts)
	}
}

func TestFromGlobalAppliesOverrides(t *testing.T) {
	cfg := workspace.DefaultConfig()
	cfg.Map.Grouping.Resolution = 1.4
	cfg.Map.Grouping.MinGroupSize = 3
	cfg.Map.Grouping.MinRootGroups = 4
	cfg.Map.Grouping.MaxRootGroups = 9
	cfg.Map.Grouping.MaxChildren = 5
	cfg.Map.Grouping.MaxDepth = 2
	cfg.Map.Grouping.MaxLeafFiles = 12
	cfg.Map.Budget.MaxConnectorsPerView = 30
	cfg.Map.Budget.MaxLeafConnectorsPerView = 6
	disabled := false
	cfg.Map.CrossView = workspace.MapCrossViewConfig{
		Connectors:              &disabled,
		MaxViews:                3,
		MaxElementsPerView:      4,
		MaxConnectorsPerView:    5,
		MaxConnectorsPerElement: 6,
	}

	opts := mapconfig.FromGlobal(cfg)
	want := community.Options{
		Resolution:    1.4,
		MinGroupSize:  3,
		MinRootGroups: 4,
		MaxRootGroups: 9,
		MaxChildren:   5,
		MaxDepth:      2,
		MaxLeafFiles:  12,
	}
	if !reflect.DeepEqual(opts.Grouping, want) {
		t.Fatalf("grouping = %+v, want %+v", opts.Grouping, want)
	}
	if opts.MaxConnectorsPerView != 30 || opts.MaxLeafConnectorsPerView != 6 {
		t.Fatalf("budgets = %+v", opts)
	}
	if opts.CrossViewConnectors || opts.CrossViewMaxViews != 3 || opts.CrossViewMaxElementsPerView != 4 || opts.CrossViewMaxConnectorsPerView != 5 || opts.CrossViewMaxConnectorsPerElement != 6 {
		t.Fatalf("cross-view = %+v", opts)
	}
}

func TestFromGlobalRepairsInvertedRootBounds(t *testing.T) {
	cfg := &workspace.Config{Map: workspace.MapConfig{Grouping: workspace.MapGroupingConfig{MinRootGroups: 50}}}
	opts := mapconfig.FromGlobal(cfg)
	if opts.Grouping.MaxRootGroups < opts.Grouping.MinRootGroups {
		t.Fatalf("root bounds inverted: %+v", opts.Grouping)
	}
}

func TestConfigHashTracksEverySetting(t *testing.T) {
	base := mapconfig.FromGlobal(workspace.DefaultConfig())
	rebuilt := mapconfig.FromGlobal(workspace.DefaultConfig())
	if base.ConfigHash() != rebuilt.ConfigHash() {
		t.Fatal("config hash is not stable across rebuilds")
	}
	mutations := map[string]func(*mapconfig.Options){
		"resolution":                            func(o *mapconfig.Options) { o.Grouping.Resolution += 0.5 },
		"min_group_size":                        func(o *mapconfig.Options) { o.Grouping.MinGroupSize++ },
		"min_root_groups":                       func(o *mapconfig.Options) { o.Grouping.MinRootGroups++ },
		"max_root_groups":                       func(o *mapconfig.Options) { o.Grouping.MaxRootGroups++ },
		"max_children":                          func(o *mapconfig.Options) { o.Grouping.MaxChildren++ },
		"max_depth":                             func(o *mapconfig.Options) { o.Grouping.MaxDepth++ },
		"max_leaf_files":                        func(o *mapconfig.Options) { o.Grouping.MaxLeafFiles++ },
		"max_connectors":                        func(o *mapconfig.Options) { o.MaxConnectorsPerView++ },
		"max_leaf_connector":                    func(o *mapconfig.Options) { o.MaxLeafConnectorsPerView++ },
		"annotate_connectors":                   func(o *mapconfig.Options) { o.AnnotateConnectors = !o.AnnotateConnectors },
		"annotate_tags":                         func(o *mapconfig.Options) { o.AnnotateTags = !o.AnnotateTags },
		"annotate_technology":                   func(o *mapconfig.Options) { o.AnnotateTechnology = !o.AnnotateTechnology },
		"group_layers":                          func(o *mapconfig.Options) { o.GroupLayers = !o.GroupLayers },
		"cross_view_connectors":                 func(o *mapconfig.Options) { o.CrossViewConnectors = !o.CrossViewConnectors },
		"cross_view_max_views":                  func(o *mapconfig.Options) { o.CrossViewMaxViews++ },
		"cross_view_max_elements_per_view":      func(o *mapconfig.Options) { o.CrossViewMaxElementsPerView++ },
		"cross_view_max_connectors_per_view":    func(o *mapconfig.Options) { o.CrossViewMaxConnectorsPerView++ },
		"cross_view_max_connectors_per_element": func(o *mapconfig.Options) { o.CrossViewMaxConnectorsPerElement++ },
	}
	for name, mutate := range mutations {
		changed := base
		mutate(&changed)
		if base.ConfigHash() == changed.ConfigHash() {
			t.Fatalf("config hash ignores %s", name)
		}
	}
}

func TestConfigHashIsImportModeSpecific(t *testing.T) {
	options := mapconfig.FromGlobal(workspace.DefaultConfig())
	if options.IncludeExternalImports {
		t.Fatal("external imports must default off")
	}
	other := options
	other.IncludeExternalImports = true
	if options.ConfigHash() == other.ConfigHash() {
		t.Fatal("config hash must differ across external-import modes")
	}
}
