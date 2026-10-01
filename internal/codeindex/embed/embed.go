package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

// Store is the persistence surface the embedding pipeline needs. It is
// implemented by the CodeIndexStore storage layer.
type Store interface {
	Chunks(ctx context.Context, snapshotID string) ([]*pb.Chunk, error)
	GetCachedEmbedding(ctx context.Context, key string) ([]float32, error)
	CacheEmbedding(ctx context.Context, key string, vector []float32) error
	SaveEmbedding(ctx context.Context, embedding *pb.Embedding) error
	SaveFactEmbedding(ctx context.Context, embedding *pb.Embedding) error
	UpdateSnapshot(ctx context.Context, snapshot *pb.Snapshot) error
}

type Client struct {
	Config config.Config
	HTTP   *http.Client
	Store  Store
}

// Profile names the embedding space produced by this configuration. It covers
// the task-specific passage instruction so different indexing tasks never share
// vectors. The query instruction is request-scoped and deliberately excluded.
func (c Client) Profile() string {
	query, _ := c.queryPrefix("")
	return graph.ID(c.Config.Embedding.Endpoint, c.Config.Embedding.Model, fmt.Sprint(c.Config.Embedding.Dimensions), fmt.Sprint(c.Config.Embedding.MaxInputChars), c.documentPrefix(), query)
}
func (c Client) Enabled() bool { return c.Config.Embedding.Endpoint != "" }

// documentPrefix is the passage instruction prepended to indexed chunks. A
// configured task takes precedence over the raw document_prefix.
func (c Client) documentPrefix() string {
	if t, ok := LookupTask(c.Config.Embedding.Task); ok {
		return t.Passage
	}
	return c.Config.Embedding.DocumentPrefix
}

// queryPrefix is the instruction prepended to search text. An explicit request
// task wins, then the configured task, then the raw query_prefix.
func (c Client) queryPrefix(task string) (string, error) {
	if task != "" {
		t, err := ResolveTask(task)
		if err != nil {
			return "", err
		}
		return t.Query, nil
	}
	if t, ok := LookupTask(c.Config.Embedding.Task); ok {
		return t.Query, nil
	}
	return c.Config.Embedding.QueryPrefix, nil
}

// CheckTask reports whether a request task can run against this index. Tasks
// share a query space only when their passage instruction matches what the
// snapshot was embedded with.
func (c Client) CheckTask(task string) error {
	t, err := ResolveTask(task)
	if err != nil {
		return err
	}
	if doc := c.documentPrefix(); t.Passage != doc {
		return fmt.Errorf("task %q indexes passages as %q but this index uses %q; reindex with embedding.task=%s", task, t.Passage, doc, task)
	}
	return nil
}
func (c Client) fit(text string) string {
	if c.Config.Embedding.MaxInputChars <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) > c.Config.Embedding.MaxInputChars {
		return string(runes[:c.Config.Embedding.MaxInputChars])
	}
	return text
}
func Text(chunk *pb.Chunk) string {
	return strings.TrimSpace(chunk.Context + "\n" + chunk.Text)
}
func (c Client) Embed(ctx context.Context, snap *pb.Snapshot) error {
	if !c.Enabled() {
		return nil
	}
	chunks, e := c.Store.Chunks(ctx, snap.Id)
	if e != nil {
		return e
	}
	profile := c.Profile()
	documentPrefix := c.documentPrefix()
	pending := make([]*pb.Chunk, 0)
	texts := make([]string, 0)
	factSum := map[string][]float32{}
	factCount := map[string]int{}
	accumulate := func(factID string, v []float32) {
		if factID == "" || len(v) == 0 {
			return
		}
		sum := factSum[factID]
		if sum == nil {
			sum = make([]float32, len(v))
			factSum[factID] = sum
		}
		if len(sum) != len(v) {
			return
		}
		for i := range v {
			sum[i] += v[i]
		}
		factCount[factID]++
	}
	for _, chunk := range chunks {
		t := Text(chunk)
		if t == "" {
			continue
		}
		t = c.fit(documentPrefix + t)
		hash := graph.Hash([]byte(t))
		key := graph.ID(profile, hash)
		cached, e := c.Store.GetCachedEmbedding(ctx, key)
		if e != nil {
			return e
		}
		if len(cached) > 0 {
			if e := c.Store.SaveEmbedding(ctx, &pb.Embedding{ChunkId: chunk.Id, FactId: chunk.FactId, SnapshotId: snap.Id, Profile: profile, Model: c.Config.Embedding.Model, Dimensions: uint32(c.Config.Embedding.Dimensions), InputHash: hash, Vector: cached}); e != nil {
				return e
			}
			accumulate(chunk.FactId, cached)
			continue
		}
		pending = append(pending, chunk)
		texts = append(texts, t)
	}
	batch := c.Config.Embedding.BatchSize
	for start := 0; start < len(pending); start += batch {
		end := start + batch
		if end > len(pending) {
			end = len(pending)
		}
		vectors, e := c.request(ctx, texts[start:end])
		if e != nil {
			return e
		}
		for j, v := range vectors {
			chunk := pending[start+j]
			hash := graph.Hash([]byte(texts[start+j]))
			if e := c.Store.CacheEmbedding(ctx, graph.ID(profile, hash), v); e != nil {
				return e
			}
			if e := c.Store.SaveEmbedding(ctx, &pb.Embedding{ChunkId: chunk.Id, FactId: chunk.FactId, SnapshotId: snap.Id, Profile: profile, Model: c.Config.Embedding.Model, Dimensions: uint32(c.Config.Embedding.Dimensions), InputHash: hash, Vector: v}); e != nil {
				return e
			}
			accumulate(chunk.FactId, v)
		}
	}
	for factID, sum := range factSum {
		count := factCount[factID]
		if count == 0 {
			continue
		}
		mean := make([]float32, len(sum))
		for i := range sum {
			mean[i] = sum[i] / float32(count)
		}
		hash := graph.ID(profile, factID, fmt.Sprint(count))
		if e := c.Store.SaveFactEmbedding(ctx, &pb.Embedding{FactId: factID, SnapshotId: snap.Id, Profile: profile, Model: c.Config.Embedding.Model, Dimensions: uint32(c.Config.Embedding.Dimensions), InputHash: hash, Vector: mean}); e != nil {
			return e
		}
	}
	snap.EmbeddingStatus = "complete"
	// Warnings are currently emitted only for embedding failures. A successful
	// retry resolves those warnings along with the incomplete status.
	snap.Warnings = nil
	return c.Store.UpdateSnapshot(ctx, snap)
}
func (c Client) Query(ctx context.Context, text string) ([]float32, error) {
	return c.QueryTask(ctx, text, "")
}

// QueryTask embeds search text with the instruction for the requested Jina
// task. An empty task uses the configured default. When a task is given its
// passage instruction must match the index profile, so a query can never be
// compared against incompatible passage vectors.
func (c Client) QueryTask(ctx context.Context, text, task string) ([]float32, error) {
	if !c.Enabled() {
		return nil, fmt.Errorf("embeddings not configured")
	}
	if task != "" {
		if e := c.CheckTask(task); e != nil {
			return nil, e
		}
	}
	instruction, e := c.queryPrefix(task)
	if e != nil {
		return nil, e
	}
	v, e := c.request(ctx, []string{instruction + text})
	if e != nil {
		return nil, e
	}
	return v[0], nil
}
func (c Client) request(ctx context.Context, input []string) ([][]float32, error) {
	for i, text := range input {
		input[i] = c.fit(text)
	}
	endpoint := c.Config.Embedding.Endpoint
	if !strings.HasSuffix(endpoint, "/embeddings") {
		endpoint += "/embeddings"
	}
	body, _ := json.Marshal(map[string]any{"model": c.Config.Embedding.Model, "input": input, "dimensions": c.Config.Embedding.Dimensions})
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 45 * time.Second}
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		req, e := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
		if e != nil {
			return nil, e
		}
		req.Header.Set("Content-Type", "application/json")
		if c.Config.Embedding.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.Config.Embedding.APIKey)
		}
		resp, e := httpClient.Do(req)
		if e != nil {
			last = e
		} else {
			data, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
			_ = resp.Body.Close()
			if readErr != nil {
				last = readErr
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				var payload struct {
					Data []struct {
						Index     int       `json:"index"`
						Embedding []float32 `json:"embedding"`
					} `json:"data"`
				}
				if e := json.Unmarshal(data, &payload); e != nil {
					return nil, e
				}
				if len(payload.Data) != len(input) {
					return nil, fmt.Errorf("embedding count mismatch")
				}
				out := make([][]float32, len(input))
				for _, item := range payload.Data {
					if item.Index < 0 || item.Index >= len(out) || out[item.Index] != nil {
						return nil, fmt.Errorf("invalid embedding index")
					}
					if len(item.Embedding) != c.Config.Embedding.Dimensions {
						return nil, fmt.Errorf("embedding dimensions mismatch")
					}
					for _, n := range item.Embedding {
						if math.IsNaN(float64(n)) || math.IsInf(float64(n), 0) {
							return nil, fmt.Errorf("non-finite embedding")
						}
					}
					out[item.Index] = item.Embedding
				}
				return out, nil
			} else {
				last = fmt.Errorf("embedding HTTP %d: %s", resp.StatusCode, string(data))
				if resp.StatusCode < 500 && resp.StatusCode != 429 {
					return nil, last
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 300 * time.Millisecond):
		}
	}
	return nil, last
}
