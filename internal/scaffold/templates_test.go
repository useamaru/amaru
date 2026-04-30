package scaffold

import (
	"strings"
	"testing"

	"github.com/useamaru/amaru/internal/types"
)

func TestItemManifestFor(t *testing.T) {
	tests := []struct {
		name     string
		itemType types.ItemType
		wantFile string
		wantType string
	}{
		{"skill", types.Skill, "skill.md", "skill"},
		{"command", types.Command, "command.md", "command"},
		{"agent", types.Agent, "agent.md", "agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := ItemManifestFor(tt.itemType, "test-item", "A test item", "author", []string{"tag1"})
			if m.Name != "test-item" {
				t.Errorf("Name = %q, want %q", m.Name, "test-item")
			}
			if m.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", m.Type, tt.wantType)
			}
			if len(m.Files) != 1 || m.Files[0] != tt.wantFile {
				t.Errorf("Files = %v, want [%q]", m.Files, tt.wantFile)
			}
			if m.Version != "" {
				t.Errorf("Version = %q, want empty", m.Version)
			}
		})
	}
}

func TestContentTemplateFor(t *testing.T) {
	tests := []struct {
		name     string
		itemType types.ItemType
		contains string
	}{
		{"skill has frontmatter", types.Skill, "description:"},
		{"skill has name", types.Skill, "# my-skill"},
		{"command has frontmatter", types.Command, "description:"},
		{"command has steps", types.Command, "## Steps"},
		{"agent has frontmatter", types.Agent, "description:"},
		{"agent has instructions", types.Agent, "## Instructions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := ContentTemplateFor(tt.itemType, "my-skill", "A test skill")
			if !strings.Contains(content, tt.contains) {
				t.Errorf("template missing %q:\n%s", tt.contains, content)
			}
		})
	}
}
