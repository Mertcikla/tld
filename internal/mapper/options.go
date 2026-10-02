// Package mapper is a faithful Go port of the deterministic embedding
// clustering and folder binning pipeline originally implemented in Rust
// (../mapper). It turns per-fact embeddings into size-constrained semantic
// clusters and folder-local bins. Behavior, defaults and tie-breaking match the
// Rust implementation so results are reproducible and comparable.
package mapper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
)

// SizePolicy bounds cluster or bin sizes. Bounds are inclusive.
type SizePolicy struct {
	Min      int `json:"min"`
	IdealMin int `json:"ideal_min"`
	IdealMax int `json:"ideal_max"`
	Max      int `json:"max"`
}

// DefaultSizePolicy mirrors SizePolicy::default in the Rust crate.
func DefaultSizePolicy() SizePolicy {
	return SizePolicy{Min: 2, IdealMin: 5, IdealMax: 15, Max: 25}
}

// Validate checks 2 <= min <= ideal_min <= ideal_max <= max.
func (p SizePolicy) Validate() error {
	if p.Min < 2 || p.Min > p.IdealMin || p.IdealMin > p.IdealMax || p.IdealMax > p.Max {
		return fmt.Errorf("size policy requires 2 <= min <= ideal_min <= ideal_max <= max")
	}
	return nil
}

// Preferred reports whether size is in the preferred ideal range.
func (p SizePolicy) Preferred(size int) bool {
	return p.IdealMin <= size && size <= p.IdealMax
}

// Accepts reports whether size is within the hard bounds.
func (p SizePolicy) Accepts(size int) bool {
	return p.Min <= size && size <= p.Max
}

// Options controls the clustering pipeline. It mirrors the Rust Options struct:
// all fields are public, serializable, and defaulted when omitted.
type Options struct {
	Neighbors          int        `json:"neighbors"`
	MinSimilarity      float64    `json:"min_similarity"`
	SymmetricNeighbors bool       `json:"symmetric_neighbors"`
	ClusterSizes       SizePolicy `json:"cluster_sizes"`
	SplitStep          float64    `json:"split_step"`
	SplitMaxPasses     int        `json:"split_max_passes"`
	SweepFloor         *float64   `json:"sweep_floor"`
	MemberFloor        float64    `json:"member_floor"`
	TightnessFloor     float64    `json:"tightness_floor"`
	AmbiguityMargin    float64    `json:"ambiguity_margin"`
	SweepMaxPasses     int        `json:"sweep_max_passes"`
	WeakPairFloor      float64    `json:"weak_pair_floor"`
	UnclusteredTarget  float64    `json:"unclustered_target"`
	BlockSize          int        `json:"block_size"`
}

// DefaultOptions mirrors Options::default in the Rust crate.
func DefaultOptions() Options {
	floor := 0.4
	return Options{
		Neighbors:          10,
		MinSimilarity:      0.5,
		SymmetricNeighbors: true,
		ClusterSizes:       DefaultSizePolicy(),
		SplitStep:          0.05,
		SplitMaxPasses:     8,
		SweepFloor:         &floor,
		MemberFloor:        0.4,
		TightnessFloor:     0.4,
		AmbiguityMargin:    0.0,
		SweepMaxPasses:     2,
		WeakPairFloor:      0.35,
		UnclusteredTarget:  0.05,
		BlockSize:          512,
	}
}

func checkRange(name string, value, min, max float64) error {
	if !isFinite(value) || value < min || value > max {
		return fmt.Errorf("%s must be finite and in [%v, %v]", name, min, max)
	}
	return nil
}

// Validate mirrors Options::validate in the Rust crate.
func (o Options) Validate() error {
	if err := o.ClusterSizes.Validate(); err != nil {
		return err
	}
	if o.Neighbors == 0 || o.BlockSize == 0 {
		return fmt.Errorf("neighbors and block_size must be positive")
	}
	for _, check := range []struct {
		name  string
		value float64
	}{
		{"min_similarity", o.MinSimilarity},
		{"member_floor", o.MemberFloor},
		{"tightness_floor", o.TightnessFloor},
		{"weak_pair_floor", o.WeakPairFloor},
	} {
		if err := checkRange(check.name, check.value, -1.0, 1.0); err != nil {
			return err
		}
	}
	if o.SweepFloor != nil {
		if err := checkRange("sweep_floor", *o.SweepFloor, -1.0, 1.0); err != nil {
			return err
		}
	}
	if err := checkRange("ambiguity_margin", o.AmbiguityMargin, 0.0, 2.0); err != nil {
		return err
	}
	if err := checkRange("unclustered_target", o.UnclusteredTarget, 0.0, 1.0); err != nil {
		return err
	}
	if !isFinite(o.SplitStep) || o.SplitStep <= 0.0 || o.SplitStep > 2.0 {
		return fmt.Errorf("split_step must be finite and in (0, 2]")
	}
	return nil
}

// UnmarshalJSON applies defaults before decoding so omitted fields (including
// partial nested policies) keep their defaults, and rejects unknown keys,
// matching the Rust serde configuration.
func (o *Options) UnmarshalJSON(data []byte) error {
	type alias Options
	value := alias(DefaultOptions())
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&value); err != nil {
		return err
	}
	*o = Options(value)
	return nil
}

// BinOptions controls folder assignment and bin packing.
type BinOptions struct {
	Sizes                  SizePolicy `json:"sizes"`
	FolderPoolingThreshold int        `json:"folder_pooling_threshold"`
}

// DefaultBinOptions mirrors BinOptions::default in the Rust crate.
func DefaultBinOptions() BinOptions {
	return BinOptions{Sizes: DefaultSizePolicy(), FolderPoolingThreshold: 5}
}

// Validate mirrors BinOptions::validate in the Rust crate.
func (b BinOptions) Validate(options *Options) error {
	if err := b.Sizes.Validate(); err != nil {
		return err
	}
	if b.FolderPoolingThreshold == 0 {
		return fmt.Errorf("folder_pooling_threshold must be positive")
	}
	if b.Sizes.Min > options.ClusterSizes.Min || b.Sizes.Max < options.ClusterSizes.Max {
		return fmt.Errorf("bin bounds must accommodate cluster bounds to preserve whole clusters")
	}
	return nil
}

// UnmarshalJSON applies defaults before decoding, matching Rust serde.
func (b *BinOptions) UnmarshalJSON(data []byte) error {
	type alias BinOptions
	value := alias(DefaultBinOptions())
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&value); err != nil {
		return err
	}
	*b = BinOptions(value)
	return nil
}

// Configuration bundles clustering and binning options.
type Configuration struct {
	Clustering Options    `json:"clustering"`
	Binning    BinOptions `json:"binning"`
}

// DefaultConfiguration returns the default clustering and binning options.
func DefaultConfiguration() Configuration {
	return Configuration{Clustering: DefaultOptions(), Binning: DefaultBinOptions()}
}

// Validate validates clustering first, then binning against it.
func (c Configuration) Validate() error {
	if err := c.Clustering.Validate(); err != nil {
		return err
	}
	return c.Binning.Validate(&c.Clustering)
}

// UnmarshalJSON applies defaults before decoding, matching Rust serde.
func (c *Configuration) UnmarshalJSON(data []byte) error {
	type alias Configuration
	value := alias(DefaultConfiguration())
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&value); err != nil {
		return err
	}
	*c = Configuration(value)
	return nil
}

func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
