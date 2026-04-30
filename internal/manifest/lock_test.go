package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/useamaru/amaru/internal/types"
)

func TestLoadLockNotExist(t *testing.T) {
	dir := t.TempDir()
	lock, err := LoadLock(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lock == nil {
		t.Fatal("expected non-nil lock")
	}
	if len(lock.Skills) != 0 || len(lock.Commands) != 0 {
		t.Error("expected empty maps for missing lock file")
	}
	if len(lock.Skillsets) != 0 {
		t.Error("expected empty skillsets map for missing lock file")
	}
}

func TestEntriesForType(t *testing.T) {
	lock := &Lock{
		Skills:   map[string]LockedEntry{"research": {Version: "1.0.0"}},
		Commands: map[string]LockedEntry{"bootstrap": {Version: "2.0.0"}},
		Agents:   map[string]LockedEntry{"coder": {Version: "1.0.0"}},
	}

	tests := []struct {
		name     string
		itemType types.ItemType
		wantKey  string
	}{
		{"skill", types.Skill, "research"},
		{"command", types.Command, "bootstrap"},
		{"agent", types.Agent, "coder"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := lock.EntriesForType(tt.itemType)
			if entries == nil {
				t.Fatal("expected non-nil entries")
			}
			if _, ok := entries[tt.wantKey]; !ok {
				t.Errorf("expected key %s", tt.wantKey)
			}
		})
	}

	if lock.EntriesForType(types.ItemType("widget")) != nil {
		t.Error("expected nil for unknown type")
	}
}

func TestNewLockedEntry(t *testing.T) {
	entry := NewLockedEntry("1.2.3", "main", "abc123")
	if entry.Version != "1.2.3" {
		t.Errorf("expected version 1.2.3, got %s", entry.Version)
	}
	if entry.Registry != "main" {
		t.Errorf("expected registry main, got %s", entry.Registry)
	}
	if entry.Hash != "abc123" {
		t.Errorf("expected hash abc123, got %s", entry.Hash)
	}
	if entry.InstalledAt == "" {
		t.Error("expected installed_at to be set")
	}
}

func TestSaveAndLoadLock(t *testing.T) {
	dir := t.TempDir()
	lock := &Lock{
		Skills: map[string]LockedEntry{
			"research": {
				Version:     "1.0.3",
				Registry:    "main",
				Hash:        "a1b2c3d4e5f6",
				InstalledAt: "2026-02-25T10:00:00Z",
			},
		},
		Commands: map[string]LockedEntry{
			"dev/bootstrap": {
				Version:     "2.0.0",
				Registry:    "main",
				Hash:        "c3d4e5f6a1b2",
				InstalledAt: "2026-02-26T09:00:00Z",
			},
		},
		Skillsets: map[string]LockedSkillset{
			"starter-pack": {
				Registry:    "main",
				Digest:      "abc123def456",
				Members:     []string{"skill/research", "command/bootstrap"},
				InstalledAt: "2026-02-27T08:00:00Z",
			},
		},
	}

	if err := SaveLock(dir, lock); err != nil {
		t.Fatalf("SaveLock error: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(filepath.Join(dir, LockFile)); os.IsNotExist(err) {
		t.Fatal("lock file not created")
	}

	loaded, err := LoadLock(dir)
	if err != nil {
		t.Fatalf("LoadLock error: %v", err)
	}
	if loaded.Skills["research"].Version != "1.0.3" {
		t.Errorf("expected 1.0.3, got %s", loaded.Skills["research"].Version)
	}
	if loaded.Skills["research"].Hash != "a1b2c3d4e5f6" {
		t.Errorf("expected hash a1b2c3d4e5f6, got %s", loaded.Skills["research"].Hash)
	}
	if loaded.Commands["dev/bootstrap"].Version != "2.0.0" {
		t.Errorf("expected 2.0.0, got %s", loaded.Commands["dev/bootstrap"].Version)
	}
	if loaded.LockedAt == "" {
		t.Error("expected locked_at to be set")
	}

	// Verify skillset was persisted
	ss, ok := loaded.Skillsets["starter-pack"]
	if !ok {
		t.Fatal("expected skillset starter-pack in loaded lock")
	}
	if ss.Registry != "main" {
		t.Errorf("expected registry main, got %s", ss.Registry)
	}
	if ss.Digest != "abc123def456" {
		t.Errorf("expected digest abc123def456, got %s", ss.Digest)
	}
	if len(ss.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(ss.Members))
	}
}

func TestSkillsetDigest(t *testing.T) {
	// Deterministic: same items, same digest
	items1 := []string{"skill/research/1.0.0", "command/bootstrap/2.0.0"}
	items2 := []string{"skill/research/1.0.0", "command/bootstrap/2.0.0"}
	if SkillsetDigest(items1) != SkillsetDigest(items2) {
		t.Error("expected same digest for same items")
	}

	// Order-independent: sorted internally
	items3 := []string{"command/bootstrap/2.0.0", "skill/research/1.0.0"}
	if SkillsetDigest(items1) != SkillsetDigest(items3) {
		t.Error("expected same digest regardless of order")
	}

	// Different items, different digest
	items4 := []string{"skill/research/1.1.0", "command/bootstrap/2.0.0"}
	if SkillsetDigest(items1) == SkillsetDigest(items4) {
		t.Error("expected different digest for different versions")
	}

	// Empty items
	empty := SkillsetDigest(nil)
	if empty == "" {
		t.Error("expected non-empty digest even for empty items")
	}

	// Does not modify input slice
	original := []string{"b", "a"}
	_ = SkillsetDigest(original)
	if original[0] != "b" || original[1] != "a" {
		t.Error("SkillsetDigest should not modify input slice")
	}
}
