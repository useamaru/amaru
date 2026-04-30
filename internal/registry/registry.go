package registry

import (
	"context"
	"fmt"

	"github.com/Masterminds/semver/v3"
	"github.com/useamaru/amaru/internal/types"
)

// RegistryIndex is the parsed amaru_registry.json from the remote registry.
type RegistryIndex struct {
	AmaruVersion string                   `json:"amaru_version"`
	UpdatedAt    string                   `json:"updated_at"`
	Mirrors      []string                 `json:"mirrors,omitempty"` // e.g. ["github:vercel-labs/agent-skills"]
	Skills       map[string]RegistryEntry `json:"skills,omitempty"`
	Commands     map[string]RegistryEntry `json:"commands,omitempty"`
	Agents       map[string]RegistryEntry `json:"agents,omitempty"`
	Skillsets    map[string]SkillsetEntry `json:"skillsets,omitempty"`
}

// LayoutVersion returns the on-disk layout version encoded in AmaruVersion.
// Empty/missing values default to 1 (legacy nested layout under .amaru_registry/).
// "1" → 1, "2" → 2 (flat layout at the repo root). Any other value is an error,
// so older amarus fail loudly when they encounter a future v3+ registry.
func (idx *RegistryIndex) LayoutVersion() (int, error) {
	switch idx.AmaruVersion {
	case "", "1":
		return 1, nil
	case "2":
		return 2, nil
	default:
		return 0, fmt.Errorf("unknown amaru_version %q (this amaru only understands 1 and 2)", idx.AmaruVersion)
	}
}

// MergeFrom merges skills, commands, agents, and skillsets from another index
// and stamps each newly-copied entry with otherURL as its provenance Source.
//
// Semantics, asserted by tests in github_mirror_test.go:
//   - Receiver wins on collision: an entry already present in the receiver is
//     never replaced, and its Source field is left untouched (primary entries
//     keep Source = "").
//   - Receiver-only entries are unmodified — their Source carries forward
//     whatever value it already had (which is "" for the primary, or an
//     earlier mirror's URL in chained-mirror scenarios).
//   - Entries unique to other are copied with Source = otherURL.
//
// otherURL should be the canonical URL the merged index was fetched from
// (e.g. "github:vercel-labs/agent-skills"). Empty otherURL is allowed but
// produces empty Source values, which makes provenance indistinguishable
// from the primary — call sites that care should pass a non-empty URL.
func (idx *RegistryIndex) MergeFrom(other *RegistryIndex, otherURL string) {
	for name, entry := range other.Skills {
		if _, exists := idx.Skills[name]; !exists {
			entry.Source = otherURL
			idx.Skills[name] = entry
		}
	}
	for name, entry := range other.Commands {
		if _, exists := idx.Commands[name]; !exists {
			entry.Source = otherURL
			idx.Commands[name] = entry
		}
	}
	for name, entry := range other.Agents {
		if _, exists := idx.Agents[name]; !exists {
			entry.Source = otherURL
			idx.Agents[name] = entry
		}
	}
	for name, entry := range other.Skillsets {
		if _, exists := idx.Skillsets[name]; !exists {
			entry.Source = otherURL
			idx.Skillsets[name] = entry
		}
	}
}

// SkillsetsContaining returns the names of skillsets whose Items list
// includes the given item. Returned slice is in iteration order — callers
// that need a stable order should sort.
func (idx *RegistryIndex) SkillsetsContaining(itemType types.ItemType, name string) []string {
	var hits []string
	for ssName, ss := range idx.Skillsets {
		for _, it := range ss.Items {
			if it.Type == string(itemType) && it.Name == name {
				hits = append(hits, ssName)
				break
			}
		}
	}
	return hits
}

// EntriesForType returns the registry entries for a given item type.
func (idx *RegistryIndex) EntriesForType(t types.ItemType) map[string]RegistryEntry {
	switch t {
	case types.Skill:
		return idx.Skills
	case types.Command:
		return idx.Commands
	case types.Agent:
		return idx.Agents
	default:
		return nil
	}
}

// RegistryEntry is one skill or command in the registry index.
//
// Source is a runtime-only provenance marker populated by MergeFrom for
// entries that came from a mirror. It is intentionally non-serialized
// (json:"-") — the registry contract on disk doesn't change. Empty Source
// means the entry came from the primary registry.
type RegistryEntry struct {
	Latest      string   `json:"latest"`
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description"`
	Source      string   `json:"-"`
}

// SkillsetEntry is a named group of skills/commands/agents in the registry index.
// Skillsets expand to individual items on install (VS Code Extension Pack pattern).
// Items may be inline in the index, or stored in a separate manifest.json file
// under skillsets/<name>/manifest.json (or .amaru_registry/skillsets/<name>/manifest.json
// in v1 layout).
//
// Source is a runtime-only provenance marker (see RegistryEntry.Source).
type SkillsetEntry struct {
	Latest      string         `json:"latest,omitempty"`
	Description string         `json:"description"`
	Tags        []string       `json:"tags,omitempty"`
	Items       []SkillsetItem `json:"items,omitempty"`
	Source      string         `json:"-"`
}

// SkillsetItem is one member of a skillset.
//
// Registry is optional: when set, the member is sourced from that registry
// alias (the consumer must have it configured in amaru.json). When empty,
// the member is sourced from the skillset's own home registry — the
// pre-existing single-registry behavior.
type SkillsetItem struct {
	Type     string `json:"type"` // "skill", "command", or "agent"
	Name     string `json:"name"`
	Registry string `json:"registry,omitempty"`
}

// ItemManifest is the manifest.json inside a skill/command directory in the registry.
type ItemManifest struct {
	Name        string           `json:"name"`
	Type        string           `json:"type"`
	Version     string           `json:"version"`
	Description string           `json:"description"`
	Author      string           `json:"author"`
	Changelog   []ChangelogEntry `json:"changelog,omitempty"`
	Files       []string         `json:"files"`
	Tags        []string         `json:"tags,omitempty"`
}

// ChangelogEntry records a version change.
type ChangelogEntry struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	Note    string `json:"note"`
}

// SkillsetManifest is the manifest.json inside a skillset directory in the registry.
// It lists members by type using string arrays (e.g., "skills": ["foo", "bar"]).
type SkillsetManifest struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Skills      []string `json:"skills,omitempty"`
	Commands    []string `json:"commands,omitempty"`
	Agents      []string `json:"agents,omitempty"`
}

// ToSkillsetItems converts the manifest's member lists into []SkillsetItem.
func (m *SkillsetManifest) ToSkillsetItems() []SkillsetItem {
	var items []SkillsetItem
	for _, name := range m.Skills {
		items = append(items, SkillsetItem{Type: "skill", Name: name})
	}
	for _, name := range m.Commands {
		items = append(items, SkillsetItem{Type: "command", Name: name})
	}
	for _, name := range m.Agents {
		items = append(items, SkillsetItem{Type: "agent", Name: name})
	}
	return items
}

// File represents a downloaded file from the registry.
type File struct {
	Path    string // Relative path within the skill/command directory
	Content []byte
}

// Client is the interface for accessing a remote registry.
type Client interface {
	// FetchIndex downloads and parses the registry.json index.
	FetchIndex(ctx context.Context) (*RegistryIndex, error)

	// ListVersions returns all available versions for an item.
	// itemType is "skill", "command", or "agent".
	ListVersions(ctx context.Context, itemType, name string) ([]*semver.Version, error)

	// DownloadFiles downloads all files for a specific version of an item.
	DownloadFiles(ctx context.Context, itemType, name, version string) ([]File, error)

	// FetchSkillsetManifest downloads and parses the manifest.json for a skillset.
	// This is used when the index doesn't include inline items.
	FetchSkillsetManifest(ctx context.Context, name, version string) (*SkillsetManifest, error)
}
