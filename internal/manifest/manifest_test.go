package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/useamaru/amaru/internal/types"
)

func TestDependencySpecUnmarshalShorthand(t *testing.T) {
	input := `"^1.0.0"`
	var spec DependencySpec
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Version != "^1.0.0" {
		t.Errorf("expected version ^1.0.0, got %s", spec.Version)
	}
	if spec.Registry != "" {
		t.Errorf("expected empty registry, got %s", spec.Registry)
	}
}

func TestDependencySpecUnmarshalFullForm(t *testing.T) {
	input := `{"version": "^1.2.0", "registry": "main"}`
	var spec DependencySpec
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Version != "^1.2.0" {
		t.Errorf("expected version ^1.2.0, got %s", spec.Version)
	}
	if spec.Registry != "main" {
		t.Errorf("expected registry main, got %s", spec.Registry)
	}
}

func TestDependencySpecMarshalShorthand(t *testing.T) {
	spec := DependencySpec{Version: "^1.0.0"}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `"^1.0.0"` {
		t.Errorf("expected shorthand marshal, got %s", string(data))
	}
}

func TestDependencySpecMarshalFullForm(t *testing.T) {
	spec := DependencySpec{Version: "^1.2.0", Registry: "main"}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unexpected error unmarshaling result: %v", err)
	}
	if result["version"] != "^1.2.0" || result["registry"] != "main" {
		t.Errorf("unexpected full form: %s", string(data))
	}
}

func TestSkillsetSpecMarshalShorthand(t *testing.T) {
	spec := SkillsetSpec{Version: "^1.0.0"}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `"^1.0.0"` {
		t.Errorf("expected shorthand marshal, got %s", string(data))
	}
}

func TestSkillsetSpecMarshalFullForm(t *testing.T) {
	spec := SkillsetSpec{Version: "^1.0.0", Registry: "main"}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result map[string]string
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("expected object form, got: %s", string(data))
	}
	if result["version"] != "^1.0.0" || result["registry"] != "main" {
		t.Errorf("unexpected full form: %s", string(data))
	}
}

func TestSkillsetSpecRoundTrip(t *testing.T) {
	// Shorthand
	input := `"^1.0.0"`
	var spec SkillsetSpec
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Version != "^1.0.0" || spec.Registry != "" {
		t.Errorf("unexpected spec: %+v", spec)
	}

	// Full form
	input = `{"version": "^2.0.0", "registry": "platform"}`
	if err := json.Unmarshal([]byte(input), &spec); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spec.Version != "^2.0.0" || spec.Registry != "platform" {
		t.Errorf("unexpected spec: %+v", spec)
	}
}

func TestSetSkillset(t *testing.T) {
	m := &Manifest{
		Version:    "1.0.0",
		Registries: map[string]RegistryConfig{"main": {URL: "github:a/b", Auth: "none"}},
	}

	m.SetSkillset("starter-pack", SkillsetSpec{Version: "^1.0.0"})
	if m.Skillsets == nil {
		t.Fatal("expected non-nil Skillsets")
	}
	if m.Skillsets["starter-pack"].Version != "^1.0.0" {
		t.Errorf("expected ^1.0.0, got %s", m.Skillsets["starter-pack"].Version)
	}
}

func TestManifestWithSkillsetsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Version:    "1.0.0",
		Registries: map[string]RegistryConfig{"main": {URL: "github:a/b", Auth: "none"}},
		Skillsets:  map[string]SkillsetSpec{"starter-pack": {Version: "^1.0.0"}},
	}

	if err := Save(dir, m); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.Skillsets["starter-pack"].Version != "^1.0.0" {
		t.Errorf("expected ^1.0.0, got %s", loaded.Skillsets["starter-pack"].Version)
	}
}

func TestOldManifestWithGroupFieldStillLoads(t *testing.T) {
	// Old manifests with "group" field should still load (field is silently ignored)
	dir := t.TempDir()
	content := `{
  "version": "1.0.0",
  "registries": {
    "main": { "url": "github:acme-org/acme-skills", "auth": "none" }
  },
  "skills": {
    "research": { "version": "^1.0.0", "group": "starter-pack" }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error loading old manifest: %v", err)
	}
	if m.Skills["research"].Version != "^1.0.0" {
		t.Errorf("expected ^1.0.0, got %s", m.Skills["research"].Version)
	}
}

func TestDependencySpecLatestVersion(t *testing.T) {
	// "latest" version with no registry or group should marshal as shorthand
	spec := DependencySpec{Version: "latest"}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != `"latest"` {
		t.Errorf("expected shorthand \"latest\", got %s", string(data))
	}

	// Round-trip
	var loaded DependencySpec
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.Version != "latest" {
		t.Errorf("expected version latest, got %s", loaded.Version)
	}
}

func TestManifestLoadShorthand(t *testing.T) {
	dir := t.TempDir()
	content := `{
  "version": "1.0.0",
  "registries": {
    "main": { "url": "github:acme-org/acme-skills", "auth": "github" }
  },
  "skills": {
    "research": "^1.0.0",
    "plan": "^1.0.0"
  }
}`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Skills["research"].Version != "^1.0.0" {
		t.Errorf("expected research ^1.0.0, got %s", m.Skills["research"].Version)
	}
	if m.Skills["research"].Registry != "" {
		t.Errorf("expected empty registry for shorthand, got %s", m.Skills["research"].Registry)
	}
}

func TestManifestLoadMultiRegistry(t *testing.T) {
	dir := t.TempDir()
	content := `{
  "version": "1.0.0",
  "registries": {
    "main": { "url": "github:acme-org/acme-skills", "auth": "github" },
    "platform": { "url": "github:acme-org/platform-skills", "auth": "github" }
  },
  "skills": {
    "research": { "version": "^1.0.0", "registry": "main" },
    "deploycheck": { "version": "^1.0.0", "registry": "platform" }
  }
}`
	if err := os.WriteFile(filepath.Join(dir, ManifestFile), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Skills["deploycheck"].Registry != "platform" {
		t.Errorf("expected platform registry, got %s", m.Skills["deploycheck"].Registry)
	}
}

func TestManifestValidation(t *testing.T) {
	tests := []struct {
		name    string
		m       Manifest
		wantErr bool
	}{
		{
			name:    "empty version",
			m:       Manifest{Registries: map[string]RegistryConfig{"x": {URL: "github:a/b", Auth: "none"}}},
			wantErr: true,
		},
		{
			name:    "no registries",
			m:       Manifest{Version: "1.0.0", Registries: map[string]RegistryConfig{}},
			wantErr: true,
		},
		{
			name: "invalid auth",
			m: Manifest{
				Version:    "1.0.0",
				Registries: map[string]RegistryConfig{"x": {URL: "github:a/b", Auth: "oauth"}},
			},
			wantErr: true,
		},
		{
			name: "valid",
			m: Manifest{
				Version:    "1.0.0",
				Registries: map[string]RegistryConfig{"x": {URL: "github:a/b", Auth: "github"}},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.m.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestResolveRegistry(t *testing.T) {
	m := &Manifest{
		Version: "1.0.0",
		Registries: map[string]RegistryConfig{
			"main": {URL: "github:acme-org/acme-skills", Auth: "github"},
		},
	}

	// Shorthand should resolve to default
	alias, err := m.ResolveRegistry(DependencySpec{Version: "^1.0.0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias != "main" {
		t.Errorf("expected main, got %s", alias)
	}

	// Explicit registry
	alias, err = m.ResolveRegistry(DependencySpec{Version: "^1.0.0", Registry: "main"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alias != "main" {
		t.Errorf("expected main, got %s", alias)
	}

	// Multi-registry without explicit should error
	m.Registries["platform"] = RegistryConfig{URL: "github:acme-org/platform-skills", Auth: "github"}
	_, err = m.ResolveRegistry(DependencySpec{Version: "^1.0.0"})
	if err == nil {
		t.Error("expected error for ambiguous registry")
	}
}

func TestDepsForType(t *testing.T) {
	m := &Manifest{
		Skills:   map[string]DependencySpec{"research": {Version: "^1.0.0"}},
		Commands: map[string]DependencySpec{"bootstrap": {Version: "^2.0.0"}},
		Agents:   map[string]DependencySpec{"coder": {Version: "^1.0.0"}},
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
			deps := m.DepsForType(tt.itemType)
			if deps == nil {
				t.Fatal("expected non-nil deps")
			}
			if _, ok := deps[tt.wantKey]; !ok {
				t.Errorf("expected key %s in deps", tt.wantKey)
			}
		})
	}

	// Unknown type returns nil
	if m.DepsForType(types.ItemType("widget")) != nil {
		t.Error("expected nil for unknown type")
	}
}

func TestAllDeps(t *testing.T) {
	m := &Manifest{
		Skills:   map[string]DependencySpec{"research": {Version: "^1.0.0"}},
		Commands: map[string]DependencySpec{"bootstrap": {Version: "^2.0.0"}},
		Agents:   map[string]DependencySpec{"coder": {Version: "^1.0.0"}},
	}

	var names []string
	err := m.AllDeps(func(t types.ItemType, name string, spec DependencySpec) error {
		names = append(names, name)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 3 {
		t.Errorf("expected 3 deps, got %d", len(names))
	}

	// Test error propagation
	err = m.AllDeps(func(t types.ItemType, name string, spec DependencySpec) error {
		return fmt.Errorf("stop")
	})
	if err == nil {
		t.Error("expected error propagation")
	}
}

func TestIsIgnored(t *testing.T) {
	m := &Manifest{Ignored: []string{"research", "bootstrap"}}

	if !m.IsIgnored("research") {
		t.Error("expected research to be ignored")
	}
	if !m.IsIgnored("bootstrap") {
		t.Error("expected bootstrap to be ignored")
	}
	if m.IsIgnored("plan") {
		t.Error("expected plan to not be ignored")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	m := &Manifest{
		Version: "1.0.0",
		Registries: map[string]RegistryConfig{
			"main": {URL: "github:acme-org/acme-skills", Auth: "github"},
		},
		Skills: map[string]DependencySpec{
			"research": {Version: "^1.0.0"},
			"plan":     {Version: "^1.0.0", Registry: "main"},
		},
		Commands: map[string]DependencySpec{
			"dev/bootstrap": {Version: "^2.0.0", Registry: "main"},
		},
	}

	if err := Save(dir, m); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.Skills["research"].Version != "^1.0.0" {
		t.Errorf("expected ^1.0.0, got %s", loaded.Skills["research"].Version)
	}
	if loaded.Commands["dev/bootstrap"].Registry != "main" {
		t.Errorf("expected main registry, got %s", loaded.Commands["dev/bootstrap"].Registry)
	}
}
