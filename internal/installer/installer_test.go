package installer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/useamaru/amaru/internal/registry"
)

func TestInstallAndHash(t *testing.T) {
	dir := t.TempDir()
	files := []registry.File{
		{Path: "skill.md", Content: []byte("# Research\nThis is a skill.")},
		{Path: "manifest.json", Content: []byte(`{"name": "research", "version": "1.0.0"}`)},
	}

	hash, err := Install(dir, "skill", "research", files)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if len(hash) != 12 {
		t.Errorf("expected 12-char hash, got %d chars: %s", len(hash), hash)
	}

	// Verify files exist
	skillDir := filepath.Join(dir, SkillsDir, "research")
	if _, err := os.Stat(filepath.Join(skillDir, "skill.md")); os.IsNotExist(err) {
		t.Error("skill.md not created")
	}
	if _, err := os.Stat(filepath.Join(skillDir, "manifest.json")); os.IsNotExist(err) {
		t.Error("manifest.json not created")
	}

	// Verify hash is deterministic
	hash2, err := ComputeHash(skillDir)
	if err != nil {
		t.Fatalf("ComputeHash error: %v", err)
	}
	if hash != hash2 {
		t.Errorf("hash not deterministic: %s != %s", hash, hash2)
	}
}

func TestInstallCommand(t *testing.T) {
	dir := t.TempDir()
	files := []registry.File{
		{Path: "command.md", Content: []byte("# Bootstrap\nBootstrap command.")},
		{Path: "manifest.json", Content: []byte(`{"name": "bootstrap", "version": "2.0.0"}`)},
	}

	hash, err := Install(dir, "command", "dev/bootstrap", files)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}

	cmdDir := filepath.Join(dir, CommandsDir, "dev/bootstrap")
	if _, err := os.Stat(filepath.Join(cmdDir, "command.md")); os.IsNotExist(err) {
		t.Error("command.md not created")
	}
}

func TestInstallWithSubdirectory(t *testing.T) {
	dir := t.TempDir()
	files := []registry.File{
		{Path: "skill.md", Content: []byte("# Skill")},
		{Path: "examples/example1.md", Content: []byte("# Example 1")},
		{Path: "examples/example2.md", Content: []byte("# Example 2")},
	}

	hash, err := Install(dir, "skill", "research", files)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}
	if hash == "" {
		t.Error("expected non-empty hash")
	}

	// Verify subdirectory files
	skillDir := filepath.Join(dir, SkillsDir, "research")
	if _, err := os.Stat(filepath.Join(skillDir, "examples", "example1.md")); os.IsNotExist(err) {
		t.Error("examples/example1.md not created")
	}
}

func TestHashDifferentContent(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	files1 := []registry.File{
		{Path: "skill.md", Content: []byte("content A")},
	}
	files2 := []registry.File{
		{Path: "skill.md", Content: []byte("content B")},
	}

	hash1, _ := Install(dir1, "skill", "test", files1)
	hash2, _ := Install(dir2, "skill", "test", files2)

	if hash1 == hash2 {
		t.Error("different content should produce different hashes")
	}
}

func TestDirForType(t *testing.T) {
	tests := []struct {
		itemType string
		want     string
	}{
		{"skill", SkillsDir},
		{"command", CommandsDir},
		{"agent", AgentsDir},
		{"widget", ".claude/widgets"},
	}

	for _, tt := range tests {
		t.Run(tt.itemType, func(t *testing.T) {
			got := DirForType(tt.itemType)
			if got != tt.want {
				t.Errorf("DirForType(%s) = %s, want %s", tt.itemType, got, tt.want)
			}
		})
	}
}

func TestUninstall(t *testing.T) {
	dir := t.TempDir()
	files := []registry.File{
		{Path: "skill.md", Content: []byte("# Test")},
	}

	Install(dir, "skill", "test", files)

	if !IsInstalled(dir, "skill", "test") {
		t.Error("expected skill to be installed")
	}

	if err := Uninstall(dir, "skill", "test"); err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}

	if IsInstalled(dir, "skill", "test") {
		t.Error("expected skill to be uninstalled")
	}
}
