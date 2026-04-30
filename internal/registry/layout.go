package registry

import (
	"path/filepath"

	"github.com/useamaru/amaru/internal/types"
)

// Layout encodes the on-disk arrangement of a registry repository.
// It is the path-math half of the (layout, source) pair tracked alongside
// each loaded RegistryIndex. The other half — whether the index was parsed
// from amaru_registry.json or synthesized from a directory walk — lives in
// the registry client and does not affect path computation.
type Layout int

const (
	// LayoutNested is the legacy layout where installable content lives under
	// .amaru_registry/{skills,commands,agents,context,.sparse-profiles}/.
	LayoutNested Layout = 1
	// LayoutFlat is the v2 layout where the same directories live at the
	// repository root.
	LayoutFlat Layout = 2
)

// NestedRoot is the directory prefix used by LayoutNested. Centralized here
// so the legacy string lives in exactly one place.
const NestedRoot = ".amaru_registry"

// LayoutFor resolves the layout encoded in a registry index.
// Empty/missing AmaruVersion defaults to nested for back-compat.
func LayoutFor(idx *RegistryIndex) (Layout, error) {
	v, err := idx.LayoutVersion()
	if err != nil {
		return 0, err
	}
	return Layout(v), nil
}

// IsLegacy reports whether the layout uses the .amaru_registry/ prefix.
func (l Layout) IsLegacy() bool {
	return l == LayoutNested
}

// String returns a human-readable name.
func (l Layout) String() string {
	switch l {
	case LayoutNested:
		return "nested"
	case LayoutFlat:
		return "flat"
	default:
		return "unknown"
	}
}

// contentRoot returns the directory under which installable content lives
// for this layout, joined with the registry root.
func (l Layout) contentRoot(root string) string {
	if l.IsLegacy() {
		return filepath.Join(root, NestedRoot)
	}
	return root
}

// ItemDir returns the absolute path to a skill/command/agent directory.
func (l Layout) ItemDir(root string, itemType types.ItemType, name string) string {
	return filepath.Join(l.contentRoot(root), itemType.DirName(), name)
}

// TypeDir returns the absolute path to the directory holding all items of a type.
func (l Layout) TypeDir(root string, itemType types.ItemType) string {
	return filepath.Join(l.contentRoot(root), itemType.DirName())
}

// ContextDir returns the absolute path to the per-project context directory.
func (l Layout) ContextDir(root, project string) string {
	return filepath.Join(l.contentRoot(root), "context", project)
}

// SparseProfilePath returns the absolute path to a project's sparse profile file.
func (l Layout) SparseProfilePath(root, project string) string {
	return filepath.Join(l.contentRoot(root), ".sparse-profiles", project)
}

// SparseProfilesDir returns the absolute path to the .sparse-profiles directory.
func (l Layout) SparseProfilesDir(root string) string {
	return filepath.Join(l.contentRoot(root), ".sparse-profiles")
}

// SkillsetManifestPath returns the absolute path to a skillset's manifest.json.
func (l Layout) SkillsetManifestPath(root, name string) string {
	return filepath.Join(l.contentRoot(root), "skillsets", name, "manifest.json")
}

// RelativeContentRoot returns the layout's content prefix as a forward-slash
// relative path, suitable for use in registry-internal references like
// sparse profile bodies and remote API paths. Returns "" for the flat layout.
func (l Layout) RelativeContentRoot() string {
	if l.IsLegacy() {
		return NestedRoot
	}
	return ""
}

// RelativeItemPath returns the registry-internal slash-separated path to
// an item directory (no root prefix). Suitable for the GitHub Contents API.
func (l Layout) RelativeItemPath(itemType types.ItemType, name string) string {
	parts := []string{}
	if r := l.RelativeContentRoot(); r != "" {
		parts = append(parts, r)
	}
	parts = append(parts, itemType.DirName(), name)
	return joinSlash(parts...)
}

// RelativeSkillsetManifestPath returns the registry-internal slash-separated
// path to a skillset's manifest.json.
func (l Layout) RelativeSkillsetManifestPath(name string) string {
	parts := []string{}
	if r := l.RelativeContentRoot(); r != "" {
		parts = append(parts, r)
	}
	parts = append(parts, "skillsets", name, "manifest.json")
	return joinSlash(parts...)
}

// RelativeContextPath returns the registry-internal slash-separated path to
// a project's context directory.
func (l Layout) RelativeContextPath(project string) string {
	parts := []string{}
	if r := l.RelativeContentRoot(); r != "" {
		parts = append(parts, r)
	}
	parts = append(parts, "context", project)
	return joinSlash(parts...)
}

// joinSlash joins path components with forward slashes regardless of OS.
func joinSlash(parts ...string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "/"
		}
		out += p
	}
	return out
}
