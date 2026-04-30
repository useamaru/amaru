package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/useamaru/amaru/internal/types"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var browseRegistry string

var browseCmd = &cobra.Command{
	Use:   "browse",
	Short: "List available skills/commands/agents from registries",
	Long:  "List everything available in configured registries (discovery).",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBrowse(cmd.Context())
	},
}

func init() {
	browseCmd.Flags().StringVar(&browseRegistry, "registry", "", "Filter by registry")
	rootCmd.AddCommand(browseCmd)
}

func runBrowse(ctx context.Context) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	clients, err := buildClients(ctx, m, true)
	if err != nil {
		return err
	}

	for alias, regConf := range m.Registries {
		if browseRegistry != "" && alias != browseRegistry {
			continue
		}

		client, ok := clients[alias]
		if !ok {
			continue
		}

		idx, err := client.FetchIndex(ctx)
		if err != nil {
			ui.Err("Failed to fetch %s: %v", alias, err)
			continue
		}

		fmt.Printf("\n[%s] %s\n", ui.Bold(alias), regConf.URL)

		for _, itemType := range types.AllInstallableTypes() {
			entries := idx.EntriesForType(itemType)
			if len(entries) > 0 {
				label := string(itemType.Plural())
				label = strings.ToUpper(label[:1]) + label[1:]
				fmt.Printf("  %s:\n", label)
				var rows [][]string
				for name, entry := range entries {
					tags := ""
					if len(entry.Tags) > 0 {
						tags = "[" + strings.Join(entry.Tags, ", ") + "]"
					}
					desc := entry.Description
					if entry.Source != "" {
						desc = fmt.Sprintf("%s  (← mirror:%s)", desc, entry.Source)
					}
					rows = append(rows, []string{"    " + name, entry.Latest, tags, desc})
				}
				ui.Table(rows)
			}
		}

		// Display skillsets
		if len(idx.Skillsets) > 0 {
			fmt.Printf("  Skillsets:\n")
			var rows [][]string
			for name, ss := range idx.Skillsets {
				tags := ""
				if len(ss.Tags) > 0 {
					tags = "[" + strings.Join(ss.Tags, ", ") + "]"
				}
				count := fmt.Sprintf("%d items", len(ss.Items))
				desc := ss.Description
				if ss.Source != "" {
					desc = fmt.Sprintf("%s  (← mirror:%s)", desc, ss.Source)
				}
				rows = append(rows, []string{"    " + name, count, tags, desc})
			}
			ui.Table(rows)
		}
	}

	return nil
}
