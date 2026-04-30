package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/scaffold"
)

func TestNextPatchVersion(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "0.1.0", false},
		{"0.0.0", "0.0.1", false},
		{"1.0.0", "1.0.1", false},
		{"2.3.9", "2.3.10", false},
		{"v1.0.0", "1.0.1", false}, // semver lib accepts the v prefix
		{"not-semver", "", true},
		{"1", "1.0.1", false}, // semver lib expands "1" → "1.0.0"
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := nextPatchVersion(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Errorf("nextPatchVersion(%q) expected error, got %q", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("nextPatchVersion(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("nextPatchVersion(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSkillsetsContaining(t *testing.T) {
	idx := &registry.RegistryIndex{
		Skillsets: map[string]registry.SkillsetEntry{
			"starter": {
				Items: []registry.SkillsetItem{
					{Type: "skill", Name: "research"},
					{Type: "command", Name: "deploy"},
				},
			},
			"power-pack": {
				Items: []registry.SkillsetItem{
					{Type: "skill", Name: "research"},
					{Type: "agent", Name: "reviewer"},
				},
			},
			"unrelated": {
				Items: []registry.SkillsetItem{
					{Type: "skill", Name: "lint"},
				},
			},
		},
	}

	got := idx.SkillsetsContaining("skill", "research")
	sort.Strings(got)
	want := []string{"power-pack", "starter"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("SkillsetsContaining(skill, research) = %v, want %v", got, want)
	}

	if hits := idx.SkillsetsContaining("skill", "missing"); len(hits) != 0 {
		t.Errorf("expected no hits for missing skill, got %v", hits)
	}

	// Type/name combination must both match — same name under different type doesn't hit.
	if hits := idx.SkillsetsContaining("agent", "research"); len(hits) != 0 {
		t.Errorf("expected no hits for agent/research (it's a skill), got %v", hits)
	}
}

// gitInit prepares dir as a minimal git repo with a configured user so
// runRepoTag's git invocations succeed without a real ~/.gitconfig.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q", "--initial-branch=main"},
		{"config", "user.email", "tester@example.com"},
		{"config", "user.name", "Tester"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	// An initial commit so HEAD exists for tag operations.
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initial commit: %v (%s)", err, out)
	}
}

func TestRepoTag_CascadePatchBumpsSkillsets(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "research")
	// Add a skillset containing the skill, with a known starting Latest.
	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skillsets["starter"] = registry.SkillsetEntry{
		Latest:      "1.0.0",
		Description: "test pack",
		Items:       []registry.SkillsetItem{{Type: "skill", Name: "research"}},
	}
	idx.Skillsets["unrelated"] = registry.SkillsetEntry{
		Latest:      "2.5.0",
		Description: "other pack",
		Items:       []registry.SkillsetItem{{Type: "skill", Name: "other"}},
	}
	if err := scaffold.SaveLocalIndex(dir, idx); err != nil {
		t.Fatal(err)
	}

	gitInit(t, dir)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Reset/seed flags.
	repoTagType = "skill"
	repoTagNote = ""
	repoTagPush = false
	repoTagCascade = true
	defer func() { repoTagCascade = false }()

	if err := runRepoTag("research", "1.0.0"); err != nil {
		t.Fatalf("runRepoTag err: %v", err)
	}

	// Re-load and assert the cascade landed.
	out, err := scaffold.LoadLocalIndex(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Skills["research"].Latest; got != "1.0.0" {
		t.Errorf("research.Latest = %q, want 1.0.0", got)
	}
	if got := out.Skillsets["starter"].Latest; got != "1.0.1" {
		t.Errorf("starter cascade should land 1.0.0 → 1.0.1, got %q", got)
	}
	if got := out.Skillsets["unrelated"].Latest; got != "2.5.0" {
		t.Errorf("unrelated should not bump (skill not a member), got %q", got)
	}

	// Git tag for the item itself must exist.
	cmd := exec.Command("git", "tag", "-l", "skill/research/1.0.0")
	cmd.Dir = dir
	tagOut, err := cmd.Output()
	if err != nil {
		t.Fatalf("git tag -l: %v", err)
	}
	if filepath.Base(string(tagOut[:len(tagOut)-1])) != "1.0.0" {
		t.Errorf("expected per-item git tag, got %q", tagOut)
	}
}

func TestRepoTag_CascadeWithoutLatestUses010(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "research")
	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skillsets["fresh"] = registry.SkillsetEntry{
		// No Latest — first cascade should set 0.1.0
		Description: "freshly-created pack",
		Items:       []registry.SkillsetItem{{Type: "skill", Name: "research"}},
	}
	scaffold.SaveLocalIndex(dir, idx)

	gitInit(t, dir)
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoTagType = "skill"
	repoTagNote = ""
	repoTagPush = false
	repoTagCascade = true
	defer func() { repoTagCascade = false }()

	if err := runRepoTag("research", "1.0.0"); err != nil {
		t.Fatalf("err: %v", err)
	}

	out, _ := scaffold.LoadLocalIndex(dir)
	if got := out.Skillsets["fresh"].Latest; got != "0.1.0" {
		t.Errorf("first-cascade Latest = %q, want 0.1.0", got)
	}
}

func TestRepoTag_CascadeAbortsOnInvalidSkillsetVersion(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "research")
	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skillsets["bad"] = registry.SkillsetEntry{
		Latest:      "garbage",
		Description: "malformed pack",
		Items:       []registry.SkillsetItem{{Type: "skill", Name: "research"}},
	}
	scaffold.SaveLocalIndex(dir, idx)
	gitInit(t, dir)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoTagType = "skill"
	repoTagNote = ""
	repoTagPush = false
	repoTagCascade = true
	defer func() { repoTagCascade = false }()

	err := runRepoTag("research", "1.0.0")
	if err == nil {
		t.Fatal("expected error for invalid skillset Latest, got nil")
	}

	// Pre-write validation must have prevented any disk mutation:
	// research.Latest stays empty and skillset Latest stays "garbage".
	out, _ := scaffold.LoadLocalIndex(dir)
	if got := out.Skills["research"].Latest; got != "" {
		t.Errorf("research.Latest leaked despite cascade error: %q", got)
	}
	if got := out.Skillsets["bad"].Latest; got != "garbage" {
		t.Errorf("skillset Latest mutated despite cascade error: %q", got)
	}
}

func TestRepoTag_NoCascadeFlagLeavesSkillsetsAlone(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "research")
	idx, _ := scaffold.LoadLocalIndex(dir)
	idx.Skillsets["starter"] = registry.SkillsetEntry{
		Latest: "1.0.0",
		Items:  []registry.SkillsetItem{{Type: "skill", Name: "research"}},
	}
	scaffold.SaveLocalIndex(dir, idx)
	gitInit(t, dir)

	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoTagType = "skill"
	repoTagNote = ""
	repoTagPush = false
	repoTagCascade = false // explicit: no cascade

	if err := runRepoTag("research", "1.0.0"); err != nil {
		t.Fatalf("runRepoTag err: %v", err)
	}

	out, _ := scaffold.LoadLocalIndex(dir)
	if got := out.Skillsets["starter"].Latest; got != "1.0.0" {
		t.Errorf("starter Latest = %q, want unchanged 1.0.0", got)
	}
}
