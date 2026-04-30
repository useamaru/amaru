package registry

import (
	"path/filepath"
	"testing"

	"github.com/useamaru/amaru/internal/types"
)

func TestLayoutFor_KnownVersions(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    Layout
	}{
		{"empty defaults to nested", "", LayoutNested},
		{"explicit 1 is nested", "1", LayoutNested},
		{"explicit 2 is flat", "2", LayoutFlat},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := &RegistryIndex{AmaruVersion: tt.version}
			got, err := LayoutFor(idx)
			if err != nil {
				t.Fatalf("LayoutFor(%q) returned error: %v", tt.version, err)
			}
			if got != tt.want {
				t.Errorf("LayoutFor(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestLayoutFor_UnknownVersionErrors(t *testing.T) {
	tests := []string{"99", "0", "v2", "two", "1.0"}
	for _, v := range tests {
		t.Run(v, func(t *testing.T) {
			idx := &RegistryIndex{AmaruVersion: v}
			_, err := LayoutFor(idx)
			if err == nil {
				t.Errorf("LayoutFor(%q) expected error, got nil", v)
			}
		})
	}
}

func TestLayout_IsLegacy(t *testing.T) {
	if !LayoutNested.IsLegacy() {
		t.Error("LayoutNested.IsLegacy() should be true")
	}
	if LayoutFlat.IsLegacy() {
		t.Error("LayoutFlat.IsLegacy() should be false")
	}
}

func TestLayout_String(t *testing.T) {
	cases := map[Layout]string{
		LayoutNested: "nested",
		LayoutFlat:   "flat",
		Layout(99):   "unknown",
	}
	for l, want := range cases {
		if got := l.String(); got != want {
			t.Errorf("Layout(%d).String() = %q, want %q", l, got, want)
		}
	}
}

func TestLayout_ItemDir(t *testing.T) {
	root := "/tmp/reg"
	tests := []struct {
		name     string
		layout   Layout
		itemType types.ItemType
		item     string
		want     string
	}{
		{"nested skill", LayoutNested, types.Skill, "research", filepath.Join(root, ".amaru_registry", "skills", "research")},
		{"nested command", LayoutNested, types.Command, "deploy", filepath.Join(root, ".amaru_registry", "commands", "deploy")},
		{"nested agent", LayoutNested, types.Agent, "reviewer", filepath.Join(root, ".amaru_registry", "agents", "reviewer")},
		{"flat skill", LayoutFlat, types.Skill, "research", filepath.Join(root, "skills", "research")},
		{"flat command", LayoutFlat, types.Command, "deploy", filepath.Join(root, "commands", "deploy")},
		{"flat agent", LayoutFlat, types.Agent, "reviewer", filepath.Join(root, "agents", "reviewer")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.layout.ItemDir(root, tt.itemType, tt.item)
			if got != tt.want {
				t.Errorf("ItemDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLayout_TypeDir(t *testing.T) {
	root := "/tmp/reg"
	if got := LayoutNested.TypeDir(root, types.Skill); got != filepath.Join(root, ".amaru_registry", "skills") {
		t.Errorf("nested TypeDir(skill) = %q", got)
	}
	if got := LayoutFlat.TypeDir(root, types.Skill); got != filepath.Join(root, "skills") {
		t.Errorf("flat TypeDir(skill) = %q", got)
	}
}

func TestLayout_ContextDir(t *testing.T) {
	root := "/tmp/reg"
	if got := LayoutNested.ContextDir(root, "myapp"); got != filepath.Join(root, ".amaru_registry", "context", "myapp") {
		t.Errorf("nested ContextDir = %q", got)
	}
	if got := LayoutFlat.ContextDir(root, "myapp"); got != filepath.Join(root, "context", "myapp") {
		t.Errorf("flat ContextDir = %q", got)
	}
}

func TestLayout_SparseProfilePath(t *testing.T) {
	root := "/tmp/reg"
	if got := LayoutNested.SparseProfilePath(root, "myapp"); got != filepath.Join(root, ".amaru_registry", ".sparse-profiles", "myapp") {
		t.Errorf("nested SparseProfilePath = %q", got)
	}
	if got := LayoutFlat.SparseProfilePath(root, "myapp"); got != filepath.Join(root, ".sparse-profiles", "myapp") {
		t.Errorf("flat SparseProfilePath = %q", got)
	}
}

func TestLayout_SkillsetManifestPath(t *testing.T) {
	root := "/tmp/reg"
	if got := LayoutNested.SkillsetManifestPath(root, "starter-pack"); got != filepath.Join(root, ".amaru_registry", "skillsets", "starter-pack", "manifest.json") {
		t.Errorf("nested SkillsetManifestPath = %q", got)
	}
	if got := LayoutFlat.SkillsetManifestPath(root, "starter-pack"); got != filepath.Join(root, "skillsets", "starter-pack", "manifest.json") {
		t.Errorf("flat SkillsetManifestPath = %q", got)
	}
}

func TestLayout_RelativeItemPath(t *testing.T) {
	tests := []struct {
		name     string
		layout   Layout
		itemType types.ItemType
		item     string
		want     string
	}{
		{"nested uses forward slashes", LayoutNested, types.Skill, "research", ".amaru_registry/skills/research"},
		{"flat has no prefix", LayoutFlat, types.Skill, "research", "skills/research"},
		{"flat command", LayoutFlat, types.Command, "deploy", "commands/deploy"},
		{"flat agent", LayoutFlat, types.Agent, "reviewer", "agents/reviewer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.layout.RelativeItemPath(tt.itemType, tt.item)
			if got != tt.want {
				t.Errorf("RelativeItemPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLayout_RelativeSkillsetManifestPath(t *testing.T) {
	if got := LayoutNested.RelativeSkillsetManifestPath("pack"); got != ".amaru_registry/skillsets/pack/manifest.json" {
		t.Errorf("nested = %q", got)
	}
	if got := LayoutFlat.RelativeSkillsetManifestPath("pack"); got != "skillsets/pack/manifest.json" {
		t.Errorf("flat = %q", got)
	}
}

func TestLayout_RelativeContextPath(t *testing.T) {
	if got := LayoutNested.RelativeContextPath("myapp"); got != ".amaru_registry/context/myapp" {
		t.Errorf("nested = %q", got)
	}
	if got := LayoutFlat.RelativeContextPath("myapp"); got != "context/myapp" {
		t.Errorf("flat = %q", got)
	}
}

func TestRegistryIndex_LayoutVersion(t *testing.T) {
	tests := []struct {
		version string
		want    int
		wantErr bool
	}{
		{"", 1, false},
		{"1", 1, false},
		{"2", 2, false},
		{"99", 0, true},
		{"abc", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			idx := &RegistryIndex{AmaruVersion: tt.version}
			got, err := idx.LayoutVersion()
			if tt.wantErr {
				if err == nil {
					t.Errorf("LayoutVersion(%q) expected error, got %d", tt.version, got)
				}
				return
			}
			if err != nil {
				t.Errorf("LayoutVersion(%q) unexpected error: %v", tt.version, err)
			}
			if got != tt.want {
				t.Errorf("LayoutVersion(%q) = %d, want %d", tt.version, got, tt.want)
			}
		})
	}
}
