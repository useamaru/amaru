package cmd

import (
	"context"
	"fmt"

	"github.com/useamaru/amaru/internal/installer"
	"github.com/useamaru/amaru/internal/manifest"
	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/types"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed skills, commands, and agents",
	Long:  "List everything installed in the project with status and origin.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runList(cmd.Context())
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func runList(ctx context.Context) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	lock, err := loadLock()
	if err != nil {
		return err
	}

	// Try to fetch registry indexes for status
	clients, clientErr := buildClients(ctx, m, true)
	indexes := make(map[string]*registry.RegistryIndex)
	if clientErr == nil {
		for alias, client := range clients {
			idx, err := client.FetchIndex(ctx)
			if err == nil {
				indexes[alias] = idx
			}
		}
	}

	// Build a reverse map: item "type/name" → skillset name
	skillsetMembership := make(map[string]string)
	for ssName, ss := range lock.Skillsets {
		for _, member := range ss.Members {
			skillsetMembership[member] = ssName
		}
	}

	hasItems := false
	for _, itemType := range types.AllInstallableTypes() {
		entries := lock.EntriesForType(itemType)
		if len(entries) > 0 {
			hasItems = true
			ui.Header("%s:", itemType.Plural())
			var rows [][]string
			for name, entry := range entries {
				status := statusForItem(entry, itemType, name, indexes, m)
				displayVersion := entry.Version
				if displayVersion == "" {
					displayVersion = "latest"
				}
				origin := fmt.Sprintf("[%s]", entry.Registry)
				// Annotate mirror provenance when the live index records a Source.
				if idx, ok := indexes[entry.Registry]; ok {
					if regEntry, ok := idx.EntriesForType(itemType)[name]; ok && regEntry.Source != "" {
						origin = fmt.Sprintf("[%s ← mirror:%s]", entry.Registry, regEntry.Source)
					}
				}
				// Show skillset provenance from lock membership
				memberKey := fmt.Sprintf("%s/%s", itemType, name)
				if ssName, ok := skillsetMembership[memberKey]; ok {
					origin += fmt.Sprintf(" (via %s)", ssName)
				}
				rows = append(rows, []string{name, displayVersion, status, origin})
			}
			ui.Table(rows)
		}
	}

	// Show skillsets
	if len(lock.Skillsets) > 0 {
		hasItems = true
		ui.Header("Skillsets:")
		for name, ss := range lock.Skillsets {
			fmt.Printf("  %s [%s] (%d members)\n", name, ss.Registry, len(ss.Members))
		}
	}

	if !hasItems {
		fmt.Println("No items installed. Run 'amaru install' first.")
	}

	return nil
}

func statusForItem(entry manifest.LockedEntry, itemType types.ItemType, name string, indexes map[string]*registry.RegistryIndex, m *manifest.Manifest) string {
	if !installer.IsInstalled(".", string(itemType), name) {
		return ui.Error("✗ not installed")
	}

	// "latest" items skip semver comparison
	if entry.Version == "latest" || entry.Version == "" {
		return ui.Success("✓ latest")
	}

	idx, ok := indexes[entry.Registry]
	if !ok {
		return "?"
	}

	regEntries := idx.EntriesForType(itemType)
	regEntry, ok := regEntries[name]
	if !ok {
		return ui.Success("✓ up-to-date")
	}

	// Skip semver comparison if registry entry has no version
	if regEntry.Latest == "" {
		return ui.Success("✓ up-to-date")
	}

	latestV, err := semver.NewVersion(regEntry.Latest)
	if err != nil {
		return "?"
	}
	currentV, err := semver.NewVersion(entry.Version)
	if err != nil {
		return "?"
	}

	if latestV.GreaterThan(currentV) {
		if latestV.Major() > currentV.Major() {
			return ui.Warning(fmt.Sprintf("⚠ %s MAJOR", regEntry.Latest))
		}
		return ui.Warning(fmt.Sprintf("⚠ %s avail", regEntry.Latest))
	}

	return ui.Success("✓ up-to-date")
}
