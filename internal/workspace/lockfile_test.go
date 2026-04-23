package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockFile(t *testing.T) {
	tmpDir := t.TempDir()

	versionID := "v123"
	appliedBy := "tester"
	parentVersion := "v122"

	lockFile, err := CreateLockFile(versionID, appliedBy, &ResourceCounts{
		Elements:   1,
		Views:      2,
		Connectors: 3,
	}, &parentVersion)
	if err != nil {
		t.Errorf("CreateLockFile failed: %v", err)
	}
	if lockFile.VersionID != versionID {
		t.Errorf("expected version ID %s, got %s", versionID, lockFile.VersionID)
	}
	if lockFile.AppliedBy != appliedBy {
		t.Errorf("expected applied by %s, got %s", appliedBy, lockFile.AppliedBy)
	}
	if lockFile.Resources == nil || lockFile.Resources.Elements != 1 {
		t.Errorf("expected 1 element, got %+v", lockFile.Resources)
	}

	err = WriteLockFile(tmpDir, lockFile)
	if err != nil {
		t.Errorf("WriteLockFile failed: %v", err)
	}

	loaded, err := LoadLockFile(tmpDir)
	if err != nil {
		t.Errorf("LoadLockFile failed: %v", err)
	}
	if loaded.VersionID != versionID {
		t.Errorf("expected version ID %s, got %s", versionID, loaded.VersionID)
	}

	// Test Update
	newVersionID := "v124"
	UpdateLockFile(loaded, newVersionID, appliedBy, &ResourceCounts{Elements: 2, Views: 3, Connectors: 4}, "hash123", &versionID, nil)
	if loaded.VersionID != newVersionID {
		t.Errorf("expected new version ID %s, got %s", newVersionID, loaded.VersionID)
	}
	if loaded.WorkspaceHash != "hash123" {
		t.Errorf("expected hash123, got %s", loaded.WorkspaceHash)
	}
}

func TestCalculateWorkspaceHash(t *testing.T) {
	tmpDir := t.TempDir()

	if err := os.WriteFile(filepath.Join(tmpDir, "diagrams.yaml"), []byte("diagrams: {}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "objects.yaml"), []byte("objects: {}"), 0600); err != nil {
		t.Fatal(err)
	}

	hash1, err := CalculateWorkspaceHash(tmpDir)
	if err != nil {
		t.Errorf("CalculateWorkspaceHash failed: %v", err)
	}
	if hash1 == "" {
		t.Error("expected non-empty hash")
	}

	// Change a file
	if err := os.WriteFile(filepath.Join(tmpDir, "diagrams.yaml"), []byte("diagrams: {d1: {}}"), 0600); err != nil {
		t.Fatal(err)
	}
	hash2, err := CalculateWorkspaceHash(tmpDir)
	if err != nil {
		t.Errorf("CalculateWorkspaceHash failed: %v", err)
	}
	if hash1 == hash2 {
		t.Error("expected different hashes after file change")
	}
}

func TestCalculateWorkspaceHash_PositionChangeIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	base := `api:
  name: API
  diagrams:
    - diagram: system
      position_x: 10
      position_y: 20
`
	changed := `api:
  name: API
  diagrams:
    - diagram: system
      position_x: 99
      position_y: 42
`
	if err := os.WriteFile(filepath.Join(tmpDir, "objects.yaml"), []byte(base), 0600); err != nil {
		t.Fatal(err)
	}
	hash1, err := CalculateWorkspaceHash(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "objects.yaml"), []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	hash2, err := CalculateWorkspaceHash(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if hash1 != hash2 {
		t.Fatalf("expected identical hashes, got %s and %s", hash1, hash2)
	}
}

func TestCalculateWorkspaceHash_NameChangeCaptured(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "diagrams.yaml"), []byte("d1:\n  name: One\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hash1, _ := CalculateWorkspaceHash(tmpDir)
	if err := os.WriteFile(filepath.Join(tmpDir, "diagrams.yaml"), []byte("d1:\n  name: Two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hash2, _ := CalculateWorkspaceHash(tmpDir)
	if hash1 == hash2 {
		t.Fatal("expected name change to affect hash")
	}
}

func TestCalculateWorkspaceHash_Deterministic(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "elements.yaml"), []byte("api:\n  name: API\n  kind: service\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hash1, _ := CalculateWorkspaceHash(tmpDir)
	hash2, _ := CalculateWorkspaceHash(tmpDir)
	if hash1 != hash2 {
		t.Fatalf("expected deterministic hashes, got %s and %s", hash1, hash2)
	}
}
