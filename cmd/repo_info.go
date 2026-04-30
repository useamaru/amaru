package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/scaffold"
	"github.com/useamaru/amaru/internal/types"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var repoInfoType string

var repoInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show details about a registry item",
	Long:  "Display detailed information about a skill, command, or agent in the local registry.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoInfo(args[0])
	},
}

func init() {
	repoInfoCmd.Flags().StringVarP(&repoInfoType, "type", "t", "skill", "Item type: skill, command, or agent")
	repoCmd.AddCommand(repoInfoCmd)
}

func runRepoInfo(name string) error {
	dir, err := scaffold.FindRegistryRoot(".")
	if err != nil {
		return err
	}

	idx, err := scaffold.LoadLocalIndex(dir)
	if err != nil {
		return err
	}

	itemType := types.ItemType(repoInfoType)
	entries := idx.EntriesForType(itemType)
	if entries == nil {
		return fmt.Errorf("invalid item type %q", repoInfoType)
	}

	entry, exists := entries[name]
	if !exists {
		return fmt.Errorf("%s %q not found in registry", itemType.Singular(), name)
	}

	version := entry.Latest
	if version == "" {
		version = "latest (unversioned)"
	} else {
		version = "v" + version
	}

	fmt.Printf("Name:        %s\n", ui.Bold(name))
	fmt.Printf("Type:        %s\n", itemType.Singular())
	fmt.Printf("Version:     %s\n", version)
	fmt.Printf("Description: %s\n", entry.Description)

	// Read manifest for more details — path resolved via the index's declared layout.
	layout, err := registry.LayoutFor(idx)
	if err != nil {
		return err
	}
	itemDir := layout.ItemDir(dir, itemType, name)
	manifestPath := filepath.Join(itemDir, "manifest.json")
	if data, err := os.ReadFile(manifestPath); err == nil {
		var m registry.ItemManifest
		if err := json.Unmarshal(data, &m); err == nil {
			if m.Author != "" {
				fmt.Printf("Author:      %s\n", m.Author)
			}
			if len(m.Tags) > 0 {
				fmt.Printf("Tags:        %s\n", strings.Join(m.Tags, ", "))
			}
			if len(m.Files) > 0 {
				fmt.Printf("Files:       %s\n", strings.Join(m.Files, ", "))
			}
		}
	}

	// Check skillset membership
	var memberOf []string
	for ssName, ss := range idx.Skillsets {
		for _, item := range ss.Items {
			if item.Type == itemType.Singular() && item.Name == name {
				memberOf = append(memberOf, ssName)
			}
		}
	}
	if len(memberOf) > 0 {
		fmt.Printf("Skillsets:   %s\n", strings.Join(memberOf, ", "))
	}

	relItem := layout.RelativeItemPath(itemType, name)
	fmt.Printf("\nmanifest.json: %s/manifest.json\n", relItem)
	fmt.Printf("Content:       %s/%s.md\n", relItem, itemType.Singular())

	return nil
}
