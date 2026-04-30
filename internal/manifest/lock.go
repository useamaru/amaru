package manifest

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/useamaru/amaru/internal/types"
)

const LockFile = "amaru.lock"

type Lock struct {
	LockedAt  string                       `json:"locked_at"`
	Skills    map[string]LockedEntry       `json:"skills,omitempty"`
	Commands  map[string]LockedEntry       `json:"commands,omitempty"`
	Agents    map[string]LockedEntry       `json:"agents,omitempty"`
	Skillsets map[string]LockedSkillset    `json:"skillsets,omitempty"`
}

type LockedEntry struct {
	Version     string `json:"version"`
	Registry    string `json:"registry"`
	Hash        string `json:"hash"`
	InstalledAt string `json:"installed_at"`
}

// LockedSkillset tracks an installed skillset and its member digest.
type LockedSkillset struct {
	Registry    string   `json:"registry"`
	Digest      string   `json:"digest"`       // hash of sorted member type+name+version
	Members     []string `json:"members"`       // "type/name" list for display
	InstalledAt string   `json:"installed_at"`
}

// SkillsetDigest computes a deterministic hash of skillset member versions.
// Items is a list of "type/name/version" strings.
func SkillsetDigest(items []string) string {
	sorted := make([]string, len(items))
	copy(sorted, items)
	sort.Strings(sorted)
	h := sha256.New()
	for _, item := range sorted {
		h.Write([]byte(item + "\n"))
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

// LoadLock reads and parses amaru.lock from the given directory.
func LoadLock(dir string) (*Lock, error) {
	path := filepath.Join(dir, LockFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Lock{
				Skills:    make(map[string]LockedEntry),
				Commands:  make(map[string]LockedEntry),
				Agents:    make(map[string]LockedEntry),
				Skillsets: make(map[string]LockedSkillset),
			}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", LockFile, err)
	}

	var l Lock
	if err := json.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", LockFile, err)
	}
	if l.Skills == nil {
		l.Skills = make(map[string]LockedEntry)
	}
	if l.Commands == nil {
		l.Commands = make(map[string]LockedEntry)
	}
	if l.Agents == nil {
		l.Agents = make(map[string]LockedEntry)
	}
	if l.Skillsets == nil {
		l.Skillsets = make(map[string]LockedSkillset)
	}
	return &l, nil
}

// EntriesForType returns the lock entries map for the given item type.
func (l *Lock) EntriesForType(t types.ItemType) map[string]LockedEntry {
	switch t {
	case types.Skill:
		return l.Skills
	case types.Command:
		return l.Commands
	case types.Agent:
		return l.Agents
	default:
		return nil
	}
}

// SaveLock writes amaru.lock to the given directory.
func SaveLock(dir string, l *Lock) error {
	l.LockedAt = time.Now().UTC().Format(time.RFC3339)
	path := filepath.Join(dir, LockFile)
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", LockFile, err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// NewLockedEntry creates a new lock entry with the current timestamp.
func NewLockedEntry(version, registry, hash string) LockedEntry {
	return LockedEntry{
		Version:     version,
		Registry:    registry,
		Hash:        hash,
		InstalledAt: time.Now().UTC().Format(time.RFC3339),
	}
}
