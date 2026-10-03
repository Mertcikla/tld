package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mertcikla/tld/v2/internal/codeindex/config"
)

func TestTaskInstructions(t *testing.T) {
	for _, name := range []string{"nl2code", "code2code", "code2nl", "code2completion", "qa"} {
		task, ok := LookupTask(name)
		if !ok || task.Query == "" || task.Passage == "" {
			t.Fatalf("task %q not fully defined: %+v", name, task)
		}
	}
	if _, ok := LookupTask("missing"); ok {
		t.Fatal("unknown task resolved")
	}

	cfg := config.Default()
	cfg.Embedding.Task = "nl2code"
	c := Client{Config: cfg}
	if got := c.documentPrefix(); got != "Candidate code snippet:\n" {
		t.Fatalf("document prefix %q", got)
	}
	query, err := c.queryPrefix("")
	if err != nil || query != "Find the most relevant code snippet given the following query:\n" {
		t.Fatalf("configured query prefix %q %v", query, err)
	}
	query, err = c.queryPrefix("code2code")
	if err != nil || query != "Find an equivalent code snippet given the following code snippet:\n" {
		t.Fatalf("request query prefix %q %v", query, err)
	}
	if _, err = c.queryPrefix("nope"); err == nil {
		t.Fatal("unknown task accepted")
	}
	if err = c.CheckTask("code2code"); err != nil {
		t.Fatalf("shared passage task rejected: %v", err)
	}
	if err = c.CheckTask("qa"); err == nil {
		t.Fatal("mismatched passage task accepted")
	}
}

// TestProfileTaskEquivalence guards existing indexes: spelling nl2code as a task
// must hash to the same profile as the explicit prefixes it replaces.
func TestProfileTaskEquivalence(t *testing.T) {
	explicit := config.Default()
	explicit.Embedding.Endpoint = "http://embed"
	explicit.Embedding.Model = "jina"
	explicit.Embedding.Dimensions = 896
	explicit.Embedding.DocumentPrefix = "Candidate code snippet:\n"
	explicit.Embedding.QueryPrefix = "Find the most relevant code snippet given the following query:\n"

	tasked := explicit
	tasked.Embedding.Task = "nl2code"
	tasked.Embedding.DocumentPrefix = ""
	tasked.Embedding.QueryPrefix = ""

	if a, b := (Client{Config: explicit}).Profile(), (Client{Config: tasked}).Profile(); a != b {
		t.Fatalf("profile drift: %s != %s", a, b)
	}
}

func TestEndpointAndValidation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path %s", r.URL.Path)
		}
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		data := make([]map[string]any, len(req.Input))
		for i := range data {
			data[i] = map[string]any{"index": i, "embedding": []float32{1, 0}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()
	cfg := config.Default()
	cfg.Embedding.Endpoint = server.URL + "/v1"
	cfg.Embedding.Model = "test"
	cfg.Embedding.Dimensions = 2
	c := Client{Config: cfg}
	v, e := c.request(context.Background(), []string{"a", "b"})
	if e != nil || len(v) != 2 || calls != 1 {
		t.Fatalf("got %v %v calls=%d", v, e, calls)
	}
	cfg.Embedding.Dimensions = 3
	if _, e = (Client{Config: cfg}).request(context.Background(), []string{"a"}); e == nil {
		t.Fatal("invalid dimensions accepted")
	}
}

func TestHealthRequiresReachableServer(t *testing.T) {
	if err := (Client{Config: config.Default()}).Health(context.Background()); err == nil {
		t.Fatal("health should fail when embeddings are unconfigured")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		data := make([]map[string]any, len(req.Input))
		for i := range data {
			data[i] = map[string]any{"index": i, "embedding": []float32{1, 0}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Embedding.Endpoint = server.URL + "/v1"
	cfg.Embedding.Model = "test"
	cfg.Embedding.Dimensions = 2
	if err := (Client{Config: cfg}).Health(context.Background()); err != nil {
		t.Fatalf("health: %v", err)
	}

	cfg.Embedding.Dimensions = 3
	if err := (Client{Config: cfg}).Health(context.Background()); err == nil {
		t.Fatal("health should reject a dimension mismatch")
	}
}

func TestRequestReportsWaitingOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer server.Close()
	cfg := config.Default()
	cfg.Embedding.Endpoint = server.URL
	var states []bool
	client := Client{Config: cfg, RequestState: func(waiting bool) { states = append(states, waiting) }}
	_, err := client.request(context.Background(), []string{"code"})
	if err == nil || len(states) != 2 || !states[0] || states[1] {
		t.Fatalf("request error %v states %v", err, states)
	}
}
