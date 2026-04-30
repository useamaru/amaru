package scaffold

import (
	"fmt"

	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/types"
)

// ItemManifestFor creates a manifest.json struct for the given item type.
func ItemManifestFor(itemType types.ItemType, name, description, author string, tags []string) registry.ItemManifest {
	contentFile := fmt.Sprintf("%s.md", itemType.Singular())
	return registry.ItemManifest{
		Name:        name,
		Type:        itemType.Singular(),
		Version:     "",
		Description: description,
		Author:      author,
		Files:       []string{contentFile},
		Tags:        tags,
	}
}

// SkillTemplate returns the template content for a new skill.md file.
func SkillTemplate(name, description string) string {
	return fmt.Sprintf(`---
description: %s
---

# %s

TODO: Describe what this skill does and when Claude Code should use it.

## Usage

TODO: Add usage instructions and examples.
`, description, name)
}

// CommandTemplate returns the template content for a new command.md file.
func CommandTemplate(name, description string) string {
	return fmt.Sprintf(`---
description: %s
---

# %s

TODO: Describe what this command does.

## Steps

1. TODO: Add command steps.
`, description, name)
}

// AgentTemplate returns the template content for a new agent.md file.
func AgentTemplate(name, description string) string {
	return fmt.Sprintf(`---
description: %s
---

# %s

TODO: Describe the agent's role and capabilities.

## Instructions

TODO: Add agent instructions.
`, description, name)
}

// ContentTemplateFor returns the template content for the given item type.
func ContentTemplateFor(itemType types.ItemType, name, description string) string {
	switch itemType {
	case types.Skill:
		return SkillTemplate(name, description)
	case types.Command:
		return CommandTemplate(name, description)
	case types.Agent:
		return AgentTemplate(name, description)
	default:
		return fmt.Sprintf("# %s\n\nTODO: Add content.\n", name)
	}
}
