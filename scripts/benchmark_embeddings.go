//go:build ignore

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
}

func main() {
	endpoint := flag.String("endpoint", envDefault("TLD_EMBEDDING_ENDPOINT", "http://127.0.0.1:8000/v1/embeddings"), "OpenAI-compatible embeddings endpoint")
	model := flag.String("model", envDefault("TLD_EMBEDDING_MODEL", "embeddinggemma-300m-4bit"), "embedding model")
	repeats := flag.Int("repeats", intEnvDefault("TLD_EMBEDDING_REPEATS", 3), "measured requests per batch size")
	warmup := flag.Int("warmup", intEnvDefault("TLD_EMBEDDING_WARMUP", 1), "warmup requests per batch size")
	flag.Parse()

	client := &http.Client{Timeout: 5 * time.Minute}
	ctx := context.Background()
	fmt.Printf("endpoint=%s\nmodel=%s\nrepeats=%d warmup=%d\n\n", *endpoint, *model, *repeats, *warmup)
	fmt.Printf("%6s %10s %10s %10s %10s %10s\n", "batch", "avg_ms", "min_ms", "max_ms", "items/s", "dim")

	for batch := 1; batch <= 512; batch *= 2 {
		for i := 0; i < *warmup; i++ {
			if _, _, err := runOnce(ctx, client, *endpoint, *model, batch); err != nil {
				fail(batch, err)
			}
		}

		var total, min, max time.Duration
		dim := 0
		for i := 0; i < *repeats; i++ {
			elapsed, nextDim, err := runOnce(ctx, client, *endpoint, *model, batch)
			if err != nil {
				fail(batch, err)
			}
			if i == 0 || elapsed < min {
				min = elapsed
			}
			if elapsed > max {
				max = elapsed
			}
			total += elapsed
			dim = nextDim
		}
		avg := total / time.Duration(*repeats)
		itemsPerSecond := float64(batch) / avg.Seconds()
		fmt.Printf("%6d %10.1f %10.1f %10.1f %10.1f %10d\n", batch, ms(avg), ms(min), ms(max), itemsPerSecond, dim)
	}
}

func runOnce(ctx context.Context, client *http.Client, endpoint, model string, batch int) (time.Duration, int, error) {
	body, err := json.Marshal(embeddingRequest{Model: model, Input: inputs(batch)})
	if err != nil {
		return 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tldcli")

	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, 0, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	var parsed embeddingResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return 0, 0, err
	}
	if len(parsed.Data) != batch {
		return 0, 0, fmt.Errorf("expected %d embeddings, got %d", batch, len(parsed.Data))
	}
	dim := 0
	if len(parsed.Data) > 0 {
		dim = len(parsed.Data[0].Embedding)
	}
	return elapsed, dim, nil
}

func inputs(batch int) []string {
	out := make([]string, batch)
	base := "package main\n\nfunc FetchUserProfile(ctx context.Context, userID string) (*User, error) {\n\treturn repository.LoadUser(ctx, userID)\n}\n"
	for i := range out {
		out[i] = fmt.Sprintf("%s\n// benchmark sample %d: code symbol embedding context with repository, service, handler, and tests.code symbol embedding context with repository, service, handler, and tests.code symbol embedding context with repository, service, handler, and tests.code symbol embedding context with repository, service, handler, and tests.", base, i+1)
	}
	return out
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000
}

func fail(batch int, err error) {
	fmt.Fprintf(os.Stderr, "batch %d failed: %v\n", batch, err)
	os.Exit(1)
}

func envDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func intEnvDefault(name string, fallback int) int {
	if value := os.Getenv(name); value != "" {
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
