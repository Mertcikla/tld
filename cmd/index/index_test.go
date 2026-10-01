package index

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMaterializeFlagDefaultsOff(t *testing.T) {
	cmd := NewIndexCmd()
	flag := cmd.Flags().Lookup("materialize")
	if flag == nil {
		t.Fatal("materialize flag missing")
	}
	if flag.DefValue != "false" {
		t.Fatalf("materialize default = %q, want false", flag.DefValue)
	}
}

func TestTreeSignatureChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	if err := os.WriteFile(file, []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, err := treeSignature(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("package a\nfunc A() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second, err := treeSignature(dir)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("signature did not change after file edit")
	}
}
