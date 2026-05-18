package registry

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/useamaru/amaru/internal/types"
)

// ItemSubPath returns the registry-internal subpath for an item, given an
// optional folder. Empty folder yields just the name; otherwise it joins
// folder/name with a forward slash. Use it to compute the argument passed to
// Layout path methods (ItemDir, RelativeItemPath) when an entry declares a
// Folder.
func ItemSubPath(folder, name string) string {
	if folder == "" {
		return name
	}
	return folder + "/" + name
}

var validFolderPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*(/[a-z][a-z0-9-]*)*$`)

// ValidateFolder checks that a folder string is safe for use as a path
// segment under a type directory. Empty is allowed (means "no folder").
// Rules: forward-slash-separated lowercase tokens, each starting with a
// letter, containing only letters/digits/hyphens.
func ValidateFolder(folder string) error {
	if folder == "" {
		return nil
	}
	if strings.Contains(folder, "..") {
		return fmt.Errorf("invalid folder %q: must not contain '..'", folder)
	}
	if !validFolderPattern.MatchString(folder) {
		return fmt.Errorf("invalid folder %q: each segment must be lowercase alphanumeric with hyphens, starting with a letter, separated by /", folder)
	}
	return nil
}

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
