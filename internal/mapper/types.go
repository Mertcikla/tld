package mapper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
)

// Fact describes one input file fact aligned by index with its vector.
type Fact struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	DisplayName string `json:"display_name"`
	Language    string `json:"language"`
}

// Dataset is aligned facts and embedding vectors with optional metadata.
type Dataset struct {
	Snapshot string  `json:"snapshot"`
	Profile  string  `json:"profile"`
	Root     *string `json:"root"`
	Facts    []Fact  `json:"facts"`
	Vectors  Vectors `json:"vectors"`
}

// Validate mirrors Dataset::validate in the Rust crate.
func (d Dataset) Validate() error {
	if len(d.Facts) != len(d.Vectors) {
		return errFactsVectorsMismatch
	}
	seen := make(map[string]struct{}, len(d.Facts))
	for _, fact := range d.Facts {
		if fact.ID == "" || fact.Path == "" {
			return errInvalidFacts
		}
		if _, ok := seen[fact.ID]; ok {
			return errInvalidFacts
		}
		seen[fact.ID] = struct{}{}
	}
	dimension := 0
	if len(d.Vectors) > 0 {
		dimension = len(d.Vectors[0])
	}
	for _, vector := range d.Vectors {
		if len(vector) != dimension {
			return errVectorsNotRectangular
		}
	}
	return nil
}

// Vectors preserves the Rust JSON convention: non-finite components serialize
// as null and null deserializes to NaN.
type Vectors [][]float64

// UnmarshalJSON decodes null vector components as NaN.
func (v *Vectors) UnmarshalJSON(data []byte) error {
	var rows [][]*float64
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&rows); err != nil {
		return err
	}
	out := make(Vectors, len(rows))
	for i, row := range rows {
		values := make([]float64, len(row))
		for j, value := range row {
			if value == nil {
				values[j] = math.NaN()
			} else {
				values[j] = *value
			}
		}
		out[i] = values
	}
	*v = out
	return nil
}

// MarshalJSON encodes non-finite components as null.
func (v Vectors) MarshalJSON() ([]byte, error) {
	rows := make([][]*float64, len(v))
	for i, row := range v {
		values := make([]*float64, len(row))
		for j, value := range row {
			if math.IsInf(value, 0) || math.IsNaN(value) {
				values[j] = nil
				continue
			}
			values[j] = &row[j]
		}
		rows[i] = values
	}
	return json.Marshal(rows)
}

// DecodeJSON decodes data into dst (pre-filled with defaults) and rejects
// unknown fields, matching Rust's deny_unknown_fields.
func DecodeJSON(data []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode mapper configuration: %w", err)
	}
	return nil
}
