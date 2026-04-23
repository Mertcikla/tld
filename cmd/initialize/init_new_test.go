package initialize_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/tld/cmd"
)

func TestInitCmd_CreatesTldDirectory(t *testing.T) {
	dir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)

	// Change to temp dir to test default ".tld" directory creation
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// Run init without args
	_, _, err = cmd.RunCmd(t, ".", "init")
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	// Check if ".tld" directory was created
	tldDir := filepath.Join(dir, ".tld")
	if stat, err := os.Stat(tldDir); err != nil || !stat.IsDir() {
		t.Fatalf(".tld directory was not created: %v", err)
	}

	// Check if new-model YAML files were created in .tld/
	files := []string{"elements.yaml", "connectors.yaml"}
	for _, f := range files {
		path := filepath.Join(tldDir, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("file %s was not created in .tld directory", f)
		}
	}

	// Legacy bridge files should not be scaffolded during init.
	legacyFiles := []string{"diagrams.yaml", "objects.yaml", "edges.yaml", "links.yaml"}
	for _, f := range legacyFiles {
		path := filepath.Join(tldDir, f)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("legacy file %s should not be created during init", f)
		}
	}
}

func TestInitCmd_CustomDirectory(t *testing.T) {
	dir := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)

	customDir := filepath.Join(dir, "my-diagrams")

	// Run init with custom dir
	_, _, err := cmd.RunCmd(t, ".", "init", customDir)
	if err != nil {
		t.Fatalf("init: %v", err)
	}

	// Check if custom directory was created
	if stat, err := os.Stat(customDir); err != nil || !stat.IsDir() {
		t.Fatalf("custom directory %s was not created", customDir)
	}

	// Check if new-model YAML files were created in customDir/
	for _, f := range []string{"elements.yaml", "connectors.yaml"} {
		if _, err := os.Stat(filepath.Join(customDir, f)); err != nil {
			t.Errorf("%s was not created in custom directory", f)
		}
	}

	for _, f := range []string{"diagrams.yaml", "objects.yaml", "edges.yaml", "links.yaml"} {
		if _, err := os.Stat(filepath.Join(customDir, f)); !os.IsNotExist(err) {
			t.Errorf("legacy file %s should not be created in custom directory", f)
		}
	}
}
