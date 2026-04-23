package symbol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// wasmResult is the JSON shape written to stdout by each grammar WASM module.
type wasmResult struct {
	Symbols []struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		Line    int    `json:"line"`
		EndLine int    `json:"end_line"`
		Parent  string `json:"parent"`
	} `json:"symbols"`
	Refs []struct {
		Name string `json:"name"`
		Line int    `json:"line"`
	} `json:"refs"`
}

// runGrammar executes the given WASM grammar module with src as stdin and
// returns the decoded symbol/ref result.
func runGrammar(ctx context.Context, wasmBytes []byte, src []byte) (*Result, error) {
	rt := wazero.NewRuntime(ctx)
	defer func() {
		_ = rt.Close(ctx)
	}()

	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	var stdout, stderr bytes.Buffer
	cfg := wazero.NewModuleConfig().
		WithStdin(bytes.NewReader(src)).
		WithStdout(&stdout).
		WithStderr(&stderr).
		WithArgs("grammar")

	compiled, err := rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compile wasm: %w", err)
	}

	if _, err := rt.InstantiateModule(ctx, compiled, cfg); err != nil {
		detail := stderr.String()
		if detail == "" {
			detail = err.Error()
		}
		return nil, fmt.Errorf("run wasm: %s", detail)
	}

	var raw wasmResult
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, fmt.Errorf("decode wasm output: %w (output: %q)", err, stdout.String())
	}

	result := &Result{
		Symbols: make([]Symbol, 0, len(raw.Symbols)),
		Refs:    make([]Ref, 0, len(raw.Refs)),
	}
	for _, s := range raw.Symbols {
		result.Symbols = append(result.Symbols, Symbol{
			Name:    s.Name,
			Kind:    s.Kind,
			Line:    s.Line,
			EndLine: s.EndLine,
			Parent:  s.Parent,
		})
	}
	for _, r := range raw.Refs {
		result.Refs = append(result.Refs, Ref{Name: r.Name, Line: r.Line})
	}
	return result, nil
}
