// WASM grammar module for Go source files.
// Compiled with: GOOS=wasip1 GOARCH=wasm go build -o ../../go.wasm .
//
// Protocol:
//
//	stdin  Go source code
//	stdout JSON: {"symbols":[...],"refs":[...]}
//	exit 1 parse error written to stderr
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

type Symbol struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Line    int    `json:"line"`
	EndLine int    `json:"end_line"`
	Parent  string `json:"parent,omitempty"`
}

type Ref struct {
	Name string `json:"name"`
	Line int    `json:"line"`
}

type Reffer struct {
	Name string `json:"name"`
	Line int    `json:"line"`
}
type Result struct {
	Symbols []Symbol `json:"symbols"`
	Refs    []Ref    `json:"refs"`
}

func main() {
	src, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
		os.Exit(1)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}

	var result Result

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Name == nil {
				return true
			}
			kind := "function"
			parent := ""
			if node.Recv != nil && len(node.Recv.List) > 0 {
				kind = "method"
				// Try to find receiver type name
				switch t := node.Recv.List[0].Type.(type) {
				case *ast.Ident:
					parent = t.Name
				case *ast.StarExpr:
					if id, ok := t.X.(*ast.Ident); ok {
						parent = id.Name
					}
				}
			}
			pos := fset.Position(node.Name.Pos())
			end := fset.Position(node.End())
			result.Symbols = append(result.Symbols, Symbol{
				Name:    node.Name.Name,
				Kind:    kind,
				Line:    pos.Line,
				EndLine: end.Line,
				Parent:  parent,
			})
		case *ast.TypeSpec:
			pos := fset.Position(node.Name.Pos())
			end := fset.Position(node.End())
			kind := "type"
			switch node.Type.(type) {
			case *ast.StructType:
				kind = "struct"
			case *ast.InterfaceType:
				kind = "interface"
			}
			result.Symbols = append(result.Symbols, Symbol{
				Name:    node.Name.Name,
				Kind:    kind,
				Line:    pos.Line,
				EndLine: end.Line,
			})
		case *ast.CallExpr:
			// Extract direct function call references
			switch fn := node.Fun.(type) {
			case *ast.Ident:
				pos := fset.Position(fn.Pos())
				result.Refs = append(result.Refs, Ref{
					Name: fn.Name,
					Line: pos.Line,
				})
			case *ast.SelectorExpr:
				pos := fset.Position(fn.Sel.Pos())
				result.Refs = append(result.Refs, Ref{
					Name: fn.Sel.Name,
					Line: pos.Line,
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

	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}
