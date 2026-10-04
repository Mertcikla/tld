package mapconfig

import (
	"fmt"
	"math"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// WithOverrides resolves a sparse per-repository configuration on top of the
// effective global defaults. Presence matters: an explicit zero is invalid.
func (o Options) WithOverrides(overrides *pb.RepositoryMapConfiguration) (Options, error) {
	if overrides == nil {
		return o, nil
	}
	var invalid error
	overrides.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() == protoreflect.DoubleKind {
			n := value.Float()
			if n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
				invalid = fmt.Errorf("%s must be a finite positive number", field.Name())
			}
		} else if value.Uint() == 0 || value.Uint() > math.MaxInt32 {
			invalid = fmt.Errorf("%s must be a positive integer no larger than %d", field.Name(), math.MaxInt32)
		}
		return invalid == nil
	})
	if invalid != nil {
		return o, invalid
	}
	if overrides.Resolution != nil {
		o.Grouping.Resolution = overrides.GetResolution()
	}
	if overrides.MinGroupSize != nil {
		o.Grouping.MinGroupSize = int(overrides.GetMinGroupSize())
	}
	if overrides.MinRootGroups != nil {
		o.Grouping.MinRootGroups = int(overrides.GetMinRootGroups())
	}
	if overrides.MaxRootGroups != nil {
		o.Grouping.MaxRootGroups = int(overrides.GetMaxRootGroups())
	}
	if overrides.MaxChildren != nil {
		o.Grouping.MaxChildren = int(overrides.GetMaxChildren())
	}
	if overrides.MaxDepth != nil {
		o.Grouping.MaxDepth = int(overrides.GetMaxDepth())
	}
	if overrides.MaxLeafFiles != nil {
		o.Grouping.MaxLeafFiles = int(overrides.GetMaxLeafFiles())
	}
	if overrides.MaxConnectorsPerView != nil {
		o.MaxConnectorsPerView = int(overrides.GetMaxConnectorsPerView())
	}
	if overrides.MaxLeafConnectorsPerView != nil {
		o.MaxLeafConnectorsPerView = int(overrides.GetMaxLeafConnectorsPerView())
	}
	if o.Grouping.MinRootGroups > o.Grouping.MaxRootGroups {
		return o, fmt.Errorf("maximum root groups must be at least minimum root groups (including inherited defaults)")
	}
	return o, nil
}

// Configuration exposes every resolved value for inheritance-aware editors.
func (o Options) Configuration() *pb.RepositoryMapConfiguration {
	return &pb.RepositoryMapConfiguration{
		Resolution:               proto.Float64(o.Grouping.Resolution),
		MinGroupSize:             proto.Uint32(uint32(o.Grouping.MinGroupSize)),
		MinRootGroups:            proto.Uint32(uint32(o.Grouping.MinRootGroups)),
		MaxRootGroups:            proto.Uint32(uint32(o.Grouping.MaxRootGroups)),
		MaxChildren:              proto.Uint32(uint32(o.Grouping.MaxChildren)),
		MaxDepth:                 proto.Uint32(uint32(o.Grouping.MaxDepth)),
		MaxLeafFiles:             proto.Uint32(uint32(o.Grouping.MaxLeafFiles)),
		MaxConnectorsPerView:     proto.Uint32(uint32(o.MaxConnectorsPerView)),
		MaxLeafConnectorsPerView: proto.Uint32(uint32(o.MaxLeafConnectorsPerView)),
	}
}
