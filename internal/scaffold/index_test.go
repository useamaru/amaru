package scaffold

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/useamaru/amaru/internal/registry"
)

func TestLoadLocalIndex(t *testing.T) {
	dir := t.TempDir()

	content := `{
  "amaru_version": "1",
  "updated_at": "2026-03-05",
  "skills": {
    "my-skill": {
      "latest": "1.0.0",
      "description": "A test skill",
      "tags": ["test"]
    }
  },
  "commands": {},
  "agents": {},
  "skillsets": {}
}
`
	if err := os.WriteFile(filepath.Join(dir, "amaru_registry.json"), []byte(content), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	idx, err := LoadLocalIndex(dir)
	if err != nil {
		t.Fatalf("LoadLocalIndex() error = %v", err)
	}

	if idx.AmaruVersion != "1" {
		t.Errorf("AmaruVersion = %q, want %q", idx.AmaruVersion, "1")
	}
	if len(idx.Skills) != 1 {
		t.Errorf("len(Skills) = %d, want 1", len(idx.Skills))
	}
	entry, ok := idx.Skills["my-skill"]
	if !ok {
		t.Fatal("expected my-skill in Skills")
	}
	if entry.Latest != "1.0.0" {
		t.Errorf("Latest = %q, want %q", entry.Latest, "1.0.0")
	}
}

func TestLoadLocalIndexMissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadLocalIndex(dir)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadLocalIndexNilMaps(t *testing.T) {
	dir := t.TempDir()

	content := `{"amaru_version": "1", "updated_at": ""}`
	if err := os.WriteFile(filepath.Join(dir, "amaru_registry.json"), []byte(content), 0644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	idx, err := LoadLocalIndex(dir)
	if err != nil {
		t.Fatalf("LoadLocalIndex() error = %v", err)
	}
	if idx.Skills == nil || idx.Commands == nil || idx.Agents == nil || idx.Skillsets == nil {
		t.Error("expected all maps to be initialized, got nil")
	}
}

func TestSaveLocalIndex(t *testing.T) {
	dir := t.TempDir()

	idx := &registry.RegistryIndex{
		AmaruVersion: "1",
		UpdatedAt:    "2026-03-05",
		Skills: map[string]registry.RegistryEntry{
			"test-skill": {Latest: "1.0.0", Description: "Test"},
		},
		Commands:  map[string]registry.RegistryEntry{},
		Agents:    map[string]registry.RegistryEntry{},
		Skillsets: map[string]registry.SkillsetEntry{},
	}

	if err := SaveLocalIndex(dir, idx); err != nil {
		t.Fatalf("SaveLocalIndex() error = %v", err)
	}

	// Verify file exists and is valid JSON
	loaded, err := LoadLocalIndex(dir)
	if err != nil {
		t.Fatalf("LoadLocalIndex() after save error = %v", err)
	}
	if loaded.AmaruVersion != "1" {
		t.Errorf("AmaruVersion = %q, want %q", loaded.AmaruVersion, "1")
	}
	if len(loaded.Skills) != 1 {
		t.Errorf("len(Skills) = %d, want 1", len(loaded.Skills))
	}

	// Verify no temp file left behind
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() == "amaru_registry.json.tmp" {
			t.Error("temp file not cleaned up")
		}
	}
}

func TestFindRegistryRoot(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "amaru_registry.json"), []byte("{}"), 0644)

		root, err := FindRegistryRoot(dir)
		if err != nil {
			t.Fatalf("FindRegistryRoot() error = %v", err)
		}
		if root != dir {
			t.Errorf("root = %q, want %q", root, dir)
		}
	})

	t.Run("not found", func(t *testing.T) {
		dir := t.TempDir()
		_, err := FindRegistryRoot(dir)
		if err == nil {
			t.Fatal("expected error for missing registry")
		}
	})
}
