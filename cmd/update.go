package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/useamaru/amaru/internal/installer"
	"github.com/useamaru/amaru/internal/manifest"
	"github.com/useamaru/amaru/internal/registry"
	"github.com/useamaru/amaru/internal/resolver"
	"github.com/useamaru/amaru/internal/types"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/Masterminds/semver/v3"
	"github.com/spf13/cobra"
)

var updateSkillset string

var updateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update skills/commands to latest compatible versions",
	Long:  "Update skills/commands to the latest versions compatible with manifest ranges.\nUse --skillset to update all members of a skillset.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var name string
		if len(args) > 0 {
			name = args[0]
		}
		return runUpdate(cmd.Context(), name)
	},
}

func init() {
	updateCmd.Flags().StringVar(&updateSkillset, "skillset", "", "Update all members of a skillset")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(ctx context.Context, filterName string) error {
	m, err := loadManifest()
	if err != nil {
		return err
	}

	lock, err := loadLock()
	if err != nil {
		return err
	}

	clients, err := buildClients(ctx, m, false)
	if err != nil {
		return err
	}

	// If --skillset flag is set, update all members of that skillset
	if updateSkillset != "" {
		return runUpdateSkillset(ctx, updateSkillset, m, lock, clients)
	}

	// If filterName matches a skillset, delegate to skillset update
	if filterName != "" {
		if _, isSkillset := m.Skillsets[filterName]; isSkillset {
			return runUpdateSkillset(ctx, filterName, m, lock, clients)
		}
	}

	updated := 0

	for _, itemType := range types.AllInstallableTypes() {
		for name, spec := range m.DepsForType(itemType) {
			if filterName != "" && name != filterName {
				continue
			}
			did, err := updateItem(ctx, m, lock, clients, string(itemType), name, spec, lock.EntriesForType(itemType))
			if err != nil {
				return fmt.Errorf("%s %s: %w", itemType, name, err)
			}
			if did {
				updated++
			}
		}
	}

	if updated == 0 {
		if filterName != "" {
			fmt.Printf("\n%s is already at the latest compatible version.\n", filterName)
		} else {
			fmt.Println("\nEverything is already up to date.")
		}
		return nil
	}

	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock file: %w", err)
	}
	fmt.Println("\nLock file updated.")

	return nil
}

func runUpdateSkillset(ctx context.Context, ssName string, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client) error {
	// Source of truth is now the manifest
	ssSpec, inManifest := m.Skillsets[ssName]
	if !inManifest {
		return fmt.Errorf("skillset %q not found in manifest. Run 'amaru add %s --type=skillset' first", ssName, ssName)
	}

	regAlias, err := m.ResolveSkillsetRegistry(ssSpec)
	if err != nil {
		return err
	}

	client, ok := clients[regAlias]
	if !ok {
		return fmt.Errorf("no client for registry %q", regAlias)
	}

	// Fetch current registry index
	idx, err := client.FetchIndex(ctx)
	if err != nil {
		return fmt.Errorf("fetching registry index: %w", err)
	}

	remoteSS, exists := idx.Skillsets[ssName]
	if !exists {
		return fmt.Errorf("skillset %q no longer exists in registry %q", ssName, regAlias)
	}

	// Resolve items from manifest if not inline
	if len(remoteSS.Items) == 0 {
		ssManifest, err := client.FetchSkillsetManifest(ctx, ssName, remoteSS.Latest)
		if err != nil {
			return fmt.Errorf("fetching skillset manifest: %w", err)
		}
		remoteSS.Items = ssManifest.ToSkillsetItems()
	}

	// Build set of remote members for diffing
	remoteMembers := make(map[string]bool)
	for _, item := range remoteSS.Items {
		remoteMembers[fmt.Sprintf("%s/%s", item.Type, item.Name)] = true
	}

	// Build set of current locked members
	lockedSS := lock.Skillsets[ssName]
	localMembers := make(map[string]bool)
	for _, member := range lockedSS.Members {
		localMembers[member] = true
	}

	fmt.Printf("Updating skillset %q...\n", ssName)

	updated := 0
	added := 0
	removed := 0

	// Install new members and update existing ones
	for _, item := range remoteSS.Items {
		itemType := types.ItemType(item.Type)
		memberKey := fmt.Sprintf("%s/%s", item.Type, item.Name)
		lockEntries := lock.EntriesForType(itemType)

		entries := idx.EntriesForType(itemType)
		entry, ok := entries[item.Name]
		if !ok {
			ui.Warn("  %s %q not found in registry, skipping", item.Type, item.Name)
			continue
		}
		version := entry.Latest

		if !localMembers[memberKey] {
			// New member — download and install
			files, err := client.DownloadFiles(ctx, item.Type, item.Name, version)
			if err != nil {
				return fmt.Errorf("downloading %s %q: %w", item.Type, item.Name, err)
			}
			hash, err := installer.Install(".", item.Type, item.Name, files)
			if err != nil {
				return fmt.Errorf("installing %s %q: %w", item.Type, item.Name, err)
			}

			lockVersion := version
			if lockVersion == "" {
				lockVersion = "latest"
			}
			lockEntries[item.Name] = manifest.NewLockedEntry(lockVersion, regAlias, hash)
			displayVersion := version
			if displayVersion == "" {
				displayVersion = "latest"
			}
			ui.Check("  Added %s %s@%s (new member)", item.Type, item.Name, displayVersion)
			added++
			continue
		}

		// Existing member — re-download and compare hash
		locked, hasLock := lockEntries[item.Name]
		if !hasLock {
			continue
		}

		files, err := client.DownloadFiles(ctx, item.Type, item.Name, version)
		if err != nil {
			return fmt.Errorf("downloading %s %q: %w", item.Type, item.Name, err)
		}
		hash, err := installer.Install(".", item.Type, item.Name, files)
		if err != nil {
			return fmt.Errorf("installing %s %q: %w", item.Type, item.Name, err)
		}

		if hash != locked.Hash {
			lockVersion := version
			if lockVersion == "" {
				lockVersion = "latest"
			}
			lockEntries[item.Name] = manifest.NewLockedEntry(lockVersion, regAlias, hash)
			ui.Check("  Updated %s %s — content changed", item.Type, item.Name)
			updated++
		}
	}

	// Remove members that are no longer in the remote skillset
	for _, member := range lockedSS.Members {
		if remoteMembers[member] {
			continue
		}
		parts := strings.SplitN(member, "/", 2)
		if len(parts) != 2 {
			continue
		}
		itemType, itemName := parts[0], parts[1]

		if err := installer.Uninstall(".", itemType, itemName); err != nil {
			ui.Warn("  Failed to remove %s %s: %v", itemType, itemName, err)
			continue
		}
		delete(lock.EntriesForType(types.ItemType(itemType)), itemName)
		ui.Check("  Removed %s %s (no longer in skillset)", itemType, itemName)
		removed++
	}

	// Recompute skillset digest
	var digestItems []string
	var memberList []string
	for _, item := range remoteSS.Items {
		itemType := types.ItemType(item.Type)
		if le, ok := lock.EntriesForType(itemType)[item.Name]; ok {
			digestItems = append(digestItems, fmt.Sprintf("%s/%s/%s", item.Type, item.Name, le.Version))
		}
		memberList = append(memberList, fmt.Sprintf("%s/%s", item.Type, item.Name))
	}

	lock.Skillsets[ssName] = manifest.LockedSkillset{
		Registry:    regAlias,
		Digest:      manifest.SkillsetDigest(digestItems),
		Members:     memberList,
		InstalledAt: lockedSS.InstalledAt,
	}

	if updated == 0 && added == 0 && removed == 0 {
		fmt.Printf("\nSkillset %q is up to date.\n", ssName)
		return nil
	}

	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock file: %w", err)
	}

	fmt.Printf("\nSkillset %q: %d updated, %d added, %d removed.\n", ssName, updated, added, removed)
	return nil
}

func updateItem(ctx context.Context, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client, itemType, name string, spec manifest.DependencySpec, lockEntries map[string]manifest.LockedEntry) (bool, error) {
	regAlias, err := m.ResolveRegistry(spec)
	if err != nil {
		return false, err
	}

	client, ok := clients[regAlias]
	if !ok {
		return false, fmt.Errorf("no client for registry %q", regAlias)
	}

	locked, hasLock := lockEntries[name]
	if !hasLock {
		return false, nil // Not installed
	}

	// For "latest" items, re-download from default branch and compare hash
	if spec.Version == "latest" {
		files, err := client.DownloadFiles(ctx, itemType, name, "")
		if err != nil {
			return false, fmt.Errorf("downloading: %w", err)
		}

		hash, err := installer.Install(".", itemType, name, files)
		if err != nil {
			return false, fmt.Errorf("installing: %w", err)
		}

		if hash != locked.Hash {
			lockEntries[name] = manifest.NewLockedEntry("latest", regAlias, hash)
			ui.Check("Updating %s@latest — content changed [%s]", name, regAlias)
			return true, nil
		}
		return false, nil
	}

	versions, err := client.ListVersions(ctx, itemType, name)
	if err != nil {
		return false, fmt.Errorf("listing versions: %w", err)
	}

	// No tags found — registry doesn't use per-item version tags.
	// Re-download from default branch and compare hash (same as "latest" path).
	if len(versions) == 0 {
		files, err := client.DownloadFiles(ctx, itemType, name, "")
		if err != nil {
			return false, fmt.Errorf("downloading: %w", err)
		}
		hash, err := installer.Install(".", itemType, name, files)
		if err != nil {
			return false, fmt.Errorf("installing: %w", err)
		}
		if hash != locked.Hash {
			lockEntries[name] = manifest.NewLockedEntry(locked.Version, regAlias, hash)
			ui.Check("Updating %s — content changed [%s]", name, regAlias)
			return true, nil
		}
		return false, nil
	}

	currentV, err := semver.NewVersion(locked.Version)
	if err != nil {
		return false, err
	}

	best, err := resolver.Resolve(spec.Version, versions)
	if err != nil {
		return false, err
	}

	if !best.GreaterThan(currentV) {
		return false, nil
	}

	// Download and install new version
	files, err := client.DownloadFiles(ctx, itemType, name, best.String())
	if err != nil {
		return false, fmt.Errorf("downloading: %w", err)
	}

	hash, err := installer.Install(".", itemType, name, files)
	if err != nil {
		return false, fmt.Errorf("installing: %w", err)
	}

	lockEntries[name] = manifest.NewLockedEntry(best.String(), regAlias, hash)
	category := resolver.ClassifyUpdate(locked.Version, best.String())
	ui.Check("Updating %s: %s → %s (%s) [%s]", name, locked.Version, best.String(), category, regAlias)

	return true, nil
}
