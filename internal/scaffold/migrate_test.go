package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/useamaru/amaru/internal/registry"
)

// writeLegacyRegistry writes a minimal v1 (nested) layout into dir.
// It does NOT create the journal file.
func writeLegacyRegistry(t *testing.T, dir string) {
	t.Helper()
	idx := &registry.RegistryIndex{
		AmaruVersion: "1",
		Skills: map[string]registry.RegistryEntry{
			"foo": {Description: "test"},
		},
		Commands:  map[string]registry.RegistryEntry{},
		Agents:    map[string]registry.RegistryEntry{},
		Skillsets: map[string]registry.SkillsetEntry{},
	}
	if err := SaveLocalIndex(dir, idx); err != nil {
		t.Fatalf("saving legacy index: %v", err)
	}
	mustMkdirAll(t, filepath.Join(dir, ".amaru_registry", "skills", "foo"))
	mustWriteFile(t, filepath.Join(dir, ".amaru_registry", "skills", "foo", "skill.md"), "# foo")
	mustMkdirAll(t, filepath.Join(dir, ".amaru_registry", "commands"))
	mustMkdirAll(t, filepath.Join(dir, ".amaru_registry", "agents"))
	mustMkdirAll(t, filepath.Join(dir, ".amaru_registry", "context", "myapp"))
	mustMkdirAll(t, filepath.Join(dir, ".amaru_registry", ".sparse-profiles"))
}

// writeFlatRegistry writes a minimal v2 (flat) layout into dir.
func writeFlatRegistry(t *testing.T, dir string) {
	t.Helper()
	idx := &registry.RegistryIndex{
		AmaruVersion: "2",
		Skills:       map[string]registry.RegistryEntry{"foo": {Description: "test"}},
		Commands:     map[string]registry.RegistryEntry{},
		Agents:       map[string]registry.RegistryEntry{},
		Skillsets:    map[string]registry.SkillsetEntry{},
	}
	if err := SaveLocalIndex(dir, idx); err != nil {
		t.Fatalf("saving flat index: %v", err)
	}
	mustMkdirAll(t, filepath.Join(dir, "skills", "foo"))
	mustMkdirAll(t, filepath.Join(dir, "commands"))
	mustMkdirAll(t, filepath.Join(dir, "agents"))
}

func mustMkdirAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatalf("MkdirAll %s: %v", p, err)
	}
}

func mustWriteFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile %s: %v", p, err)
	}
}

// ---- Crash-recovery matrix ------------------------------------------------

func TestMigrateInPlace_AlreadyMigrated(t *testing.T) {
	dir := t.TempDir()
	writeFlatRegistry(t, dir)

	res, err := MigrateInPlace(dir, MigrateOptions{})
	if err != nil {
		t.Fatalf("expected no-op success, got error: %v", err)
	}
	if res.Status != MigrationStatusAlreadyMigrated {
		t.Errorf("status = %v, want AlreadyMigrated", res.Status)
	}
}

func TestMigrateInPlace_NoLayoutDetected(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "amaru_registry.json"),
		`{"amaru_version":"1","skills":{},"commands":{},"agents":{},"skillsets":{}}`+"\n")

	_, err := MigrateInPlace(dir, MigrateOptions{})
	if err == nil {
		t.Fatal("expected error for no layout detected, got nil")
	}
	if !strings.Contains(err.Error(), "no registry layout detected") {
		t.Errorf("error = %v, want 'no registry layout detected'", err)
	}
}

func TestMigrateInPlace_BothLayoutsPresent(t *testing.T) {
	dir := t.TempDir()
	writeLegacyRegistry(t, dir)
	mustMkdirAll(t, filepath.Join(dir, "skills"))

	_, err := MigrateInPlace(dir, MigrateOptions{})
	if err == nil {
		t.Fatal("expected error for both layouts, got nil")
	}
	if !strings.Contains(err.Error(), "both legacy") {
		t.Errorf("error should mention both layouts: %v", err)
	}
}

func TestMigrateInPlace_JournalAndBoth_HalfMigrated(t *testing.T) {
	dir := t.TempDir()
	writeLegacyRegistry(t, dir)
	mustMkdirAll(t, filepath.Join(dir, "skills"))
	mustWriteFile(t, filepath.Join(dir, JournalFile), `{"started_at":"now"}`+"\n")

	_, err := MigrateInPlace(dir, MigrateOptions{})
	if err == nil {
		t.Fatal("expected error for half-migrated, got nil")
	}
	if !strings.Contains(err.Error(), "did not complete") {
		t.Errorf("error should mention crashed run: %v", err)
	}
}

func TestMigrateInPlace_JournalOnlyLegacy_CrashedBeforeAnyMove(t *testing.T) {
	dir := t.TempDir()
	writeLegacyRegistry(t, dir)
	mustWriteFile(t, filepath.Join(dir, JournalFile), `{"started_at":"now"}`+"\n")

	_, err := MigrateInPlace(dir, MigrateOptions{})
	if err == nil {
		t.Fatal("expected error for stale journal, got nil")
	}
	if !strings.Contains(err.Error(), "crashed before any move") {
		t.Errorf("error should mention crashed-before-move: %v", err)
	}
}

func TestMigrateInPlace_JournalAndOnlyFlat_StaleJournal(t *testing.T) {
	dir := t.TempDir()
	writeFlatRegistry(t, dir)
	mustWriteFile(t, filepath.Join(dir, JournalFile), `{"started_at":"now"}`+"\n")

	_, err := MigrateInPlace(dir, MigrateOptions{})
	if err == nil {
		t.Fatal("expected error for stale journal, got nil")
	}
	if !strings.Contains(err.Error(), "stale") {
		t.Errorf("error should mention stale journal: %v", err)
	}
}

// ---- Conflict detection ---------------------------------------------------

func TestMigrateInPlace_RejectsSymlinkAtNestedDir(t *testing.T) {
	dir := t.TempDir()
	writeLegacyRegistry(t, dir)
	// Replace .amaru_registry/skills with a symlink to test detection.
	skillsPath := filepath.Join(dir, ".amaru_registry", "skills")
	if err := os.RemoveAll(skillsPath); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "elsewhere")
	mustMkdirAll(t, target)
	if err := os.Symlink(target, skillsPath); err != nil {
		t.Skip("symlink not supported on this filesystem")
	}

	_, err := MigrateInPlace(dir, MigrateOptions{})
	if err == nil {
		t.Fatal("expected symlink rejection")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("expected error to mention symlink: %v", err)
	}
}

func TestMigrateInPlace_StrictChildrenRejectsExtras(t *testing.T) {
	dir := t.TempDir()
	writeLegacyRegistry(t, dir)
	// Add a non-canonical child.
	mustWriteFile(t, filepath.Join(dir, ".amaru_registry", ".DS_Store"), "junk")

	_, err := MigrateInPlace(dir, MigrateOptions{StrictChildren: true})
	if err == nil {
		t.Fatal("expected --strict-children to reject extras")
	}
	if !strings.Contains(err.Error(), "non-canonical") {
		t.Errorf("expected non-canonical error: %v", err)
	}
}

func TestMigrateInPlace_NonStrictMovesExtrasVerbatim(t *testing.T) {
	dir := t.TempDir()
	writeLegacyRegistry(t, dir)
	mustWriteFile(t, filepath.Join(dir, ".amaru_registry", ".DS_Store"), "junk")

	res, err := MigrateInPlace(dir, MigrateOptions{})
	if err != nil {
		t.Fatalf("expected non-strict success, got: %v", err)
	}
	if res.Status != MigrationStatusCompleted {
		t.Errorf("status = %v", res.Status)
	}
	if _, err := os.Stat(filepath.Join(dir, ".DS_Store")); err != nil {
		t.Errorf("non-canonical child should have been moved verbatim: %v", err)
	}
	if len(res.NonCanonicalMoves) != 1 || res.NonCanonicalMoves[0].To != ".DS_Store" {
		t.Errorf("expected one non-canonical move for .DS_Store, got %v", res.NonCanonicalMoves)
	}
}

// ---- Happy path -----------------------------------------------------------

func TestMigrateInPlace_Happy(t *testing.T) {
	dir := t.TempDir()
	writeLegacyRegistry(t, dir)
	// Add a sparse profile that should be rewritten.
	profilePath := filepath.Join(dir, ".amaru_registry", ".sparse-profiles", "myapp")
	mustWriteFile(t, profilePath,
		"# Sapling sparse profile\n[include]\n.amaru_registry/context/myapp/**\nAGENTS.md\n[exclude]\n*\n")

	res, err := MigrateInPlace(dir, MigrateOptions{})
	if err != nil {
		t.Fatalf("MigrateInPlace error: %v", err)
	}
	if res.Status != MigrationStatusCompleted {
		t.Errorf("status = %v, want Completed", res.Status)
	}
	if !res.IndexBumped {
		t.Error("expected IndexBumped = true")
	}

	// Flat dirs must exist.
	for _, d := range []string{"skills/foo", "commands", "agents", "context/myapp", ".sparse-profiles"} {
		if _, err := os.Stat(filepath.Join(dir, d)); err != nil {
			t.Errorf("expected %s after migration: %v", d, err)
		}
	}
	// Legacy dir must be gone.
	if _, err := os.Stat(filepath.Join(dir, ".amaru_registry")); !os.IsNotExist(err) {
		t.Errorf(".amaru_registry/ should be removed: err=%v", err)
	}
	// Index must be v2.
	idx, err := LoadLocalIndex(dir)
	if err != nil {
		t.Fatalf("LoadLocalIndex: %v", err)
	}
	if idx.AmaruVersion != "2" {
		t.Errorf("AmaruVersion = %q, want \"2\"", idx.AmaruVersion)
	}
	// Sparse profile must be rewritten with flat path and a .bak alongside.
	body, err := os.ReadFile(filepath.Join(dir, ".sparse-profiles", "myapp"))
	if err != nil {
		t.Fatalf("reading rewritten profile: %v", err)
	}
	if strings.Contains(string(body), ".amaru_registry/") {
		t.Errorf("rewritten profile still contains .amaru_registry/:\n%s", body)
	}
	if !strings.Contains(string(body), "context/myapp/") {
		t.Errorf("rewritten profile missing flat context path:\n%s", body)
	}
	bak, err := os.ReadFile(filepath.Join(dir, ".sparse-profiles", "myapp.bak"))
	if err != nil {
		t.Fatalf("expected .bak file: %v", err)
	}
	if !strings.Contains(string(bak), ".amaru_registry/context/myapp") {
		t.Errorf(".bak should preserve original body:\n%s", bak)
	}
	// Journal must be removed.
	if _, err := os.Stat(filepath.Join(dir, JournalFile)); !os.IsNotExist(err) {
		t.Errorf("journal should be removed after success: err=%v", err)
	}
}

func TestMigrateInPlace_DryRun(t *testing.T) {
	dir := t.TempDir()
	writeLegacyRegistry(t, dir)

	res, err := MigrateInPlace(dir, MigrateOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run error: %v", err)
	}
	if res.Status != MigrationStatusDryRun {
		t.Errorf("status = %v, want DryRun", res.Status)
	}
	if len(res.PlannedMoves) == 0 {
		t.Error("expected planned moves in dry-run result")
	}
	// Filesystem must be unchanged.
	if _, err := os.Stat(filepath.Join(dir, ".amaru_registry", "skills")); err != nil {
		t.Errorf("dry-run should not move legacy dirs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills")); !os.IsNotExist(err) {
		t.Errorf("dry-run should not create flat dirs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, JournalFile)); !os.IsNotExist(err) {
		t.Errorf("dry-run should not write journal: %v", err)
	}
}

func TestMigrateInPlace_SkipSparseRewrite(t *testing.T) {
	dir := t.TempDir()
	writeLegacyRegistry(t, dir)
	profilePath := filepath.Join(dir, ".amaru_registry", ".sparse-profiles", "myapp")
	mustWriteFile(t, profilePath, "[include]\n.amaru_registry/context/myapp/**\n")

	res, err := MigrateInPlace(dir, MigrateOptions{SkipSparseRewrite: true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Status != MigrationStatusCompleted {
		t.Fatal("expected completion")
	}
	body, _ := os.ReadFile(filepath.Join(dir, ".sparse-profiles", "myapp"))
	if !strings.Contains(string(body), ".amaru_registry/context/myapp") {
		t.Errorf("--skip-sparse-rewrite should leave profile body intact:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(dir, ".sparse-profiles", "myapp.bak")); !os.IsNotExist(err) {
		t.Errorf("--skip-sparse-rewrite should not write a .bak: %v", err)
	}
}

// ---- Sparse-profile rewrite algorithm -------------------------------------

func TestRewriteSparseProfileBody_AnchoredPrefix(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{
			"include line",
			"[include]\n.amaru_registry/context/foo/**\n",
			"[include]\ncontext/foo/**\n",
			true,
		},
		{
			"negation line",
			"!.amaru_registry/context/foo/secrets/**\n",
			"!context/foo/secrets/**\n",
			true,
		},
		{
			"negation with space after bang",
			"! .amaru_registry/context/foo/**\n",
			"! context/foo/**\n",
			true,
		},
		{
			"comment with the prefix is preserved",
			"# this used to be at .amaru_registry/context/foo/\n",
			"# this used to be at .amaru_registry/context/foo/\n",
			false,
		},
		{
			"non-prefix occurrence is preserved",
			"some/path/.amaru_registry/context/foo/**\n",
			"some/path/.amaru_registry/context/foo/**\n",
			false,
		},
		{
			"leading whitespace before pattern",
			"    .amaru_registry/context/foo/**\n",
			"    context/foo/**\n",
			true,
		},
		{
			"unrelated lines untouched",
			"AGENTS.md\namaru_registry.json\n",
			"AGENTS.md\namaru_registry.json\n",
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := rewriteSparseProfileBody(tt.input)
			if got != tt.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tt.want)
			}
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
		})
	}
}

// ---- LoadLocalIndex re-read after migration -------------------------------

func TestMigrateInPlace_PostMigrationLoadAndLayout(t *testing.T) {
	dir := t.TempDir()
	writeLegacyRegistry(t, dir)

	if _, err := MigrateInPlace(dir, MigrateOptions{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	idx, err := LoadLocalIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := registry.LayoutFor(idx)
	if err != nil {
		t.Fatal(err)
	}
	if layout != registry.LayoutFlat {
		t.Errorf("post-migration layout = %v, want flat", layout)
	}
}
