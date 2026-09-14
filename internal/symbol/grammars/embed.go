// Package grammars embeds the pre-compiled WASM grammar modules.
// To rebuild from source: make grammars (requires Go 1.21+).
//
// Generate directives pin GOTOOLCHAIN and pass -trimpath so the output is
// byte-reproducible. Committed blobs can be regenerated and diffed to
// verify they match source. Keep pin in sync with the go directive in
// go.mod and regenerate blobs when it changes.
package grammars

import _ "embed"

//go:generate sh -c "cd src/go && GOTOOLCHAIN=go1.26.2 GOOS=wasip1 GOARCH=wasm go build -trimpath -buildvcs=false -o ../../go.wasm ."
//go:embed go.wasm
var Go []byte

//go:generate sh -c "cd src/typescript && GOTOOLCHAIN=go1.26.2 GOOS=wasip1 GOARCH=wasm go build -trimpath -buildvcs=false -o ../../typescript.wasm ."
//go:embed typescript.wasm
var TypeScript []byte

//go:generate cp typescript.wasm javascript.wasm
//go:embed javascript.wasm
var JavaScript []byte

//go:generate sh -c "cd src/python && GOTOOLCHAIN=go1.26.2 GOOS=wasip1 GOARCH=wasm go build -trimpath -buildvcs=false -o ../../python.wasm ."
//go:embed python.wasm
var Python []byte
