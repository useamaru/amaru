package cmd

import (
	"fmt"
	"os"

	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/scaffold"
	"github.com/useamaru/amaru/internal/types"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var (
	repoRemoveType  string
	repoRemoveForce bool
)

var repoRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an item from the registry",
	Long:  "Remove a skill, command, agent, or skillset from the local registry index and delete its files.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoRemove(args[0])
	},
}

func init() {
	repoRemoveCmd.Flags().StringVarP(&repoRemoveType, "type", "t", "skill", "Item type: skill, command, agent, or skillset")
	repoRemoveCmd.Flags().BoolVarP(&repoRemoveForce, "force", "f", false, "Skip skillset dependency check")
	repoCmd.AddCommand(repoRemoveCmd)
}

func runRepoRemove(name string) error {
	dir, err := scaffold.FindRegistryRoot(".")
	if err != nil {
		return err
	}

	idx, err := scaffold.LoadLocalIndex(dir)
	if err != nil {
		return err
	}

	if repoRemoveType == "skillset" {
		return runRepoRemoveSkillset(dir, name, idx)
	}

	itemType := types.ItemType(repoRemoveType)
	entries := idx.EntriesForType(itemType)
	if entries == nil {
		return fmt.Errorf("invalid item type %q", repoRemoveType)
	}

	if _, exists := entries[name]; !exists {
		return fmt.Errorf("%s %q not found in registry", itemType.Singular(), name)
	}

	// Check skillset dependencies
	if !repoRemoveForce {
		var refs []string
		for ssName, ss := range idx.Skillsets {
			for _, item := range ss.Items {
				if item.Type == itemType.Singular() && item.Name == name {
					refs = append(refs, ssName)
				}
			}
		}
		if len(refs) > 0 {
			return fmt.Errorf("%s %q is referenced by skillset(s): %v\nUse --force to remove anyway", itemType.Singular(), name, refs)
		}
	}

	// Remove from index
	delete(entries, name)
	scaffold.SetEntriesForType(idx, itemType, entries)
	scaffold.TouchUpdatedAt(idx)

	if err := scaffold.SaveLocalIndex(dir, idx); err != nil {
		return err
	}

	// Remove directory using the index's declared layout.
	layout, err := registry.LayoutFor(idx)
	if err != nil {
		return err
	}
	itemDir := layout.ItemDir(dir, itemType, name)
	if _, err := os.Stat(itemDir); err == nil {
		if err := os.RemoveAll(itemDir); err != nil {
			ui.Warn("Could not remove directory %s: %v", itemDir, err)
		}
	}

	ui.Check("Removed %s %q from registry", itemType.Singular(), name)
	ui.Warn("Orphaned git tags (if any) were not deleted. Remove manually with: git tag -d <tag>")

	return nil
}

func runRepoRemoveSkillset(dir, name string, idx *registry.RegistryIndex) error {
	if _, exists := idx.Skillsets[name]; !exists {
		return fmt.Errorf("skillset %q not found in registry", name)
	}

	delete(idx.Skillsets, name)
	scaffold.TouchUpdatedAt(idx)

	if err := scaffold.SaveLocalIndex(dir, idx); err != nil {
		return err
	}

	ui.Check("Removed skillset %q from registry", name)
	return nil
}
