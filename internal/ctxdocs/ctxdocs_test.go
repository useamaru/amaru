package ctxdocs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/useamaru/amaru/internal/manifest"
)

func TestResolveConfig(t *testing.T) {
	t.Run("valid config", func(t *testing.T) {
		m := &manifest.Manifest{
			Registries: map[string]manifest.RegistryConfig{
				"main": {URL: "github:acme/registry", Auth: "github"},
			},
			Context: &manifest.ContextConfig{
				Registry: "main",
				Project:  "myapp",
				Path:     "docs/ctx",
			},
		}

		cfg, err := ResolveConfig(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.RegAlias != "main" {
			t.Errorf("expected alias main, got %s", cfg.RegAlias)
		}
		if cfg.Project != "myapp" {
			t.Errorf("expected project myapp, got %s", cfg.Project)
		}
		if cfg.LocalPath != "docs/ctx" {
			t.Errorf("expected path docs/ctx, got %s", cfg.LocalPath)
		}
	})

	t.Run("default path", func(t *testing.T) {
		m := &manifest.Manifest{
			Registries: map[string]manifest.RegistryConfig{
				"main": {URL: "github:acme/registry", Auth: "github"},
			},
			Context: &manifest.ContextConfig{
				Registry: "main",
				Project:  "myapp",
			},
		}

		cfg, err := ResolveConfig(m)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.LocalPath != "docs/context" {
			t.Errorf("expected default path docs/context, got %s", cfg.LocalPath)
		}
	})

	t.Run("missing context", func(t *testing.T) {
		m := &manifest.Manifest{}
		_, err := ResolveConfig(m)
		if err == nil {
			t.Error("expected error for nil context")
		}
	})

	t.Run("missing registry", func(t *testing.T) {
		m := &manifest.Manifest{
			Registries: map[string]manifest.RegistryConfig{},
			Context: &manifest.ContextConfig{
				Registry: "missing",
				Project:  "myapp",
			},
		}
		_, err := ResolveConfig(m)
		if err == nil {
			t.Error("expected error for missing registry")
		}
	})
}

func TestRepoURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantURL string
	}{
		{"github shorthand", "github:acme/registry", "https://github.com/acme/registry.git"},
		{"plain URL", "https://example.com/repo.git", "https://example.com/repo.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Registry: manifest.RegistryConfig{URL: tt.url}}
			got, err := cfg.RepoURL()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantURL {
				t.Errorf("RepoURL() = %s, want %s", got, tt.wantURL)
			}
		})
	}
}

func TestSparsePaths(t *testing.T) {
	cfg := &Config{Project: "myapp"}
	paths := cfg.SparsePaths()
	want := []string{
		".amaru_registry/context/myapp", // legacy (v1) — kept for back-compat
		"context/myapp",                 // flat (v2)
		"AGENTS.md",
	}
	if len(paths) != len(want) {
		t.Fatalf("expected %d paths, got %d (%v)", len(want), len(paths), paths)
	}
	for i, w := range want {
		if paths[i] != w {
			t.Errorf("paths[%d] = %q, want %q", i, paths[i], w)
		}
	}
}

func TestEnsureGitIgnore(t *testing.T) {
	t.Run("creates new gitignore", func(t *testing.T) {
		dir := t.TempDir()
		if err := EnsureGitIgnore(dir); err != nil {
			t.Fatalf("EnsureGitIgnore error: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), CloneDir+"/") {
			t.Error("expected clone dir in .gitignore")
		}
	})

	t.Run("appends to existing", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("node_modules/\n"), 0644)

		if err := EnsureGitIgnore(dir); err != nil {
			t.Fatalf("EnsureGitIgnore error: %v", err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		content := string(data)
		if !strings.Contains(content, "node_modules/") {
			t.Error("existing content should be preserved")
		}
		if !strings.Contains(content, CloneDir+"/") {
			t.Error("expected clone dir appended")
		}
	})

	t.Run("skips duplicate", func(t *testing.T) {
		dir := t.TempDir()
		existing := "# ignore\n" + CloneDir + "/\n"
		os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(existing), 0644)

		if err := EnsureGitIgnore(dir); err != nil {
			t.Fatalf("EnsureGitIgnore error: %v", err)
		}
		data, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
		count := strings.Count(string(data), CloneDir+"/")
		if count != 1 {
			t.Errorf("expected 1 entry, got %d", count)
		}
	})
}

func TestLocalPath(t *testing.T) {
	t.Run("nil context", func(t *testing.T) {
		m := &manifest.Manifest{}
		if got := LocalPath(m); got != "" {
			t.Errorf("expected empty, got %s", got)
		}
	})

	t.Run("custom path", func(t *testing.T) {
		m := &manifest.Manifest{Context: &manifest.ContextConfig{Path: "custom/path"}}
		if got := LocalPath(m); got != "custom/path" {
			t.Errorf("expected custom/path, got %s", got)
		}
	})

	t.Run("default path", func(t *testing.T) {
		m := &manifest.Manifest{Context: &manifest.ContextConfig{}}
		if got := LocalPath(m); got != "docs/context" {
			t.Errorf("expected docs/context, got %s", got)
		}
	})
}

// mockBackend implements vcs.Backend for testing.
type mockBackend struct {
	name       string
	cloneErr   error
	pullErr    error
	hasChanges bool
	addErr     error
	pushErr    error
	calls      []string
}

func (m *mockBackend) Name() string { return m.name }
func (m *mockBackend) SparseClone(ctx context.Context, repoURL, targetDir string, paths []string) error {
	m.calls = append(m.calls, "SparseClone")
	if m.cloneErr != nil {
		return m.cloneErr
	}
	// Create the target directory to simulate a successful clone
	return os.MkdirAll(filepath.Join(targetDir, ".amaru_registry", "context"), 0755)
}
func (m *mockBackend) Pull(ctx context.Context, dir string) error {
	m.calls = append(m.calls, "Pull")
	return m.pullErr
}
func (m *mockBackend) HasChanges(ctx context.Context, dir string) bool {
	m.calls = append(m.calls, "HasChanges")
	return m.hasChanges
}
func (m *mockBackend) Add(ctx context.Context, dir string, paths []string) error {
	m.calls = append(m.calls, "Add")
	return m.addErr
}
func (m *mockBackend) CommitAndPush(ctx context.Context, dir, message string) error {
	m.calls = append(m.calls, "CommitAndPush")
	return m.pushErr
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Registry:  manifest.RegistryConfig{URL: "github:acme/registry", Auth: "none"},
		Project:   "myapp",
		LocalPath: "docs/context",
	}

	backend := &mockBackend{name: "git"}
	err := Init(context.Background(), dir, cfg, backend)
	if err != nil {
		t.Fatalf("Init error: %v", err)
	}

	if len(backend.calls) != 1 || backend.calls[0] != "SparseClone" {
		t.Errorf("expected [SparseClone], got %v", backend.calls)
	}

	// Verify symlink was created
	linkPath := filepath.Join(dir, "docs", "context")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected symlink at %s: %v", linkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("expected symlink")
	}
}

func TestInitAlreadyInitialized(t *testing.T) {
	dir := t.TempDir()
	// Create the clone dir to simulate already initialized
	os.MkdirAll(filepath.Join(dir, CloneDir), 0755)

	cfg := &Config{
		Registry:  manifest.RegistryConfig{URL: "github:acme/registry", Auth: "none"},
		Project:   "myapp",
		LocalPath: "docs/context",
	}

	backend := &mockBackend{name: "git"}
	err := Init(context.Background(), dir, cfg, backend)
	if err == nil {
		t.Error("expected error for already initialized")
	}
}

func TestInitSaplingPaths(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{
		Registry:  manifest.RegistryConfig{URL: "github:acme/registry", Auth: "none"},
		Project:   "myapp",
		LocalPath: "docs/context",
	}

	backend := &mockBackend{name: "sapling"}
	Init(context.Background(), dir, cfg, backend)

	// Sapling backend should have been called
	if len(backend.calls) != 1 || backend.calls[0] != "SparseClone" {
		t.Errorf("expected [SparseClone], got %v", backend.calls)
	}
}

func TestSync(t *testing.T) {
	dir := t.TempDir()
	// Create clone dir to simulate initialized state
	os.MkdirAll(filepath.Join(dir, CloneDir), 0755)

	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git"}

	err := Sync(context.Background(), dir, cfg, backend)
	if err != nil {
		t.Fatalf("Sync error: %v", err)
	}
	if len(backend.calls) != 1 || backend.calls[0] != "Pull" {
		t.Errorf("expected [Pull], got %v", backend.calls)
	}
}

func TestSyncNotInitialized(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git"}

	err := Sync(context.Background(), dir, cfg, backend)
	if err == nil {
		t.Error("expected error for not initialized")
	}
}

func TestPush(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, CloneDir), 0755)

	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git", hasChanges: true}

	err := Push(context.Background(), dir, cfg, backend, "test commit")
	if err != nil {
		t.Fatalf("Push error: %v", err)
	}
	if len(backend.calls) != 3 {
		t.Fatalf("expected 3 calls, got %v", backend.calls)
	}
	if backend.calls[0] != "HasChanges" || backend.calls[1] != "Add" || backend.calls[2] != "CommitAndPush" {
		t.Errorf("unexpected call sequence: %v", backend.calls)
	}
}

func TestPushNoChanges(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, CloneDir), 0755)

	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git", hasChanges: false}

	err := Push(context.Background(), dir, cfg, backend, "")
	if err != nil {
		t.Fatalf("Push error: %v", err)
	}
	if len(backend.calls) != 1 || backend.calls[0] != "HasChanges" {
		t.Errorf("expected only HasChanges call, got %v", backend.calls)
	}
}

func TestPushNotInitialized(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Project: "myapp"}
	backend := &mockBackend{name: "git"}

	err := Push(context.Background(), dir, cfg, backend, "")
	if err == nil {
		t.Error("expected error for not initialized")
	}
}
