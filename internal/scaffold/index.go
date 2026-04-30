package scaffold

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/types"
)

// LoadLocalIndex reads and parses amaru_registry.json from disk.
func LoadLocalIndex(dir string) (*registry.RegistryIndex, error) {
	path := filepath.Join(dir, registryIndexFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", registryIndexFile, err)
	}

	var idx registry.RegistryIndex
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", registryIndexFile, err)
	}

	// Ensure maps are initialized
	if idx.Skills == nil {
		idx.Skills = make(map[string]registry.RegistryEntry)
	}
	if idx.Commands == nil {
		idx.Commands = make(map[string]registry.RegistryEntry)
	}
	if idx.Agents == nil {
		idx.Agents = make(map[string]registry.RegistryEntry)
	}
	if idx.Skillsets == nil {
		idx.Skillsets = make(map[string]registry.SkillsetEntry)
	}

	return &idx, nil
}

// SaveLocalIndex writes amaru_registry.json atomically (temp file + rename).
func SaveLocalIndex(dir string, idx *registry.RegistryIndex) error {
	path := filepath.Join(dir, registryIndexFile)
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling index: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// SetEntriesForType sets the entries map on the index for a given item type.
func SetEntriesForType(idx *registry.RegistryIndex, t types.ItemType, entries map[string]registry.RegistryEntry) {
	switch t {
	case types.Skill:
		idx.Skills = entries
	case types.Command:
		idx.Commands = entries
	case types.Agent:
		idx.Agents = entries
	}
}

// TouchUpdatedAt sets the UpdatedAt field to today's date.
func TouchUpdatedAt(idx *registry.RegistryIndex) {
	idx.UpdatedAt = time.Now().Format("2006-01-02")
}
