package symbol

import (
	"context"
	"os"
	"testing"
)

func TestExtractSource_Go(t *testing.T) {
	src := []byte(`
package main

import "fmt"

type Server struct{}
type Handler interface { Handle() }

func NewServer() *Server { return &Server{} }
func (s *Server) Start() { fmt.Println("start") }
func helper() {}
`)
	ctx := context.Background()
	result, err := ExtractSource(ctx, ".go", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSymbols := map[string]string{
		"Server":    "struct",
		"Handler":   "interface",
		"NewServer": "function",
		"Start":     "method",
		"helper":    "function",
	}
	gotSymbols := make(map[string]string)
	for _, s := range result.Symbols {
		gotSymbols[s.Name] = s.Kind
	}
	for name, kind := range wantSymbols {
		if gotSymbols[name] != kind {
			t.Errorf("symbol %q: want kind %q, got %q", name, kind, gotSymbols[name])
		}
	}

	// Should have refs to Println
	foundPrintln := false
	for _, r := range result.Refs {
		if r.Name == "Println" {
			foundPrintln = true
		}
	}
	if !foundPrintln {
		t.Error("expected Println in refs")
	}
}

func TestExtractSource_TypeScript(t *testing.T) {
	src := []byte(`
// A TypeScript module
export class UserService {
  constructor() {}
}

export function createUser(name: string) {
  return new UserService();
}

const deleteUser = async (id: string) => {
  return fetch(id);
};

const updateUser = function(data: any) {
  console.log(data);
};
`)
	ctx := context.Background()
	result, err := ExtractSource(ctx, ".ts", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSymbols := []string{"UserService", "createUser", "deleteUser", "updateUser"}
	gotNames := make(map[string]bool)
	for _, s := range result.Symbols {
		gotNames[s.Name] = true
	}
	for _, name := range wantSymbols {
		if !gotNames[name] {
			t.Errorf("expected symbol %q, got: %v", name, result.Symbols)
		}
	}
}

func TestExtractSource_JavaScript(t *testing.T) {
	src := []byte(`
function greet(name) {
  console.log("Hello " + name);
}

class Animal {
  speak() {}
}

const run = () => {
  greet("world");
};
`)
	ctx := context.Background()
	result, err := ExtractSource(ctx, ".js", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSymbols := []string{"greet", "Animal", "run"}
	gotNames := make(map[string]bool)
	for _, s := range result.Symbols {
		gotNames[s.Name] = true
	}
	for _, name := range wantSymbols {
		if !gotNames[name] {
			t.Errorf("expected symbol %q in JS result, got: %v", name, result.Symbols)
		}
	}
}

func TestExtractSource_Python(t *testing.T) {
	src := []byte(`
class PaymentService:
    def __init__(self):
        pass

    def charge(self, amount):
        return process(amount)

def process(amount):
    return amount * 1.1
`)
	ctx := context.Background()
	result, err := ExtractSource(ctx, ".py", src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantSymbols := []string{"PaymentService", "__init__", "charge", "process"}
	gotNames := make(map[string]bool)
	for _, s := range result.Symbols {
		gotNames[s.Name] = true
	}
	for _, name := range wantSymbols {
		if !gotNames[name] {
			t.Errorf("expected symbol %q in Python result, got: %v", name, result.Symbols)
		}
	}
}

func TestExtractSource_Unsupported(t *testing.T) {
	ctx := context.Background()
	_, err := ExtractSource(ctx, ".rb", []byte("def foo; end"))
	if err == nil {
		t.Fatal("expected error for unsupported language")
	}
	if _, ok := err.(ErrUnsupportedLanguage); !ok {
		t.Errorf("expected ErrUnsupportedLanguage, got %T: %v", err, err)
	}
}

func TestHasSymbol(t *testing.T) {
	// Write a temp file and test HasSymbol
	src := []byte(`package main

func MyFunc() {}
func OtherFunc() {}
`)
	tmpFile := t.TempDir() + "/main.go"
	if err := writeFile(tmpFile, src); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	found, err := HasSymbol(ctx, tmpFile, "MyFunc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Error("expected MyFunc to be found")
	}

	notFound, err := HasSymbol(ctx, tmpFile, "NonExistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if notFound {
		t.Error("NonExistent should not be found")
	}
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0600)
}
