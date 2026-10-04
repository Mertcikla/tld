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
	if defaults.ConfigHash(false) == effective.ConfigHash(false) {
		t.Fatal("override did not change cache identity")
	}
	for _, invalid := range []*pb.RepositoryMapConfiguration{
		{Resolution: proto.Float64(math.NaN())}, {Resolution: proto.Float64(math.Inf(1))},
		{Resolution: proto.Float64(0)}, {MaxDepth: proto.Uint32(0)},
		{MinRootGroups: proto.Uint32(21)}, {MaxRootGroups: proto.Uint32(2)},
		{MaxChildren: proto.Uint32(math.MaxUint32)},
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
