package watch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const (
	DefaultEmbeddingProvider        = "openai"
	DefaultOpenAIEndpoint           = "http://127.0.0.1:8000/v1/embeddings"
	DefaultOpenAIModel              = "embeddinggemma-300m-4bit"
	DefaultOpenAIAPIKey             = "tldcli"
	DefaultOllamaEndpoint           = "http://localhost:11434"
	DefaultOllamaModel              = "jina/jina-embeddings-v2-base-en"
	DefaultEmbeddingHealthThreshold = 0.70
	RenameEmbeddingThreshold        = 0.78
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

type HealthResult struct {
	Dimension  int
	Similarity float64
}

type HealthCheckingProvider interface {
	HealthCheck(ctx context.Context) (HealthResult, error)
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

type OllamaProvider struct {
	Endpoint        string
	Model           string
	Dimension       int
	HealthThreshold float64
	Client          *http.Client
}

func (p *OllamaProvider) ModelID() ModelID {
	cfg := normalizeEmbeddingConfig(EmbeddingConfig{
		Provider:        "ollama",
		Endpoint:        p.Endpoint,
		Model:           p.Model,
		Dimension:       p.Dimension,
		HealthThreshold: p.HealthThreshold,
	})
	return ModelID{Provider: cfg.Provider, Model: cfg.Model, Dimension: cfg.Dimension, ConfigHash: stableHash(cfg)}
}

func (p *OllamaProvider) Embed(ctx context.Context, inputs []EmbeddingInput) ([]Vector, error) {
	if len(inputs) == 0 {
		return []Vector{}, nil
	}
	texts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		texts = append(texts, input.Text)
	}
	vectors, err := p.embedTexts(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(inputs) {
		return nil, fmt.Errorf("ollama returned %d embeddings for %d inputs", len(vectors), len(inputs))
	}
	if len(vectors) > 0 && p.Dimension <= 0 {
		p.Dimension = len(vectors[0])
	}
	return vectors, nil
}

func (p *OllamaProvider) HealthCheck(ctx context.Context) (HealthResult, error) {
	texts := []string{
		"Why is the sky blue?",
		"What causes the sky to look blue during the day?",
	}
	vectors, err := p.embedTexts(ctx, texts)
	if err != nil {
		return HealthResult{}, err
	}
	if len(vectors) != 2 || len(vectors[0]) == 0 || len(vectors[1]) == 0 {
		return HealthResult{}, fmt.Errorf("ollama healthcheck returned empty embeddings")
	}
	if len(vectors[0]) != len(vectors[1]) {
		return HealthResult{}, fmt.Errorf("ollama healthcheck returned mismatched dimensions %d and %d", len(vectors[0]), len(vectors[1]))
	}
	sim := CosineSimilarity(vectors[0], vectors[1])
	threshold := p.HealthThreshold
	if threshold <= 0 {
		threshold = DefaultEmbeddingHealthThreshold
	}
	if sim < threshold {
		return HealthResult{}, fmt.Errorf("ollama healthcheck similarity %.3f is below threshold %.3f", sim, threshold)
	}
	p.Dimension = len(vectors[0])
	return HealthResult{Dimension: len(vectors[0]), Similarity: sim}, nil
}

func (p *OllamaProvider) embedTexts(ctx context.Context, texts []string) ([]Vector, error) {
	endpoint := strings.TrimRight(p.Endpoint, "/")
	if endpoint == "" {
		endpoint = DefaultOllamaEndpoint
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	body, _ := json.Marshal(map[string]any{
		"model": p.Model,
		"input": texts,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama embed request failed: %s", resp.Status)
	}
	var parsed struct {
		Embedding  []float32   `json:"embedding"`
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode ollama embed response: %w", err)
	}
	if len(parsed.Embeddings) > 0 {
		return vectorsFromFloatSlices(parsed.Embeddings), nil
	}
	if len(parsed.Embedding) > 0 {
		return []Vector{Vector(parsed.Embedding)}, nil
	}
	return nil, fmt.Errorf("ollama embed response did not include embeddings")
}

type OpenAIProvider struct {
	Endpoint        string
	Model           string
	Dimension       int
	HealthThreshold float64
	Client          *http.Client
}

func (p *OpenAIProvider) ModelID() ModelID {
	cfg := normalizeEmbeddingConfig(EmbeddingConfig{
		Provider:        "openai",
		Endpoint:        p.Endpoint,
		Model:           p.Model,
		Dimension:       p.Dimension,
		HealthThreshold: p.HealthThreshold,
	})
	return ModelID{Provider: cfg.Provider, Model: cfg.Model, Dimension: cfg.Dimension, ConfigHash: stableHash(cfg)}
}

func (p *OpenAIProvider) Embed(ctx context.Context, inputs []EmbeddingInput) ([]Vector, error) {
	if len(inputs) == 0 {
		return []Vector{}, nil
	}
	texts := make([]string, 0, len(inputs))
	for _, input := range inputs {
		texts = append(texts, input.Text)
	}
	vectors, err := p.embedTexts(ctx, texts)
	if err != nil {
		return nil, err
	}
	if len(vectors) != len(inputs) {
		return nil, fmt.Errorf("openai returned %d embeddings for %d inputs", len(vectors), len(inputs))
	}
	if len(vectors) > 0 && p.Dimension <= 0 {
		p.Dimension = len(vectors[0])
	}
	return vectors, nil
}

func (p *OpenAIProvider) HealthCheck(ctx context.Context) (HealthResult, error) {
	texts := []string{
		"Why is the sky blue?",
		"What causes the sky to look blue during the day?",
	}
	vectors, err := p.embedTexts(ctx, texts)
	if err != nil {
		return HealthResult{}, err
	}
	if len(vectors) != 2 || len(vectors[0]) == 0 || len(vectors[1]) == 0 {
		return HealthResult{}, fmt.Errorf("openai healthcheck returned empty embeddings")
	}
	if len(vectors[0]) != len(vectors[1]) {
		return HealthResult{}, fmt.Errorf("openai healthcheck returned mismatched dimensions %d and %d", len(vectors[0]), len(vectors[1]))
	}
	sim := CosineSimilarity(vectors[0], vectors[1])
	threshold := p.HealthThreshold
	if threshold <= 0 {
		threshold = DefaultEmbeddingHealthThreshold
	}
	if sim < threshold {
		return HealthResult{}, fmt.Errorf("openai healthcheck similarity %.3f is below threshold %.3f", sim, threshold)
	}
	p.Dimension = len(vectors[0])
	return HealthResult{Dimension: len(vectors[0]), Similarity: sim}, nil
}

func (p *OpenAIProvider) embedTexts(ctx context.Context, texts []string) ([]Vector, error) {
	opts := []option.RequestOption{
		option.WithBaseURL(openAIBaseURL(p.Endpoint)),
		option.WithAPIKey(DefaultOpenAIAPIKey),
		option.WithRequestTimeout(30 * time.Second),
	}
	if p.Client != nil {
		opts = append(opts, option.WithHTTPClient(p.Client))
	}
	client := openai.NewClient(opts...)
	resp, err := client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Model: openai.EmbeddingModel(p.Model),
		Input: openai.EmbeddingNewParamsInputUnion{OfArrayOfStrings: texts},
	})
	if err != nil {
		return nil, fmt.Errorf("openai embeddings request: %w", err)
	}
	vectors := make([]Vector, 0, len(resp.Data))
	for _, item := range resp.Data {
		vector := make(Vector, len(item.Embedding))
		for i, value := range item.Embedding {
			vector[i] = float32(value)
		}
		vectors = append(vectors, vector)
	}
	return vectors, nil
}

func openAIBaseURL(endpoint string) string {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		endpoint = DefaultOpenAIEndpoint
	}
	if strings.HasSuffix(endpoint, "/embeddings") {
		return strings.TrimSuffix(endpoint, "/embeddings")
	}
	return endpoint
}

func NewEmbeddingProvider(cfg EmbeddingConfig) (Provider, error) {
	cfg = normalizeEmbeddingConfig(cfg)
	switch cfg.Provider {
	case "none":
		return NoopProvider{}, nil
	case "openai":
		return &OpenAIProvider{Endpoint: cfg.Endpoint, Model: cfg.Model, Dimension: cfg.Dimension, HealthThreshold: cfg.HealthThreshold}, nil
	case "ollama":
		return &OllamaProvider{Endpoint: cfg.Endpoint, Model: cfg.Model, Dimension: cfg.Dimension, HealthThreshold: cfg.HealthThreshold}, nil
	case "local-deterministic-test":
		return DeterministicProvider{Model: cfg.Model, Dimension: cfg.Dimension}, nil
	default:
		return nil, fmt.Errorf("unsupported embedding provider %q", cfg.Provider)
	}
}

func normalizeEmbeddingConfig(cfg EmbeddingConfig) EmbeddingConfig {
	cfg.Provider = strings.TrimSpace(cfg.Provider)
	cfg.Endpoint = strings.TrimRight(strings.TrimSpace(cfg.Endpoint), "/")
	cfg.Model = strings.TrimSpace(cfg.Model)
	if cfg.Provider == "" {
		cfg.Provider = DefaultEmbeddingProvider
	}
	if cfg.Provider == "none" {
		cfg.Endpoint = ""
		cfg.Model = ""
		cfg.Dimension = 0
		cfg.HealthThreshold = 0
	}
	if cfg.Provider == "openai" {
		if cfg.Endpoint == "" {
			cfg.Endpoint = DefaultOpenAIEndpoint
		}
		if cfg.Model == "" {
			cfg.Model = DefaultOpenAIModel
		}
		if cfg.HealthThreshold <= 0 {
			cfg.HealthThreshold = DefaultEmbeddingHealthThreshold
		}
	}
	if cfg.Provider == "ollama" {
		if cfg.Endpoint == "" {
			cfg.Endpoint = DefaultOllamaEndpoint
		}
		if cfg.Model == "" {
			cfg.Model = DefaultOllamaModel
		}
		if cfg.HealthThreshold <= 0 {
			cfg.HealthThreshold = DefaultEmbeddingHealthThreshold
		}
	}
	if cfg.Provider == "local-deterministic-test" && cfg.Dimension <= 0 {
		cfg.Dimension = 8
	}
	return cfg
}

func NormalizeEmbeddingConfig(cfg EmbeddingConfig) EmbeddingConfig {
	return normalizeEmbeddingConfig(cfg)
}

func CheckEmbeddingHealth(ctx context.Context, cfg EmbeddingConfig) (EmbeddingConfig, HealthResult, error) {
	cfg = normalizeEmbeddingConfig(cfg)
	provider, err := NewEmbeddingProvider(cfg)
	if err != nil {
		return cfg, HealthResult{}, err
	}
	checker, ok := provider.(HealthCheckingProvider)
	if !ok {
		return cfg, HealthResult{Dimension: provider.ModelID().Dimension, Similarity: 1}, nil
	}
	result, err := checker.HealthCheck(ctx)
	if err != nil {
		return cfg, HealthResult{}, err
	}
	if result.Dimension > 0 {
		cfg.Dimension = result.Dimension
	}
	return cfg, result, nil
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

func vectorsFromFloatSlices(values [][]float32) []Vector {
	out := make([]Vector, 0, len(values))
	for _, value := range values {
		out = append(out, Vector(value))
	}
	return out
}

func CosineSimilarity(left, right Vector) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		l := float64(left[i])
		r := float64(right[i])
		dot += l * r
		leftNorm += l * l
		rightNorm += r * r
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm))
}
