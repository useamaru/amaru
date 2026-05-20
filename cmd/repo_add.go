package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/scaffold"
	"github.com/useamaru/amaru/internal/types"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var (
	repoAddType        string
	repoAddDescription string
	repoAddAuthor      string
	repoAddTags        string
	repoAddItems       string
	repoAddFolder      string
)

var repoAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new item to the registry",
	Long:  "Create a new skill, command, agent, or skillset in the local registry with template files.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoAdd(args[0])
	},
}

func init() {
	repoAddCmd.Flags().StringVarP(&repoAddType, "type", "t", "skill", "Item type: skill, command, agent, or skillset")
	repoAddCmd.Flags().StringVarP(&repoAddDescription, "description", "d", "", "Item description")
	repoAddCmd.Flags().StringVarP(&repoAddAuthor, "author", "a", "", "Author name (defaults to git user.name)")
	repoAddCmd.Flags().StringVar(&repoAddTags, "tags", "", "Comma-separated tags")
	repoAddCmd.Flags().StringVar(&repoAddItems, "items", "", "Comma-separated type/name members (required for skillsets)")
	repoAddCmd.Flags().StringVar(&repoAddFolder, "folder", "", "Organize the item under <typedir>/<folder>/<name>/ (cosmetic; folder is not part of the item name)")
	repoCmd.AddCommand(repoAddCmd)
}

func runRepoAdd(name string) error {
	if err := types.ValidateItemName(name); err != nil {
		return err
	}

	dir, err := scaffold.FindRegistryRoot(".")
	if err != nil {
		return err
	}

	idx, err := scaffold.LoadLocalIndex(dir)
	if err != nil {
		return err
	}

	if repoAddType == "skillset" {
		return runRepoAddSkillset(dir, name, idx)
	}

	itemType := types.ItemType(repoAddType)
	if itemType != types.Skill && itemType != types.Command && itemType != types.Agent {
		return fmt.Errorf("invalid item type %q: must be skill, command, agent, or skillset", repoAddType)
	}

	if err := registry.ValidateFolder(repoAddFolder); err != nil {
		return err
	}

	// Check for name collision across all types
	for _, t := range types.AllInstallableTypes() {
		entries := idx.EntriesForType(t)
		if _, exists := entries[name]; exists {
			return fmt.Errorf("%s %q already exists in registry", t.Singular(), name)
		}
	}

	description := repoAddDescription
	if description == "" {
		description = fmt.Sprintf("TODO: describe %s", name)
	}

	author := repoAddAuthor
	if author == "" {
		author = gitUserName()
	}

	var tags []string
	if repoAddTags != "" {
		for _, tag := range strings.Split(repoAddTags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	// Create directory using the index's declared layout (+ optional folder).
	layout, err := registry.LayoutFor(idx)
	if err != nil {
		return err
	}
	subpath := registry.ItemSubPath(repoAddFolder, name)
	itemDir := layout.ItemDir(dir, itemType, subpath)
	if err := os.MkdirAll(itemDir, 0755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Write manifest.json
	manifest := scaffold.ItemManifestFor(itemType, name, description, author, tags)
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}
	manifestData = append(manifestData, '\n')
	if err := os.WriteFile(filepath.Join(itemDir, "manifest.json"), manifestData, 0644); err != nil {
		return fmt.Errorf("writing manifest.json: %w", err)
	}

	// Write content file (uppercase by default — matches Anthropic SKILL.md convention)
	contentFile := scaffold.ContentFilename(itemType)
	content := scaffold.ContentTemplateFor(itemType, name, description)
	if err := os.WriteFile(filepath.Join(itemDir, contentFile), []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", contentFile, err)
	}

	// Update index
	entries := idx.EntriesForType(itemType)
	entries[name] = registry.RegistryEntry{
		Latest:      "",
		Description: description,
		Tags:        tags,
		Folder:      repoAddFolder,
	}
	scaffold.SetEntriesForType(idx, itemType, entries)
	scaffold.TouchUpdatedAt(idx)

	if err := scaffold.SaveLocalIndex(dir, idx); err != nil {
		return err
	}

	ui.Check("Created %s %q", itemType.Singular(), name)
	relItem := layout.RelativeItemPath(itemType, subpath)
	fmt.Printf("  Directory: %s/\n", relItem)
	fmt.Printf("  Content:   %s/%s\n", relItem, contentFile)
	fmt.Printf("\n  Next steps:\n")
	fmt.Printf("    1. Edit %s/%s\n", relItem, contentFile)
	fmt.Printf("    2. amaru repo tag %s 1.0.0 --type %s\n", name, itemType.Singular())

	return nil
}

func runRepoAddSkillset(dir, name string, idx *registry.RegistryIndex) error {
	if repoAddItems == "" {
		return fmt.Errorf("--items is required for skillsets (e.g., --items \"skill/foo,command/bar\")")
	}

	// Check name collision with existing skillsets
	if _, exists := idx.Skillsets[name]; exists {
		return fmt.Errorf("skillset %q already exists in registry", name)
	}

	// Parse and validate items.
	//
	// Item syntax: <type>/<name>[@<registry>]
	//   skill/foo          → member from this registry
	//   skill/foo@platform → member sourced from registry alias "platform"
	//                        (consumer must have it configured in amaru.json)
	//
	// The publisher cannot validate that consumers will have the named registry
	// configured — they cooperate via convention. We do, however, refuse to
	// list a same-registry member (no '@' suffix) that doesn't exist in this
	// registry's index, since that's catchable here.
	var items []registry.SkillsetItem
	for _, raw := range strings.Split(repoAddItems, ",") {
		raw = strings.TrimSpace(raw)

		// Split off optional @registry suffix.
		var itemRegistry string
		if at := strings.LastIndex(raw, "@"); at != -1 {
			itemRegistry = strings.TrimSpace(raw[at+1:])
			raw = strings.TrimSpace(raw[:at])
			if itemRegistry == "" {
				return fmt.Errorf("invalid item format %q: trailing '@' but no registry alias", raw)
			}
		}

		parts := strings.SplitN(raw, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid item format %q: expected type/name[@registry] (e.g., skill/foo or skill/foo@platform)", raw)
		}
		itemType := types.ItemType(parts[0])
		itemName := parts[1]

		if itemType != types.Skill && itemType != types.Command && itemType != types.Agent {
			return fmt.Errorf("invalid type %q in item %q", parts[0], raw)
		}

		// For same-registry members, validate they exist in this index.
		// Cross-registry members can't be validated at publish time.
		if itemRegistry == "" {
			entries := idx.EntriesForType(itemType)
			if _, exists := entries[itemName]; !exists {
				return fmt.Errorf("%s %q not found in registry (all same-registry skillset members must exist; use type/name@<alias> to reference other registries)", itemType.Singular(), itemName)
			}
		}

		items = append(items, registry.SkillsetItem{
			Type:     itemType.Singular(),
			Name:     itemName,
			Registry: itemRegistry,
		})
	}

	description := repoAddDescription
	if description == "" {
		description = fmt.Sprintf("TODO: describe %s skillset", name)
	}

	var tags []string
	if repoAddTags != "" {
		for _, tag := range strings.Split(repoAddTags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	idx.Skillsets[name] = registry.SkillsetEntry{
		Description: description,
		Tags:        tags,
		Items:       items,
	}
	scaffold.TouchUpdatedAt(idx)

	if err := scaffold.SaveLocalIndex(dir, idx); err != nil {
		return err
	}

	ui.Check("Created skillset %q with %d items", name, len(items))
	return nil
}

func gitUserName() string {
	out, err := exec.Command("git", "config", "user.name").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
