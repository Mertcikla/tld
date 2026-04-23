// Package grammars embeds the pre-compiled WASM grammar modules.
// To rebuild from source: make grammars (requires Go 1.21+, no external toolchain needed).
package grammars

import _ "embed"

//go:embed go.wasm
var Go []byte

//go:embed typescript.wasm
var TypeScript []byte

//go:embed javascript.wasm
var JavaScript []byte

//go:embed python.wasm
var Python []byte
