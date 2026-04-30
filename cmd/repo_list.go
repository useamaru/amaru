package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/useamaru/amaru/internal/scaffold"
	"github.com/useamaru/amaru/internal/types"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var (
	repoListType string
	repoListJSON bool
)

var repoListCmd = &cobra.Command{
	Use:   "list",
	Short: "List items in the local registry",
	Long:  "List all skills, commands, agents, and skillsets in the local registry.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoList()
	},
}

func init() {
	repoListCmd.Flags().StringVarP(&repoListType, "type", "t", "", "Filter by type: skill, command, agent, skillset")
	repoListCmd.Flags().BoolVar(&repoListJSON, "json", false, "Output as JSON")
	repoCmd.AddCommand(repoListCmd)
}

func runRepoList() error {
	dir, err := scaffold.FindRegistryRoot(".")
	if err != nil {
		return err
	}

	idx, err := scaffold.LoadLocalIndex(dir)
	if err != nil {
		return err
	}

	if repoListJSON {
		data, err := json.MarshalIndent(idx, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling index: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	filterType := types.ItemType(repoListType)

	for _, itemType := range types.AllInstallableTypes() {
		if repoListType != "" && filterType != itemType {
			continue
		}

		entries := idx.EntriesForType(itemType)
		ui.Header("%s (%d)", itemType.Plural(), len(entries))

		if len(entries) == 0 {
			fmt.Println()
			continue
		}

		var rows [][]string
		for name, entry := range entries {
			version := entry.Latest
			if version == "" {
				version = "latest"
			} else {
				version = "v" + version
			}
			rows = append(rows, []string{"  " + name, version, entry.Description})
		}
		ui.Table(rows)
		fmt.Println()
	}

	// Skillsets
	if repoListType == "" || repoListType == "skillset" {
		ui.Header("skillsets (%d)", len(idx.Skillsets))
		if len(idx.Skillsets) > 0 {
			var rows [][]string
			for name, entry := range idx.Skillsets {
				rows = append(rows, []string{"  " + name, fmt.Sprintf("%d items", len(entry.Items)), entry.Description})
			}
			ui.Table(rows)
		}
		fmt.Println()
	}

	return nil
}
