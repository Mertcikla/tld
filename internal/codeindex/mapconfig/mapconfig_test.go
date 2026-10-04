package mapconfig_test

import (
	"reflect"
	"testing"

	"github.com/mertcikla/tld/v2/internal/codeindex/community"
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
	if base.ConfigHash(false) != rebuilt.ConfigHash(false) {
		t.Fatal("config hash is not stable across rebuilds")
	}
	if base.ConfigHash(true) == base.ConfigHash(false) {
		t.Fatal("config hash ignores include_imports")
	}
	mutations := map[string]func(*mapconfig.Options){
		"resolution":         func(o *mapconfig.Options) { o.Grouping.Resolution += 0.5 },
		"min_group_size":     func(o *mapconfig.Options) { o.Grouping.MinGroupSize++ },
		"min_root_groups":    func(o *mapconfig.Options) { o.Grouping.MinRootGroups++ },
		"max_root_groups":    func(o *mapconfig.Options) { o.Grouping.MaxRootGroups++ },
		"max_children":       func(o *mapconfig.Options) { o.Grouping.MaxChildren++ },
		"max_depth":          func(o *mapconfig.Options) { o.Grouping.MaxDepth++ },
		"max_leaf_files":     func(o *mapconfig.Options) { o.Grouping.MaxLeafFiles++ },
		"max_connectors":     func(o *mapconfig.Options) { o.MaxConnectorsPerView++ },
		"max_leaf_connector": func(o *mapconfig.Options) { o.MaxLeafConnectorsPerView++ },
	}
	for name, mutate := range mutations {
		changed := base
		mutate(&changed)
		if base.ConfigHash(false) == changed.ConfigHash(false) {
			t.Fatalf("config hash ignores %s", name)
		}
	}
}
