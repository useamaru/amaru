package checker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Masterminds/semver/v3"
	"github.com/useamaru/amaru/internal/installer"
	"github.com/useamaru/amaru/internal/manifest"
	"github.com/useamaru/amaru/internal/registry"
)

// mockRegistryClient implements registry.Client for testing.
type mockRegistryClient struct {
	versions map[string][]*semver.Version // key: "itemType/name"
	files    map[string][]registry.File   // key: "itemType/name/version"
}

func (m *mockRegistryClient) FetchIndex(ctx context.Context) (*registry.RegistryIndex, error) {
	return &registry.RegistryIndex{}, nil
}

func (m *mockRegistryClient) ListVersions(ctx context.Context, itemType, name string) ([]*semver.Version, error) {
	key := itemType + "/" + name
	vs, ok := m.versions[key]
	if !ok {
		// Simulate tag-less registry: return nil, nil (no error, no versions)
		return nil, nil
	}
	return vs, nil
}

func (m *mockRegistryClient) DownloadFiles(ctx context.Context, itemType, name, version string) ([]registry.File, error) {
	key := itemType + "/" + name + "/" + version
	fs, ok := m.files[key]
	if !ok {
		return nil, fmt.Errorf("not found: %s", key)
	}
	return fs, nil
}

func (m *mockRegistryClient) FetchSkillsetManifest(ctx context.Context, name, version string) (*registry.SkillsetManifest, error) {
	return nil, fmt.Errorf("not implemented in mock")
}

func semverList(strs ...string) []*semver.Version {
	var vs []*semver.Version
	for _, s := range strs {
		v, _ := semver.NewVersion(s)
		vs = append(vs, v)
	}
	return vs
}

func TestCheckDetectsUpdates(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0.0",
		Registries: map[string]manifest.RegistryConfig{
			"main": {URL: "github:acme/skills", Auth: "none"},
		},
		Skills: map[string]manifest.DependencySpec{
			"research": {Version: "^1.0.0"},
		},
	}

	lock := &manifest.Lock{
		Skills: map[string]manifest.LockedEntry{
			"research": {Version: "1.0.0", Registry: "main", Hash: "abc123"},
		},
		Commands: map[string]manifest.LockedEntry{},
		Agents:   map[string]manifest.LockedEntry{},
	}

	client := &mockRegistryClient{
		versions: map[string][]*semver.Version{
			"skill/research": semverList("1.0.0", "1.1.0", "1.2.0"),
		},
	}

	clients := map[string]registry.Client{"main": client}

	result, err := Check(context.Background(), t.TempDir(), m, lock, clients)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if len(result.Updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(result.Updates))
	}
	if result.Updates[0].Latest != "1.2.0" {
		t.Errorf("expected latest 1.2.0, got %s", result.Updates[0].Latest)
	}
	if result.Updates[0].Category != "minor" {
		t.Errorf("expected minor update, got %s", result.Updates[0].Category)
	}
}

func TestCheckDetectsDrift(t *testing.T) {
	dir := t.TempDir()

	// Install a skill so it exists on disk
	files := []registry.File{
		{Path: "skill.md", Content: []byte("# Research v1")},
	}
	hash, err := installer.Install(dir, "skill", "research", files)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}

	// Now modify the file locally to create drift
	skillFile := filepath.Join(dir, installer.SkillsDir, "research", "skill.md")
	os.WriteFile(skillFile, []byte("# Research MODIFIED"), 0644)

	m := &manifest.Manifest{
		Version: "1.0.0",
		Registries: map[string]manifest.RegistryConfig{
			"main": {URL: "github:acme/skills", Auth: "none"},
		},
		Skills: map[string]manifest.DependencySpec{
			"research": {Version: "^1.0.0"},
		},
	}

	lock := &manifest.Lock{
		Skills: map[string]manifest.LockedEntry{
			"research": {Version: "1.0.0", Registry: "main", Hash: hash},
		},
		Commands: map[string]manifest.LockedEntry{},
		Agents:   map[string]manifest.LockedEntry{},
	}

	client := &mockRegistryClient{
		versions: map[string][]*semver.Version{
			"skill/research": semverList("1.0.0"),
		},
	}

	clients := map[string]registry.Client{"main": client}

	result, err := Check(context.Background(), dir, m, lock, clients)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if len(result.Drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(result.Drifts))
	}
	if result.Drifts[0].Name != "research" {
		t.Errorf("expected research drift, got %s", result.Drifts[0].Name)
	}
}

func TestCheckUpToDate(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0.0",
		Registries: map[string]manifest.RegistryConfig{
			"main": {URL: "github:acme/skills", Auth: "none"},
		},
		Skills: map[string]manifest.DependencySpec{
			"research": {Version: "^1.0.0"},
		},
	}

	lock := &manifest.Lock{
		Skills: map[string]manifest.LockedEntry{
			"research": {Version: "1.0.0", Registry: "main", Hash: "abc123"},
		},
		Commands: map[string]manifest.LockedEntry{},
		Agents:   map[string]manifest.LockedEntry{},
	}

	client := &mockRegistryClient{
		versions: map[string][]*semver.Version{
			"skill/research": semverList("1.0.0"),
		},
	}

	clients := map[string]registry.Client{"main": client}

	result, err := Check(context.Background(), t.TempDir(), m, lock, clients)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if len(result.Updates) != 0 {
		t.Errorf("expected 0 updates, got %d", len(result.Updates))
	}
	if result.UpToDate != 1 {
		t.Errorf("expected 1 up-to-date, got %d", result.UpToDate)
	}
}

func TestCheckMissingClient(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0.0",
		Registries: map[string]manifest.RegistryConfig{
			"main": {URL: "github:acme/skills", Auth: "none"},
		},
		Skills: map[string]manifest.DependencySpec{
			"research": {Version: "^1.0.0"},
		},
	}

	lock := &manifest.Lock{
		Skills: map[string]manifest.LockedEntry{
			"research": {Version: "1.0.0", Registry: "main", Hash: "abc123"},
		},
		Commands: map[string]manifest.LockedEntry{},
		Agents:   map[string]manifest.LockedEntry{},
	}

	// No clients provided
	clients := map[string]registry.Client{}

	_, err := Check(context.Background(), t.TempDir(), m, lock, clients)
	if err == nil {
		t.Error("expected error for missing client")
	}
}

func TestCheckLatestConstraintSkipsSemver(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0.0",
		Registries: map[string]manifest.RegistryConfig{
			"main": {URL: "github:acme/skills", Auth: "none"},
		},
		Skills: map[string]manifest.DependencySpec{
			"unversioned": {Version: "latest"},
		},
	}

	lock := &manifest.Lock{
		Skills: map[string]manifest.LockedEntry{
			"unversioned": {Version: "latest", Registry: "main", Hash: "abc123"},
		},
		Commands:  map[string]manifest.LockedEntry{},
		Agents:    map[string]manifest.LockedEntry{},
		Skillsets: map[string]manifest.LockedSkillset{},
	}

	// Client doesn't need versions for "latest" items
	client := &mockRegistryClient{
		versions: map[string][]*semver.Version{},
	}

	clients := map[string]registry.Client{"main": client}

	result, err := Check(context.Background(), t.TempDir(), m, lock, clients)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if len(result.Updates) != 0 {
		t.Errorf("expected 0 updates for latest constraint, got %d", len(result.Updates))
	}
	if result.UpToDate != 1 {
		t.Errorf("expected 1 up-to-date, got %d", result.UpToDate)
	}
}

func TestCheckLatestConstraintDetectsDrift(t *testing.T) {
	dir := t.TempDir()

	// Install a skill so it exists on disk
	files := []registry.File{
		{Path: "skill.md", Content: []byte("# Original content")},
	}
	hash, err := installer.Install(dir, "skill", "unversioned", files)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}

	// Now modify the file locally to create drift
	skillFile := filepath.Join(dir, installer.SkillsDir, "unversioned", "skill.md")
	os.WriteFile(skillFile, []byte("# Modified locally"), 0644)

	m := &manifest.Manifest{
		Version: "1.0.0",
		Registries: map[string]manifest.RegistryConfig{
			"main": {URL: "github:acme/skills", Auth: "none"},
		},
		Skills: map[string]manifest.DependencySpec{
			"unversioned": {Version: "latest"},
		},
	}

	lock := &manifest.Lock{
		Skills: map[string]manifest.LockedEntry{
			"unversioned": {Version: "latest", Registry: "main", Hash: hash},
		},
		Commands:  map[string]manifest.LockedEntry{},
		Agents:    map[string]manifest.LockedEntry{},
		Skillsets: map[string]manifest.LockedSkillset{},
	}

	client := &mockRegistryClient{
		versions: map[string][]*semver.Version{},
	}

	clients := map[string]registry.Client{"main": client}

	result, err := Check(context.Background(), dir, m, lock, clients)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	if len(result.Drifts) != 1 {
		t.Fatalf("expected 1 drift for latest constraint, got %d", len(result.Drifts))
	}
	if result.Drifts[0].Version != "latest" {
		t.Errorf("expected version latest in drift, got %s", result.Drifts[0].Version)
	}
}

func TestCheckTaglessRegistryNoUpdate(t *testing.T) {
	// Simulates a registry that doesn't use per-item version tags.
	// ListVersions returns empty, so no update should be reported.
	m := &manifest.Manifest{
		Version: "1.0.0",
		Registries: map[string]manifest.RegistryConfig{
			"main": {URL: "github:acme/skills", Auth: "none"},
		},
		Skills: map[string]manifest.DependencySpec{
			"research": {Version: "^1.0.0"},
		},
	}

	lock := &manifest.Lock{
		Skills: map[string]manifest.LockedEntry{
			"research": {Version: "1.0.0", Registry: "main", Hash: "abc123"},
		},
		Commands:  map[string]manifest.LockedEntry{},
		Agents:    map[string]manifest.LockedEntry{},
		Skillsets: map[string]manifest.LockedSkillset{},
	}

	// No versions registered — simulates a tag-less registry
	client := &mockRegistryClient{
		versions: map[string][]*semver.Version{},
	}

	clients := map[string]registry.Client{"main": client}

	result, err := Check(context.Background(), t.TempDir(), m, lock, clients)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	// With no tags, we can't determine if there's an update, so item is up-to-date
	if len(result.Updates) != 0 {
		t.Errorf("expected 0 updates for tag-less registry, got %d", len(result.Updates))
	}
	if result.UpToDate != 1 {
		t.Errorf("expected 1 up-to-date, got %d", result.UpToDate)
	}
}

func TestCheckSkipsUnlockedDeps(t *testing.T) {
	m := &manifest.Manifest{
		Version: "1.0.0",
		Registries: map[string]manifest.RegistryConfig{
			"main": {URL: "github:acme/skills", Auth: "none"},
		},
		Skills: map[string]manifest.DependencySpec{
			"research": {Version: "^1.0.0"},
			"plan":     {Version: "^1.0.0"},
		},
	}

	// Only research is locked, plan is not
	lock := &manifest.Lock{
		Skills: map[string]manifest.LockedEntry{
			"research": {Version: "1.0.0", Registry: "main", Hash: "abc123"},
		},
		Commands: map[string]manifest.LockedEntry{},
		Agents:   map[string]manifest.LockedEntry{},
	}

	client := &mockRegistryClient{
		versions: map[string][]*semver.Version{
			"skill/research": semverList("1.0.0"),
		},
	}

	clients := map[string]registry.Client{"main": client}

	result, err := Check(context.Background(), t.TempDir(), m, lock, clients)
	if err != nil {
		t.Fatalf("Check error: %v", err)
	}
	// Should only check the locked dep, skip the unlocked one
	if result.UpToDate != 1 {
		t.Errorf("expected 1 up-to-date, got %d", result.UpToDate)
	}
}
