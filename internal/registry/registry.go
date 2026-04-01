package registry

import (
	"context"

	"github.com/Masterminds/semver/v3"
	"github.com/barelias/amaru/internal/types"
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

// MergeFrom merges skills, commands, agents, and skillsets from another index.
// Existing entries in the receiver are NOT overwritten (primary registry wins).
func (idx *RegistryIndex) MergeFrom(other *RegistryIndex) {
	for name, entry := range other.Skills {
		if _, exists := idx.Skills[name]; !exists {
			idx.Skills[name] = entry
		}
	}
	for name, entry := range other.Commands {
		if _, exists := idx.Commands[name]; !exists {
			idx.Commands[name] = entry
		}
	}
	for name, entry := range other.Agents {
		if _, exists := idx.Agents[name]; !exists {
			idx.Agents[name] = entry
		}
	}
	for name, entry := range other.Skillsets {
		if _, exists := idx.Skillsets[name]; !exists {
			idx.Skillsets[name] = entry
		}
	}
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
type RegistryEntry struct {
	Latest      string   `json:"latest"`
	Tags        []string `json:"tags,omitempty"`
	Description string   `json:"description"`
}

// SkillsetEntry is a named group of skills/commands/agents in the registry index.
// Skillsets expand to individual items on install (VS Code Extension Pack pattern).
// Items may be inline in the index, or stored in a separate manifest.json file
// under .amaru_registry/skillsets/<name>/manifest.json.
type SkillsetEntry struct {
	Latest      string         `json:"latest,omitempty"`
	Description string         `json:"description"`
	Tags        []string       `json:"tags,omitempty"`
	Items       []SkillsetItem `json:"items,omitempty"`
}

// SkillsetItem is one member of a skillset.
type SkillsetItem struct {
	Type string `json:"type"` // "skill", "command", or "agent"
	Name string `json:"name"`
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
