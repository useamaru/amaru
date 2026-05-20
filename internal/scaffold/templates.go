package scaffold

import (
	"fmt"
	"strings"

	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/types"
)

// ContentFilename returns the canonical (uppercase) on-disk filename for a
// given item type — e.g. SKILL.md, COMMAND.md, AGENT.md. amaru's readers
// still accept the lowercase form for back-compat, but new items are written
// uppercase to match the Anthropic convention.
func ContentFilename(itemType types.ItemType) string {
	return strings.ToUpper(itemType.Singular()) + ".md"
}

// ItemManifestFor creates a manifest.json struct for the given item type.
func ItemManifestFor(itemType types.ItemType, name, description, author string, tags []string) registry.ItemManifest {
	contentFile := ContentFilename(itemType)
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

// SkillTemplate returns the template content for a new SKILL.md file.
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

// CommandTemplate returns the template content for a new COMMAND.md file.
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

// AgentTemplate returns the template content for a new AGENT.md file.
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
