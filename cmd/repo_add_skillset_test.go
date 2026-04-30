package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/scaffold"
)

func TestRepoAddSkillset_ParsesCrossRegistrySyntax(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "research")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skillset"
	repoAddDescription = "test"
	repoAddAuthor = ""
	repoAddTags = ""
	// Mix: same-registry member (research) and a cross-registry member (deploy@platform).
	repoAddItems = "skill/research,command/deploy@platform"

	if err := runRepoAdd("mixed-pack"); err != nil {
		t.Fatalf("runRepoAdd err: %v", err)
	}

	idx, _ := scaffold.LoadLocalIndex(dir)
	ss, ok := idx.Skillsets["mixed-pack"]
	if !ok {
		t.Fatal("mixed-pack not in skillsets")
	}
	if len(ss.Items) != 2 {
		t.Fatalf("got %d items, want 2: %+v", len(ss.Items), ss.Items)
	}

	// Same-registry item must have empty Registry (defaults to home).
	var local, cross registry.SkillsetItem
	for _, it := range ss.Items {
		switch it.Name {
		case "research":
			local = it
		case "deploy":
			cross = it
		}
	}
	if local.Registry != "" {
		t.Errorf("same-registry member should have empty Registry, got %q", local.Registry)
	}
	if cross.Registry != "platform" {
		t.Errorf("cross-registry member Registry = %q, want \"platform\"", cross.Registry)
	}
	if cross.Type != "command" {
		t.Errorf("cross member Type = %q", cross.Type)
	}
}

func TestRepoAddSkillset_RejectsTrailingAt(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "research")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skillset"
	repoAddDescription = ""
	repoAddAuthor = ""
	repoAddTags = ""
	repoAddItems = "skill/research@"

	err := runRepoAdd("bad-pack")
	if err == nil {
		t.Fatal("expected error for trailing '@'")
	}
	if !strings.Contains(err.Error(), "trailing '@'") {
		t.Errorf("error should mention trailing '@': %v", err)
	}
}

func TestRepoAddSkillset_CrossRegistrySkipsLocalExistenceCheck(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "research")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skillset"
	repoAddDescription = ""
	repoAddAuthor = ""
	repoAddTags = ""
	// member "lives-elsewhere" doesn't exist in this registry, but @platform
	// signals cross-registry so the local-existence check must be skipped.
	repoAddItems = "skill/lives-elsewhere@platform"

	if err := runRepoAdd("borrows-pack"); err != nil {
		t.Fatalf("cross-registry add should succeed without local existence check: %v", err)
	}

	idx, _ := scaffold.LoadLocalIndex(dir)
	ss := idx.Skillsets["borrows-pack"]
	if len(ss.Items) != 1 || ss.Items[0].Registry != "platform" {
		t.Errorf("got items = %+v", ss.Items)
	}
}

func TestRepoAddSkillset_SameRegistryStillRequiresMember(t *testing.T) {
	dir := scaffoldTestRegistryWithSkill(t, "research")
	origDir, _ := os.Getwd()
	defer os.Chdir(origDir)
	os.Chdir(dir)

	repoAddType = "skillset"
	repoAddDescription = ""
	repoAddAuthor = ""
	repoAddTags = ""
	// No "@" suffix → must exist locally.
	repoAddItems = "skill/missing"

	err := runRepoAdd("bad-pack")
	if err == nil {
		t.Fatal("expected error for missing same-registry member")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention not found: %v", err)
	}
}
