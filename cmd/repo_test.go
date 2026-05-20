package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/scaffold"
)

// scaffoldTestRegistry creates a minimal v2 (flat layout) registry in a temp dir.
func scaffoldTestRegistry(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	idx := &registry.RegistryIndex{
		AmaruVersion: "2",
		UpdatedAt:    "2026-04-30",
		Skills:       map[string]registry.RegistryEntry{},
		Commands:     map[string]registry.RegistryEntry{},
		Agents:       map[string]registry.RegistryEntry{},
		Skillsets:    map[string]registry.SkillsetEntry{},
	}
	if err := scaffold.SaveLocalIndex(dir, idx); err != nil {
		t.Fatalf("saving index: %v", err)
	}

	// Create type directories at the repo root (v2 flat layout).
	for _, d := range []string{"skills", "commands", "agents"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0755); err != nil {
			t.Fatalf("creating dir: %v", err)
		}
	}

	return dir
}

// scaffoldTestRegistryWithSkill creates a v2 registry with one skill already added.
func scaffoldTestRegistryWithSkill(t *testing.T, name string) string {
	t.Helper()
	dir := scaffoldTestRegistry(t)

	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skills[name] = registry.RegistryEntry{
		Latest:      "",
		Description: "A test skill",
		Tags:        []string{"test"},
	}
	scaffold.SaveLocalIndex(dir, idx)

	// Create skill directory and files at the flat path.
	skillDir := filepath.Join(dir, "skills", name)
	os.MkdirAll(skillDir, 0755)

	manifest := registry.ItemManifest{
		Name:        name,
		Type:        "skill",
		Version:     "",
		Description: "A test skill",
		Author:      "test",
		Files:       []string{"skill.md"},
		Tags:        []string{"test"},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(skillDir, "manifest.json"), data, 0644)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# test\n"), 0644)

	return dir
}

// scaffoldLegacyTestRegistry creates a v1 (nested) registry — used to assert
// that legacy registries still read correctly via the layout helper.
func scaffoldLegacyTestRegistry(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()

	idx := &registry.RegistryIndex{
		AmaruVersion: "1",
		UpdatedAt:    "2026-03-05",
		Skills: map[string]registry.RegistryEntry{
			name: {Description: "A legacy skill", Tags: []string{"test"}},
		},
		Commands:  map[string]registry.RegistryEntry{},
		Agents:    map[string]registry.RegistryEntry{},
		Skillsets: map[string]registry.SkillsetEntry{},
	}
	if err := scaffold.SaveLocalIndex(dir, idx); err != nil {
		t.Fatalf("saving legacy index: %v", err)
	}

	skillDir := filepath.Join(dir, ".amaru_registry", "skills", name)
	os.MkdirAll(skillDir, 0755)
	manifest := registry.ItemManifest{
		Name:        name,
		Type:        "skill",
		Description: "A legacy skill",
		Author:      "test",
		Files:       []string{"skill.md"},
		Tags:        []string{"test"},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(skillDir, "manifest.json"), data, 0644)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# test\n"), 0644)

	return dir
}

func TestRepoAddCreatesSkill(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Reset flags
	repoAddType = "skill"
	repoAddDescription = "Test skill"
	repoAddAuthor = "tester"
	repoAddTags = "test,example"
	repoAddItems = ""

	if err := runRepoAdd("my-skill"); err != nil {
		t.Fatalf("runRepoAdd() error = %v", err)
	}

	// Verify directory was created at the v2 flat path.
	skillDir := filepath.Join(dir, "skills", "my-skill")
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		t.Fatal("skill directory not created")
	}
	// And not at the legacy nested path.
	if _, err := os.Stat(filepath.Join(dir, ".amaru_registry", "skills", "my-skill")); !os.IsNotExist(err) {
		t.Errorf("v2 repo add should not create legacy .amaru_registry/ path (err=%v)", err)
	}

	// Verify manifest.json
	data, err := os.ReadFile(filepath.Join(skillDir, "manifest.json"))
	if err != nil {
		t.Fatalf("reading manifest.json: %v", err)
	}
	var m registry.ItemManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing manifest.json: %v", err)
	}
	if m.Name != "my-skill" {
		t.Errorf("manifest name = %q, want %q", m.Name, "my-skill")
	}
	if m.Type != "skill" {
		t.Errorf("manifest type = %q, want %q", m.Type, "skill")
	}
	if m.Author != "tester" {
		t.Errorf("manifest author = %q, want %q", m.Author, "tester")
	}

	// Verify content file exists (uppercase by default, Anthropic convention)
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); os.IsNotExist(err) {
		t.Fatal("SKILL.md not created")
	}
	if _, err := os.Stat(filepath.Join(skillDir, "skill.md")); err == nil {
		t.Error("lowercase skill.md should not be created by default")
	}

	// Verify index was updated
	idx, err := scaffold.LoadLocalIndex(dir)
	if err != nil {
		t.Fatalf("loading index: %v", err)
	}
	entry, ok := idx.Skills["my-skill"]
	if !ok {
		t.Fatal("my-skill not in index")
	}
	if entry.Description != "Test skill" {
		t.Errorf("index description = %q, want %q", entry.Description, "Test skill")
	}
}

func TestRepoAddRejectsInvalidName(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skill"
	repoAddDescription = ""
	repoAddAuthor = ""
	repoAddTags = ""
	repoAddItems = ""

	if err := runRepoAdd("Invalid Name!"); err == nil {
		t.Fatal("expected error for invalid name")
	}
}

func TestRepoAddRejectsDuplicate(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "existing")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skill"
	repoAddDescription = ""
	repoAddAuthor = ""
	repoAddTags = ""
	repoAddItems = ""

	if err := runRepoAdd("existing"); err == nil {
		t.Fatal("expected error for duplicate name")
	}
}

func TestRepoAddCommand(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "command"
	repoAddDescription = "Test command"
	repoAddAuthor = "tester"
	repoAddTags = ""
	repoAddItems = ""

	if err := runRepoAdd("my-cmd"); err != nil {
		t.Fatalf("runRepoAdd() error = %v", err)
	}

	// Verify command directory at flat path.
	cmdDir := filepath.Join(dir, "commands", "my-cmd")
	if _, err := os.Stat(filepath.Join(cmdDir, "COMMAND.md")); os.IsNotExist(err) {
		t.Fatal("COMMAND.md not created")
	}

	idx, _ := scaffold.LoadLocalIndex(dir)
	if _, ok := idx.Commands["my-cmd"]; !ok {
		t.Fatal("my-cmd not in index commands")
	}
}

func TestRepoAddSkillset(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "foo")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skillset"
	repoAddDescription = "Test skillset"
	repoAddAuthor = ""
	repoAddTags = ""
	repoAddItems = "skill/foo"

	if err := runRepoAdd("my-pack"); err != nil {
		t.Fatalf("runRepoAdd() error = %v", err)
	}

	idx, _ := scaffold.LoadLocalIndex(dir)
	ss, ok := idx.Skillsets["my-pack"]
	if !ok {
		t.Fatal("my-pack not in skillsets")
	}
	if len(ss.Items) != 1 {
		t.Errorf("skillset items count = %d, want 1", len(ss.Items))
	}
}

func TestRepoAddSkillsetRejectsMissingMember(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skillset"
	repoAddDescription = ""
	repoAddAuthor = ""
	repoAddTags = ""
	repoAddItems = "skill/nonexistent"

	if err := runRepoAdd("bad-pack"); err == nil {
		t.Fatal("expected error for missing member")
	}
}

func TestRepoRemoveSkill(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "to-remove")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoRemoveType = "skill"
	repoRemoveForce = false

	if err := runRepoRemove("to-remove"); err != nil {
		t.Fatalf("runRepoRemove() error = %v", err)
	}

	// Verify removed from index
	idx, _ := scaffold.LoadLocalIndex(dir)
	if _, ok := idx.Skills["to-remove"]; ok {
		t.Fatal("to-remove still in index")
	}

	// Verify directory removed at flat path.
	skillDir := filepath.Join(dir, "skills", "to-remove")
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatal("skill directory not removed")
	}
}

func TestRepoRemoveBlockedBySkillset(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "member")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Add a skillset referencing the skill
	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skillsets["my-pack"] = registry.SkillsetEntry{
		Description: "test",
		Items:       []registry.SkillsetItem{{Type: "skill", Name: "member"}},
	}
	scaffold.SaveLocalIndex(dir, idx)

	repoRemoveType = "skill"
	repoRemoveForce = false

	if err := runRepoRemove("member"); err == nil {
		t.Fatal("expected error when removing item referenced by skillset")
	}

	// Force should work
	repoRemoveForce = true
	if err := runRepoRemove("member"); err != nil {
		t.Fatalf("forced remove error = %v", err)
	}
}

func TestRepoRemoveNotFound(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoRemoveType = "skill"
	repoRemoveForce = false

	if err := runRepoRemove("nonexistent"); err == nil {
		t.Fatal("expected error for nonexistent item")
	}
}

func TestRepoValidateClean(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "valid-skill")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	if err := runRepoValidate(); err != nil {
		t.Fatalf("runRepoValidate() error = %v", err)
	}
}

func TestRepoValidateErrors(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Add an entry with no matching directory
	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skills["ghost"] = registry.RegistryEntry{Description: "missing directory"}
	scaffold.SaveLocalIndex(dir, idx)

	if err := runRepoValidate(); err == nil {
		t.Fatal("expected validation error for missing directory")
	}
}

// TestRepoValidateOnLegacyRegistry proves repo subcommands still read v1
// (nested) registries correctly after the v2 write flip.
func TestRepoValidateOnLegacyRegistry(t *testing.T) {
	dir := scaffoldLegacyTestRegistry(t, "legacy-skill")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	if err := runRepoValidate(); err != nil {
		t.Fatalf("validate on v1 registry should succeed, got: %v", err)
	}
}

func TestRepoAddWithFolder(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skill"
	repoAddDescription = "Test skill in folder"
	repoAddAuthor = "tester"
	repoAddTags = ""
	repoAddItems = ""
	repoAddFolder = "dev"
	defer func() { repoAddFolder = "" }()

	if err := runRepoAdd("research"); err != nil {
		t.Fatalf("runRepoAdd() error = %v", err)
	}

	// Skill should live at skills/dev/research/
	if _, err := os.Stat(filepath.Join(dir, "skills", "dev", "research", "manifest.json")); os.IsNotExist(err) {
		t.Fatal("manifest.json not created at skills/dev/research/")
	}
	// And NOT at skills/research/
	if _, err := os.Stat(filepath.Join(dir, "skills", "research")); !os.IsNotExist(err) {
		t.Errorf("skill should not exist at flat path when folder is set; err=%v", err)
	}

	idx, _ := scaffold.LoadLocalIndex(dir)
	entry, ok := idx.Skills["research"]
	if !ok {
		t.Fatal("research not in index — folder must not be part of the item name")
	}
	if entry.Folder != "dev" {
		t.Errorf("entry Folder = %q, want %q", entry.Folder, "dev")
	}
}

func TestRepoAddRejectsInvalidFolder(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skill"
	repoAddDescription = ""
	repoAddAuthor = ""
	repoAddTags = ""
	repoAddItems = ""
	repoAddFolder = "Bad Folder"
	defer func() { repoAddFolder = "" }()

	if err := runRepoAdd("research"); err == nil {
		t.Fatal("expected error for invalid folder")
	}
}

// TestRepoValidateWithFolder verifies that validate finds folder-organized
// skills correctly and doesn't flag the folder itself as an orphan.
func TestRepoValidateWithFolder(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Build a skill at skills/dev/research/ manually.
	skillDir := filepath.Join(dir, "skills", "dev", "research")
	os.MkdirAll(skillDir, 0755)
	manifest := registry.ItemManifest{
		Name:        "research",
		Type:        "skill",
		Description: "folder-organized",
		Author:      "tester",
		Files:       []string{"skill.md"},
	}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(skillDir, "manifest.json"), data, 0644)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# r\n"), 0644)

	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skills["research"] = registry.RegistryEntry{
		Description: "folder-organized",
		Folder:      "dev",
	}
	scaffold.SaveLocalIndex(dir, idx)

	if err := runRepoValidate(); err != nil {
		t.Fatalf("validate on folder-organized registry should succeed, got: %v", err)
	}
}

// TestRepoValidateFlagsFolderOrphan verifies that a skill on disk inside a
// folder but not in the index is reported as an orphan.
func TestRepoValidateFlagsFolderOrphan(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Orphan: skills/dev/orphan/ with manifest.json but no index entry.
	orphanDir := filepath.Join(dir, "skills", "dev", "orphan")
	os.MkdirAll(orphanDir, 0755)
	manifest := registry.ItemManifest{Name: "orphan", Type: "skill"}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(orphanDir, "manifest.json"), data, 0644)

	// Validate should succeed (orphans are warnings, not errors) but warn.
	if err := runRepoValidate(); err != nil {
		t.Fatalf("validate should warn but not error on orphans, got: %v", err)
	}
}

func TestRepoRemoveWithFolder(t *testing.T) {
	dir := scaffoldTestRegistry(t)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	// Set up a folder-organized skill.
	skillDir := filepath.Join(dir, "skills", "dev", "research")
	os.MkdirAll(skillDir, 0755)
	manifest := registry.ItemManifest{Name: "research", Type: "skill", Files: []string{"skill.md"}}
	data, _ := json.MarshalIndent(manifest, "", "  ")
	os.WriteFile(filepath.Join(skillDir, "manifest.json"), data, 0644)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# r\n"), 0644)

	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skills["research"] = registry.RegistryEntry{Folder: "dev"}
	scaffold.SaveLocalIndex(dir, idx)

	repoRemoveType = "skill"
	repoRemoveForce = false
	if err := runRepoRemove("research"); err != nil {
		t.Fatalf("remove with folder error = %v", err)
	}

	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatal("folder-organized skill directory not removed")
	}
	idx2, _ := scaffold.LoadLocalIndex(dir)
	if _, ok := idx2.Skills["research"]; ok {
		t.Fatal("research still in index after remove")
	}
}

func TestRepoRemoveOnLegacyRegistry(t *testing.T) {
	dir := scaffoldLegacyTestRegistry(t, "to-remove")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoRemoveType = "skill"
	repoRemoveForce = false

	if err := runRepoRemove("to-remove"); err != nil {
		t.Fatalf("remove on v1 registry should succeed, got: %v", err)
	}

	// The legacy nested directory should be gone.
	if _, err := os.Stat(filepath.Join(dir, ".amaru_registry", "skills", "to-remove")); !os.IsNotExist(err) {
		t.Error("legacy skill directory should have been removed")
	}
}
