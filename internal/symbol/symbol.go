// Package symbol extracts symbols and call-site references from source files.
// It uses per-language WASM grammar modules embedded in the binary and executed
// via the wazero WASM/WASI runtime no CGO required.
package symbol

// Symbol is a named declaration found in a source file.
type Symbol struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"` // "function" | "method" | "class" | "struct" | "interface" | "type"
	FilePath string `json:"file_path,omitempty"`
	Line     int    `json:"line"`
	EndLine  int    `json:"end_line,omitempty"`
	Parent   string `json:"parent,omitempty"`
}

// Ref is a call-site reference to a named symbol found within a source file.
type Ref struct {
	Name     string `json:"name"`
	FilePath string `json:"file_path,omitempty"`
	Line     int    `json:"line"`
}

// Result holds the output of extracting symbols from one file.
type Result struct {
	Symbols []Symbol `json:"symbols"`
	Refs    []Ref    `json:"refs"`
}
