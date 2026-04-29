package watch

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type ModelID struct {
	Provider   string
	Model      string
	Dimension  int
	ConfigHash string
}

type EmbeddingInput struct {
	OwnerType string
	OwnerKey  string
	Text      string
}

type Vector []float32

type Provider interface {
	ModelID() ModelID
	Embed(ctx context.Context, inputs []EmbeddingInput) ([]Vector, error)
}

type NoopProvider struct{}

func (NoopProvider) ModelID() ModelID {
	return ModelID{Provider: "none", Model: "", Dimension: 0, ConfigHash: stableHash(normalizeEmbeddingConfig(EmbeddingConfig{}))}
}

func (NoopProvider) Embed(context.Context, []EmbeddingInput) ([]Vector, error) {
	return []Vector{}, nil
}

type DeterministicProvider struct {
	Model     string
	Dimension int
}

func (p DeterministicProvider) ModelID() ModelID {
	dimension := p.Dimension
	if dimension <= 0 {
		dimension = 8
	}
	model := p.Model
	if strings.TrimSpace(model) == "" {
		model = "local-deterministic-test"
	}
	cfg := EmbeddingConfig{Provider: "local-deterministic-test", Model: model, Dimension: dimension}
	return ModelID{Provider: cfg.Provider, Model: cfg.Model, Dimension: cfg.Dimension, ConfigHash: stableHash(cfg)}
}

func (p DeterministicProvider) Embed(_ context.Context, inputs []EmbeddingInput) ([]Vector, error) {
	id := p.ModelID()
	out := make([]Vector, 0, len(inputs))
	for _, input := range inputs {
		vector := make(Vector, id.Dimension)
		seed := []byte(input.OwnerType + "\x00" + input.OwnerKey + "\x00" + input.Text)
		for i := range vector {
			sum := sha256.Sum256(append(seed, byte(i)))
			raw := binary.BigEndian.Uint32(sum[:4])
			vector[i] = float32(float64(raw)/float64(math.MaxUint32)*2 - 1)
		}
		out = append(out, vector)
	}
	return out, nil
}

func NewEmbeddingProvider(cfg EmbeddingConfig) (Provider, error) {
	cfg = normalizeEmbeddingConfig(cfg)
	switch cfg.Provider {
	case "none":
		return NoopProvider{}, nil
	case "local-deterministic-test":
		return DeterministicProvider{Model: cfg.Model, Dimension: cfg.Dimension}, nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider %q", cfg.Provider)
	}
}

func normalizeEmbeddingConfig(cfg EmbeddingConfig) EmbeddingConfig {
	cfg.Provider = strings.TrimSpace(cfg.Provider)
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Provider == "" {
		cfg.Provider = "none"
	}
	if cfg.Provider == "none" {
		cfg.Model = ""
		cfg.Dimension = 0
	}
	if cfg.Provider == "local-deterministic-test" && cfg.Dimension <= 0 {
		cfg.Dimension = 8
	}
	return cfg
}

func vectorBytes(vector Vector) []byte {
	out := make([]byte, len(vector)*4)
	for i, value := range vector {
		binary.LittleEndian.PutUint32(out[i*4:(i+1)*4], math.Float32bits(value))
	}
	return out
}

func inputHash(input EmbeddingInput) string {
	return hashString(input.Text)
}

func stableHash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
