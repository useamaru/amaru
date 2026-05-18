package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/scaffold"
	"github.com/useamaru/amaru/internal/types"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var repoValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Check registry consistency",
	Long:  "Validate that the registry index matches the filesystem and all entries are well-formed.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runRepoValidate()
	},
}

func init() {
	repoCmd.AddCommand(repoValidateCmd)
}

type validateResult struct {
	errors   int
	warnings int
	ok       int
}

func runRepoValidate() error {
	dir, err := scaffold.FindRegistryRoot(".")
	if err != nil {
		return err
	}

	idx, err := scaffold.LoadLocalIndex(dir)
	if err != nil {
		return err
	}

	layout, err := registry.LayoutFor(idx)
	if err != nil {
		return err
	}

	fmt.Printf("Validating registry at %s (%s layout)...\n\n", dir, layout)

	result := &validateResult{}

	// Validate items for each type
	for _, itemType := range types.AllInstallableTypes() {
		entries := idx.EntriesForType(itemType)
		validateEntries(dir, layout, itemType, entries, result)
		checkOrphans(dir, layout, itemType, entries, result)
	}

	// Validate skillsets
	for name, skillset := range idx.Skillsets {
		allValid := true
		for _, item := range skillset.Items {
			itemType := types.ItemType(item.Type)
			entries := idx.EntriesForType(itemType)
			if entries == nil {
				ui.Err("skillsets/%s — member %s/%s has invalid type", name, item.Type, item.Name)
				result.errors++
				allValid = false
				continue
			}
			if _, exists := entries[item.Name]; !exists {
				ui.Err("skillsets/%s — member %s/%s not found in index", name, item.Type, item.Name)
				result.errors++
				allValid = false
			}
		}
		if allValid {
			ui.Check("skillsets/%s — all %d members present", name, len(skillset.Items))
			result.ok++
		}
	}

	fmt.Printf("\nErrors: %d  Warnings: %d  OK: %d\n", result.errors, result.warnings, result.ok)

	if result.errors > 0 {
		return fmt.Errorf("validation failed with %d error(s)", result.errors)
	}
	return nil
}

func validateEntries(dir string, layout registry.Layout, itemType types.ItemType, entries map[string]registry.RegistryEntry, result *validateResult) {
	for name, entry := range entries {
		subpath := registry.ItemSubPath(entry.Folder, name)
		itemDir := layout.ItemDir(dir, itemType, subpath)
		display := itemType.DirName() + "/" + subpath

		// Check name validity
		if err := types.ValidateItemName(name); err != nil {
			ui.Err("%s — invalid name: %v", display, err)
			result.errors++
			continue
		}

		if err := registry.ValidateFolder(entry.Folder); err != nil {
			ui.Err("%s — %v", display, err)
			result.errors++
			continue
		}

		// Check directory exists
		if _, err := os.Stat(itemDir); os.IsNotExist(err) {
			ui.Err("%s — directory not found", display)
			result.errors++
			continue
		}

		// Check manifest.json exists and is valid
		manifestPath := filepath.Join(itemDir, "manifest.json")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			ui.Err("%s — manifest.json not found", display)
			result.errors++
			continue
		}

		var manifest registry.ItemManifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			ui.Err("%s — invalid manifest.json: %v", display, err)
			result.errors++
			continue
		}

		hasWarnings := false

		// Check name matches
		if manifest.Name != name {
			ui.Err("%s — manifest name %q does not match directory", display, manifest.Name)
			result.errors++
			continue
		}

		// Check type matches
		if manifest.Type != itemType.Singular() {
			ui.Err("%s — manifest type %q does not match parent directory", display, manifest.Type)
			result.errors++
			continue
		}

		// Check files array matches actual files (warning)
		for _, f := range manifest.Files {
			fPath := filepath.Join(itemDir, f)
			if _, err := os.Stat(fPath); os.IsNotExist(err) {
				ui.Warn("%s — listed file %q not found", display, f)
				result.warnings++
				hasWarnings = true
			}
		}

		// Check description drift (warning)
		if entry.Description != manifest.Description {
			ui.Warn("%s — description differs between index and manifest", display)
			result.warnings++
			hasWarnings = true
		}

		// Check version drift (warning)
		if entry.Latest != "" && entry.Latest != manifest.Version {
			ui.Warn("%s — index latest %q differs from manifest version %q", display, entry.Latest, manifest.Version)
			result.warnings++
			hasWarnings = true
		}

		if !hasWarnings {
			ui.Check("%s — OK", display)
		}
		result.ok++
	}
}

// checkOrphans walks the type directory one level deep and flags any directory
// that doesn't correspond to an indexed entry. A directory at the top level
// that contains a manifest.json is treated as a flat item; a directory without
// one is treated as a folder and we recurse into its children. Items recorded
// in the index with a Folder are matched only if found at the expected path.
func checkOrphans(dir string, layout registry.Layout, itemType types.ItemType, entries map[string]registry.RegistryEntry, result *validateResult) {
	typeDir := layout.TypeDir(dir, itemType)
	topLevel, err := os.ReadDir(typeDir)
	if err != nil {
		return
	}

	indexedByPath := make(map[string]bool, len(entries))
	for name, entry := range entries {
		indexedByPath[registry.ItemSubPath(entry.Folder, name)] = true
	}

	for _, de := range topLevel {
		if !de.IsDir() {
			continue
		}
		childName := de.Name()
		childPath := filepath.Join(typeDir, childName)

		// A child with manifest.json is a flat item. A child without is a folder.
		if _, err := os.Stat(filepath.Join(childPath, "manifest.json")); err == nil {
			if !indexedByPath[childName] {
				ui.Warn("%s/%s — orphaned directory (not in index)", itemType.DirName(), childName)
				result.warnings++
			}
			continue
		}

		grand, err := os.ReadDir(childPath)
		if err != nil {
			continue
		}
		for _, ge := range grand {
			if !ge.IsDir() {
				continue
			}
			subpath := childName + "/" + ge.Name()
			if !indexedByPath[subpath] {
				ui.Warn("%s/%s — orphaned directory (not in index)", itemType.DirName(), subpath)
				result.warnings++
			}
		}
	}
}
