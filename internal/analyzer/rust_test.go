package analyzer

import (
	"context"
	"testing"
)

func TestRustParser_ParseFile(t *testing.T) {
	parser := &rustParser{}
	source := `
use std::collections::HashMap;
use std::io::{self, Write};
use std::fs as filesystem;

mod internal {
    fn secret() {}
}

struct Point {
    x: i32,
    y: i32,
}

impl Point {
    fn new(x: i32, y: i32) -> Self {
        Point { x, y }
    }

    fn distance(&self) -> f64 {
        ((self.x * self.x + self.y * self.y) as f64).sqrt()
    }
}

trait Drawable {
    fn draw(&self);
}

fn main() {
    let p = Point::new(10, 20);
    p.distance();
    println!("Hello");
}
`
	result, err := parser.ParseFile(context.Background(), "test.rs", []byte(source))
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Verify Symbols
	expectedSymbols := []Symbol{
		{Name: "internal", Kind: "module", Parent: ""},
		{Name: "secret", Kind: "method", Parent: "internal"},
		{Name: "Point", Kind: "struct", Parent: ""},
		{Name: "new", Kind: "method", Parent: "Point"},
		{Name: "distance", Kind: "method", Parent: "Point"},
		{Name: "Drawable", Kind: "trait", Parent: ""},
		{Name: "draw", Kind: "method", Parent: "Drawable"},
		{Name: "main", Kind: "function", Parent: ""},
	}

	for _, expected := range expectedSymbols {
		found := false
		for _, actual := range result.Symbols {
			if actual.Name == expected.Name && actual.Kind == expected.Kind && actual.Parent == expected.Parent {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Symbol not found or mismatch: %+v", expected)
		}
	}

	// Verify Refs
	expectedRefs := []Ref{
		{Name: "HashMap", Kind: "import", TargetPath: "std::collections::HashMap"},
		{Name: "io", Kind: "import", TargetPath: "std::io"},
		{Name: "Write", Kind: "import", TargetPath: "std::io::Write"},
		{Name: "filesystem", Kind: "import", TargetPath: "std::fs"},
		{Name: "new", Kind: "call"},
		{Name: "distance", Kind: "call"},
		{Name: "println!", Kind: "call"},
		{Name: "sqrt", Kind: "call"},
	}

	for _, expected := range expectedRefs {
		found := false
		for _, actual := range result.Refs {
			if actual.Name == expected.Name && (expected.Kind == "" || actual.Kind == expected.Kind) {
				if expected.TargetPath != "" && actual.TargetPath != expected.TargetPath {
					continue
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Ref not found or mismatch: %+v", expected)
		}
	}
}
