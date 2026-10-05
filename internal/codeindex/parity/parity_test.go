package parity

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/v2/internal/codeindex/config"
)

// fixtureFiles is a tool-free repository: it has source files but no project
// marker, so discovery yields no SCIP projects and the harness exercises only
// the pure-Go tree-sitter walker and the infra scanners. That keeps the
// regression baseline deterministic and independent of external binaries.
var fixtureFiles = map[string]string{
	"main.go": `package main

type Item struct {
	Name string
}

func Run() {
	helper()
}

func helper() {}
`,
	"app.py": `import os


class Box:
    def open(self):
        return helper()

    def __init__(self):
        pass


def top():
    pass
`,
	"web.ts": `import { dep } from "pkg";

export class Widget {
  render() {
    return make(1);
  }
}

function make(x: number): number {
  return x;
}
`,
	"Dockerfile": `FROM golang:1.26 AS build
WORKDIR /src
RUN go build -o /out/app ./...
CMD ["/out/app"]
`,
	"service.yaml": `apiVersion: v1
kind: Service
metadata:
  name: greeter
spec:
  selector:
    app: greeter
`,
}

func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range fixtureFiles {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", name, err)
		}
	}
	return dir
}

func TestParityFixture(t *testing.T) {
	dir := writeFixture(t)
	got, err := Run(context.Background(), dir, config.Default())
	if err != nil {
		t.Fatalf("run parity: %v", err)
	}
	if got.Sources != len(fixtureFiles) {
		t.Errorf("sources: want %d got %d", len(fixtureFiles), got.Sources)
	}
	if got.Facts == 0 {
		t.Error("expected source facts from the fixture")
	}
	if got.Chunks == 0 {
		t.Error("expected chunks from the fixture")
	}
}

// TestParityBaseline compares against an upstream codeindex baseline when one
// is provided. The file is a JSON Report captured from the original engine; set
// CODEINDEX_BASELINE to point at it. Skipped otherwise.
func TestParityBaseline(t *testing.T) {
	baseline := os.Getenv("CODEINDEX_BASELINE")
	if baseline == "" {
		t.Skip("CODEINDEX_BASELINE not set; skipping upstream parity comparison")
	}
	dir := writeFixture(t)
	got, err := Run(context.Background(), dir, config.Default())
	if err != nil {
		t.Fatalf("run parity: %v", err)
	}
	b, err := os.ReadFile(baseline)
	if err != nil {
		t.Fatal(err)
	}
	var want Report
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	for _, d := range Compare(want, got) {
		t.Errorf("%s: baseline %v got %v", d.Field, d.Want, d.Got)
	}
}
