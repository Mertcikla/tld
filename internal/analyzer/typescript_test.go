package analyzer

import (
	"context"
	"testing"
)

// TestTSParser_TypeScript tests TypeScript-specific declarations and references.
func TestTSParser_TypeScript(t *testing.T) {
	parser := &tsParser{}
	source := `
import React from "react";
import { useState, useEffect } from "react";
import type { FC } from "react";
import * as utils from "./utils";

// UserService manages user data.
export class UserService {
  constructor(private repo: UserRepository) {}

  async getUser(id: string): Promise<User> {
    return this.repo.findById(id);
  }
}

export interface UserRepository {
  findById(id: string): Promise<User>;
}

export type UserId = string;

export enum Role {
  Admin = "admin",
  User  = "user",
}

export function createUser(name: string): User {
  return { name };
}

const helperFn = (x: number) => x * 2;
const anotherHelper = function(x: number) { return x; };

export default function App() {
  const [count, setCount] = useState(0);
  useEffect(() => {
    console.log(count);
  }, [count]);
  return helperFn(count);
}
`
	result, err := parser.ParseFile(context.Background(), "test.ts", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	expectedSymbols := []Symbol{
		{Name: "UserService", Kind: "class"},
		{Name: "constructor", Kind: "constructor", Parent: "UserService"},
		{Name: "getUser", Kind: "method", Parent: "UserService"},
		{Name: "UserRepository", Kind: "interface"},
		{Name: "UserId", Kind: "type"},
		{Name: "Role", Kind: "enum"},
		{Name: "createUser", Kind: "function"},
		{Name: "helperFn", Kind: "function"},
		{Name: "anotherHelper", Kind: "function"},
		{Name: "App", Kind: "function"},
	}
	for _, expected := range expectedSymbols {
		found := false
		for _, actual := range result.Symbols {
			if actual.Name == expected.Name && actual.Kind == expected.Kind {
				if expected.Parent != "" && actual.Parent != expected.Parent {
					continue
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("symbol not found: %+v (got %v)", expected, result.Symbols)
		}
	}

	expectedRefs := []Ref{
		{Name: "React", Kind: "import", TargetPath: "react"},
		{Name: "useState", Kind: "import", TargetPath: "react"},
		{Name: "useEffect", Kind: "import", TargetPath: "react"},
		{Name: "utils", Kind: "import", TargetPath: "./utils"},
		{Name: "findById", Kind: "call"},
		{Name: "useState", Kind: "call"},
		{Name: "useEffect", Kind: "call"},
		{Name: "helperFn", Kind: "call"},
	}
	for _, expected := range expectedRefs {
		found := false
		for _, actual := range result.Refs {
			if actual.Name == expected.Name && actual.Kind == expected.Kind {
				if expected.TargetPath != "" && actual.TargetPath != expected.TargetPath {
					continue
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ref not found: %+v", expected)
		}
	}
}

// TestTSParser_TSX tests TSX-specific parsing (.tsx file extension selects the TSX grammar).
func TestTSParser_TSX(t *testing.T) {
	parser := &tsParser{}
	source := `
import React from "react";

interface ButtonProps {
  label: string;
  onClick: () => void;
}

const Button = ({ label, onClick }: ButtonProps) => (
  <button onClick={onClick}>{label}</button>
);

export default function App() {
  return <Button label="Click me" onClick={() => console.log("clicked")} />;
}
`
	result, err := parser.ParseFile(context.Background(), "test.tsx", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile tsx: %v", err)
	}

	expectedSymbols := []Symbol{
		{Name: "ButtonProps", Kind: "interface"},
		{Name: "App", Kind: "function"},
	}
	for _, expected := range expectedSymbols {
		found := false
		for _, actual := range result.Symbols {
			if actual.Name == expected.Name && actual.Kind == expected.Kind {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("tsx symbol not found: %+v", expected)
		}
	}
}

// TestJSParser_JavaScript tests JavaScript declarations and references.
func TestJSParser_JavaScript(t *testing.T) {
	parser := &jsParser{}
	source := `
import { readFile } from "fs";
import path from "path";

class FileReader {
  constructor(basePath) {
    this.basePath = basePath;
  }

  read(name) {
    const full = path.join(this.basePath, name);
    return readFile(full);
  }
}

function createReader(base) {
  return new FileReader(base);
}

const processFile = (name) => {
  const reader = createReader("./data");
  return reader.read(name);
};

export { FileReader, createReader, processFile };
`
	result, err := parser.ParseFile(context.Background(), "test.js", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile js: %v", err)
	}

	expectedSymbols := []Symbol{
		{Name: "FileReader", Kind: "class"},
		{Name: "constructor", Kind: "constructor", Parent: "FileReader"},
		{Name: "read", Kind: "method", Parent: "FileReader"},
		{Name: "createReader", Kind: "function"},
		{Name: "processFile", Kind: "function"},
	}
	for _, expected := range expectedSymbols {
		found := false
		for _, actual := range result.Symbols {
			if actual.Name == expected.Name && actual.Kind == expected.Kind {
				if expected.Parent != "" && actual.Parent != expected.Parent {
					continue
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("js symbol not found: %+v (got %v)", expected, result.Symbols)
		}
	}

	expectedRefs := []Ref{
		{Name: "readFile", Kind: "import", TargetPath: "fs"},
		{Name: "path", Kind: "import", TargetPath: "path"},
		{Name: "join", Kind: "call"},
		{Name: "readFile", Kind: "call"},
		{Name: "FileReader", Kind: "call"},
		{Name: "createReader", Kind: "call"},
	}
	for _, expected := range expectedRefs {
		found := false
		for _, actual := range result.Refs {
			if actual.Name == expected.Name && actual.Kind == expected.Kind {
				if expected.TargetPath != "" && actual.TargetPath != expected.TargetPath {
					continue
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("js ref not found: %+v", expected)
		}
	}
}

// TestTSParser_DeclarationSpans verifies that EndLine is populated so that
// symbolByFileAndLine can correctly attribute call-site references.
func TestTSParser_DeclarationSpans(t *testing.T) {
	parser := &tsParser{}
	source := `
function outer() {
  function inner() {
    return 1;
  }
  return inner();
}
`
	result, err := parser.ParseFile(context.Background(), "spans.ts", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	for _, sym := range result.Symbols {
		if sym.EndLine == 0 {
			t.Errorf("symbol %q has EndLine=0; EndLine is required for span-based ref attribution", sym.Name)
		}
		if sym.EndLine < sym.Line {
			t.Errorf("symbol %q EndLine %d < Line %d", sym.Name, sym.EndLine, sym.Line)
		}
	}
}

// TestTSParser_ImportKinds verifies that import refs carry the correct TargetPath.
func TestTSParser_ImportKinds(t *testing.T) {
	parser := &tsParser{}
	source := `
import DefaultExport from "some-lib";
import { named } from "other-lib";
import * as all from "@scope/pkg";
import "side-effect-only";
`
	result, err := parser.ParseFile(context.Background(), "imports.ts", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	cases := []struct {
		name       string
		targetPath string
	}{
		{"DefaultExport", "some-lib"},
		{"named", "other-lib"},
		{"all", "@scope/pkg"},
		{"side-effect-only", "side-effect-only"},
	}
	for _, tc := range cases {
		found := false
		for _, ref := range result.Refs {
			if ref.Kind == "import" && ref.Name == tc.name && ref.TargetPath == tc.targetPath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("import ref not found: name=%q targetPath=%q (refs: %v)", tc.name, tc.targetPath, result.Refs)
		}
	}
}
