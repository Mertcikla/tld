package mapconfig_test

import (
	"math"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"google.golang.org/protobuf/proto"
)

func TestRepositoryOverridesInheritAndValidate(t *testing.T) {
	cfg := workspace.DefaultConfig()
	cfg.Map.Grouping.Resolution = 1.7
	defaults := mapconfig.FromGlobal(cfg)
	overrides := &pb.RepositoryMapConfiguration{MaxLeafFiles: proto.Uint32(17)}
	effective, err := defaults.WithOverrides(overrides)
	if err != nil {
		t.Fatal(err)
	}
	if effective.Grouping.MaxLeafFiles != 17 || effective.Grouping.Resolution != 1.7 || defaults.Grouping.MaxLeafFiles != 40 {
		t.Fatalf("inheritance mutated defaults or lost override: defaults=%+v effective=%+v", defaults, effective)
	}
	if defaults.ConfigHash() == effective.ConfigHash() {
		t.Fatal("override did not change cache identity")
	}
	for _, invalid := range []*pb.RepositoryMapConfiguration{
		{Resolution: proto.Float64(math.NaN())}, {Resolution: proto.Float64(math.Inf(1))},
		{Resolution: proto.Float64(0)}, {MaxDepth: proto.Uint32(0)},
		{MinRootGroups: proto.Uint32(21)}, {MaxRootGroups: proto.Uint32(2)},
		{MaxChildren: proto.Uint32(math.MaxUint32)},
		{CrossViewMaxConnectorsPerView: proto.Uint32(0)},
	} {
		if _, err := defaults.WithOverrides(invalid); err == nil {
			t.Fatalf("accepted invalid override: %v", invalid)
		}
	}
	reset, err := effective.WithOverrides(nil)
	if err != nil || reset.Fingerprint() != effective.Fingerprint() {
		t.Fatalf("nil overrides must preserve the base: %+v %v", reset, err)
	}
}

func TestRepositoryOverridesExternalImports(t *testing.T) {
	defaults := mapconfig.FromGlobal(workspace.DefaultConfig())
	if defaults.IncludeExternalImports {
		t.Fatal("external imports must default off")
	}
	on, err := defaults.WithOverrides(&pb.RepositoryMapConfiguration{IncludeExternalImports: proto.Bool(true)})
	if err != nil || !on.IncludeExternalImports {
		t.Fatalf("enable override: %+v %v", on, err)
	}
	off, err := defaults.WithOverrides(&pb.RepositoryMapConfiguration{IncludeExternalImports: proto.Bool(false)})
	if err != nil || off.IncludeExternalImports {
		t.Fatalf("explicit false override must be accepted: %+v %v", off, err)
	}
}

func TestRepositoryOverridesCrossView(t *testing.T) {
	defaults := mapconfig.FromGlobal(workspace.DefaultConfig())
	if !defaults.CrossViewConnectors {
		t.Fatal("cross-view connectors must default on")
	}
	if defaults.CrossViewMaxViews != 8 || defaults.CrossViewMaxConnectorsPerView != 8 {
		t.Fatalf("cross-view defaults = %+v", defaults)
	}
	effective, err := defaults.WithOverrides(&pb.RepositoryMapConfiguration{
		CrossViewConnectors:              proto.Bool(false),
		CrossViewMaxViews:                proto.Uint32(3),
		CrossViewMaxElementsPerView:      proto.Uint32(4),
		CrossViewMaxConnectorsPerView:    proto.Uint32(5),
		CrossViewMaxConnectorsPerElement: proto.Uint32(6),
	})
	if err != nil {
		t.Fatal(err)
	}
	if effective.CrossViewConnectors || effective.CrossViewMaxViews != 3 || effective.CrossViewMaxElementsPerView != 4 || effective.CrossViewMaxConnectorsPerView != 5 || effective.CrossViewMaxConnectorsPerElement != 6 {
		t.Fatalf("cross-view override = %+v", effective)
	}
	if defaults.CrossViewMaxConnectorsPerView != 8 {
		t.Fatal("override mutated defaults")
	}
	config := effective.Configuration()
	if config.GetCrossViewConnectors() || config.GetCrossViewMaxConnectorsPerView() != 5 {
		t.Fatalf("configuration lost cross-view values: %+v", config)
	}
}
