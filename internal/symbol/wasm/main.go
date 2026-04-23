//go:build wasip1

// Package wasm provides a WASI-compatible proof-of-concept for the tld symbol
// extraction engine.  Build with:
//
//	GOOS=wasip1 GOARCH=wasm go build -o symbol.wasm ./internal/symbol/wasm/
//
// The resulting symbol.wasm can be loaded by the vscode extension via Node.js WASI
// (or by wazero in tests) to prove the interface compiles and runs correctly.
//
// Interface:
//   - stdin  JSON: {"source": "<base64 or raw>", "ext": ".go"}
//   - stdout JSON: {"symbols":[...],"refs":[...]}
//   - exit 1 error written to stderr
//
// Note: This module uses the same Result JSON types as the CLI's internal/symbol
// package.  In production the vscode extension uses web-tree-sitter (npm) with the
// same tree-sitter queries for richer parsing; this WASM module proves the build
// path and interface compatibility.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
)

// Symbol mirrors internal/symbol.Symbol for JSON compatibility.
type Symbol struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Line int    `json:"line"`
}

// Ref mirrors internal/symbol.Ref for JSON compatibility.
type Ref struct {
	Name string `json:"name"`
	Line int    `json:"line"`
}

// Result mirrors internal/symbol.Result for JSON compatibility.
type Result struct {
	Symbols []Symbol `json:"symbols"`
	Refs    []Ref    `json:"refs"`
}

// Request is the JSON payload expected on stdin.
type Request struct {
	Source string `json:"source"`
	Ext    string `json:"ext"`
}

func main() {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
		os.Exit(1)
	}

	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		fmt.Fprintf(os.Stderr, "parse request: %v\n", err)
		os.Exit(1)
	}

	var result Result

	switch req.Ext {
	case ".go":
		result, err = extractGo([]byte(req.Source))
		if err != nil {
			fmt.Fprintf(os.Stderr, "extract go: %v\n", err)
			os.Exit(1)
		}
	default:
		// For languages other than Go, return empty (extension would use web-tree-sitter)
		result = Result{Symbols: []Symbol{}, Refs: []Ref{}}
	}

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "encode result: %v\n", err)
		os.Exit(1)
	}
}

func extractGo(src []byte) (Result, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		return Result{}, fmt.Errorf("parse: %w", err)
	}

	var result Result
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Name == nil {
				return true
			}
			kind := "function"
			if node.Recv != nil && len(node.Recv.List) > 0 {
				kind = "method"
			}
			result.Symbols = append(result.Symbols, Symbol{
				Name: node.Name.Name,
				Kind: kind,
				Line: fset.Position(node.Name.Pos()).Line,
			})
		case *ast.TypeSpec:
			kind := "type"
			switch node.Type.(type) {
			case *ast.StructType:
				kind = "struct"
			case *ast.InterfaceType:
				kind = "interface"
			}
			result.Symbols = append(result.Symbols, Symbol{
				Name: node.Name.Name,
				Kind: kind,
				Line: fset.Position(node.Name.Pos()).Line,
			})
		case *ast.CallExpr:
			switch fn := node.Fun.(type) {
			case *ast.Ident:
				result.Refs = append(result.Refs, Ref{
					Name: fn.Name,
					Line: fset.Position(fn.Pos()).Line,
				})
			case *ast.SelectorExpr:
				result.Refs = append(result.Refs, Ref{
					Name: fn.Sel.Name,
					Line: fset.Position(fn.Sel.Pos()).Line,
				})
			}
		}
		return true
	})

	if result.Symbols == nil {
		result.Symbols = []Symbol{}
	}
	if result.Refs == nil {
		result.Refs = []Ref{}
	}
	return result, nil
}
