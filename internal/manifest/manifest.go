package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/useamaru/amaru/internal/types"
)

const ManifestFile = "amaru.json"

type Manifest struct {
	Version    string                    `json:"version"`
	Registries map[string]RegistryConfig `json:"registries"`
	Skills     map[string]DependencySpec `json:"skills,omitempty"`
	Commands   map[string]DependencySpec `json:"commands,omitempty"`
	Agents     map[string]DependencySpec `json:"agents,omitempty"`
	Skillsets  map[string]SkillsetSpec   `json:"skillsets,omitempty"`
	Context    *ContextConfig            `json:"context,omitempty"`
	Ignored    []string                  `json:"ignored,omitempty"`
}

// ContextConfig holds the configuration for context documentation sync.
type ContextConfig struct {
	Registry string `json:"registry"`
	Project  string `json:"project"`
	Path     string `json:"path,omitempty"`
}

type RegistryConfig struct {
	URL  string `json:"url"`
	Auth string `json:"auth"`
}

// DependencySpec handles both shorthand ("^1.0.0") and full form ({ "version": "^1.0.0", "registry": "main" }).
type DependencySpec struct {
	Version  string `json:"version"`
	Registry string `json:"registry,omitempty"`
}

// SkillsetSpec handles both shorthand ("^1.0.0") and full form ({ "version": "^1.0.0", "registry": "main" }).
type SkillsetSpec struct {
	Version  string `json:"version"`
	Registry string `json:"registry,omitempty"`
}

func (s *SkillsetSpec) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		s.Version = str
		return nil
	}
	type alias SkillsetSpec
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return fmt.Errorf("invalid skillset spec: %s", string(data))
	}
	*s = SkillsetSpec(a)
	return nil
}

func (s SkillsetSpec) MarshalJSON() ([]byte, error) {
	if s.Registry == "" {
		return json.Marshal(s.Version)
	}
	type alias SkillsetSpec
	return json.Marshal(alias(s))
}

func (d *DependencySpec) UnmarshalJSON(data []byte) error {
	// Try string shorthand first
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		d.Version = s
		d.Registry = ""
		return nil
	}

	// Full form
	type alias DependencySpec
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return fmt.Errorf("invalid dependency spec: %s", string(data))
	}
	*d = DependencySpec(a)
	return nil
}

func (d DependencySpec) MarshalJSON() ([]byte, error) {
	if d.Registry == "" {
		return json.Marshal(d.Version)
	}
	type alias DependencySpec
	return json.Marshal(alias(d))
}

// Load reads and parses amaru.json from the given directory.
func Load(dir string) (*Manifest, error) {
	path := filepath.Join(dir, ManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ManifestFile, err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", ManifestFile, err)
	}

	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", ManifestFile, err)
	}

	return &m, nil
}

// Save writes amaru.json to the given directory.
func Save(dir string, m *Manifest) error {
	path := filepath.Join(dir, ManifestFile)
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", ManifestFile, err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}

// Validate checks that the manifest is well-formed.
func (m *Manifest) Validate() error {
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if len(m.Registries) == 0 {
		return fmt.Errorf("at least one registry is required")
	}
	for alias, reg := range m.Registries {
		if reg.URL == "" {
			return fmt.Errorf("registry %q: url is required", alias)
		}
		switch reg.Auth {
		case "github", "token", "none":
		default:
			return fmt.Errorf("registry %q: auth must be github, token, or none (got %q)", alias, reg.Auth)
		}
	}
	return nil
}

// DefaultRegistry returns the first (only) registry alias if there is exactly one.
// Returns empty string if there are multiple registries.
func (m *Manifest) DefaultRegistry() string {
	if len(m.Registries) == 1 {
		for alias := range m.Registries {
			return alias
		}
	}
	return ""
}

// ResolveRegistry returns the effective registry alias for a dependency spec.
// If the spec doesn't specify a registry, it falls back to the single-registry default.
func (m *Manifest) ResolveRegistry(spec DependencySpec) (string, error) {
	if spec.Registry != "" {
		if _, ok := m.Registries[spec.Registry]; !ok {
			return "", fmt.Errorf("registry %q not found in manifest", spec.Registry)
		}
		return spec.Registry, nil
	}
	def := m.DefaultRegistry()
	if def == "" {
		return "", fmt.Errorf("registry must be specified when multiple registries are configured")
	}
	return def, nil
}

// IsIgnored returns true if the given name is in the ignored list.
func (m *Manifest) IsIgnored(name string) bool {
	for _, ignored := range m.Ignored {
		if ignored == name {
			return true
		}
	}
	return false
}

// DepsForType returns the dependency map for the given item type.
func (m *Manifest) DepsForType(t types.ItemType) map[string]DependencySpec {
	switch t {
	case types.Skill:
		return m.Skills
	case types.Command:
		return m.Commands
	case types.Agent:
		return m.Agents
	default:
		return nil
	}
}

// SetDep sets a dependency in the appropriate map, initializing if needed.
func (m *Manifest) SetDep(t types.ItemType, name string, spec DependencySpec) {
	switch t {
	case types.Skill:
		if m.Skills == nil {
			m.Skills = make(map[string]DependencySpec)
		}
		m.Skills[name] = spec
	case types.Command:
		if m.Commands == nil {
			m.Commands = make(map[string]DependencySpec)
		}
		m.Commands[name] = spec
	case types.Agent:
		if m.Agents == nil {
			m.Agents = make(map[string]DependencySpec)
		}
		m.Agents[name] = spec
	}
}

// AllDeps iterates over all installable types and calls fn for each dependency.
func (m *Manifest) AllDeps(fn func(t types.ItemType, name string, spec DependencySpec) error) error {
	for _, t := range types.AllInstallableTypes() {
		for name, spec := range m.DepsForType(t) {
			if err := fn(t, name, spec); err != nil {
				return err
			}
		}
	}
	return nil
}

// HasDep returns true if the given name exists in any installable type.
func (m *Manifest) HasDep(name string) bool {
	for _, t := range types.AllInstallableTypes() {
		if deps := m.DepsForType(t); deps != nil {
			if _, ok := deps[name]; ok {
				return true
			}
		}
	}
	return false
}

// SetSkillset adds or updates a skillset in the manifest.
func (m *Manifest) SetSkillset(name string, spec SkillsetSpec) {
	if m.Skillsets == nil {
		m.Skillsets = make(map[string]SkillsetSpec)
	}
	m.Skillsets[name] = spec
}

// ResolveSkillsetRegistry returns the effective registry alias for a skillset spec.
func (m *Manifest) ResolveSkillsetRegistry(spec SkillsetSpec) (string, error) {
	if spec.Registry != "" {
		if _, ok := m.Registries[spec.Registry]; !ok {
			return "", fmt.Errorf("registry %q not found in manifest", spec.Registry)
		}
		return spec.Registry, nil
	}
	def := m.DefaultRegistry()
	if def == "" {
		return "", fmt.Errorf("registry must be specified when multiple registries are configured")
	}
	return def, nil
}
