/**
 * test.mjs Node.js WASI proof-of-concept test for symbol.wasm
 *
 * Run with:
 *   node --experimental-wasi-unstable-preview1 internal/symbol/wasm/test.mjs
 *
 * Expected output: JSON with symbols for the embedded Go snippet.
 */

import { WASI } from "wasi";
import { readFileSync } from "fs";
import { fileURLToPath } from "url";
import { dirname, join } from "path";
import { Readable, Writable } from "stream";

const __filename = fileURLToPath(import.meta.url);
const __dir = dirname(__filename);

const wasmPath = join(__dir, "symbol.wasm");
const wasmBytes = readFileSync(wasmPath);

const goSource = `
package main

type Server struct{}
func NewServer() *Server { return &Server{} }
func (s *Server) Start() {}
`;

const request = JSON.stringify({ source: goSource, ext: ".go" });

// Capture stdout into a buffer
let output = "";
const stdout = new Writable({
  write(chunk, _enc, cb) {
    output += chunk.toString();
    cb();
  },
});

// Provide stdin from the request
const stdin = Readable.from([Buffer.from(request)]);

const wasi = new WASI({
  version: "preview1",
  stdin,
  stdout,
  stderr: process.stderr,
  args: ["symbol"],
});

const { instance } = await WebAssembly.instantiate(wasmBytes, {
  wasi_snapshot_preview1: wasi.wasiImport,
});

wasi.start(instance);

const result = JSON.parse(output);

// Assertions
const symbolNames = result.symbols.map((s) => s.name);
const wantSymbols = ["Server", "NewServer", "Start"];

let passed = true;
for (const name of wantSymbols) {
  if (!symbolNames.includes(name)) {
    console.error(`FAIL: expected symbol "${name}", got:`, symbolNames);
    passed = false;
  }
}

if (passed) {
  console.log("PASS symbol.wasm POC test");
  console.log("Symbols:", symbolNames);
} else {
  process.exit(1);
}
