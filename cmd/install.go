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

	"github.com/spf13/cobra"
)

var installForce bool

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install skills and commands from manifest",
	Long:  "Reads amaru.json, authenticates with registries, resolves versions, copies files, and generates amaru.lock.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInstall(cmd.Context())
	},
}

func init() {
	installCmd.Flags().BoolVar(&installForce, "force", false, "Reinstall even if lock exists and versions are compatible")
	rootCmd.AddCommand(installCmd)
}

func runInstall(ctx context.Context) error {
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

	for _, itemType := range types.AllInstallableTypes() {
		deps := m.DepsForType(itemType)
		if len(deps) > 0 {
			ui.Header("Installing %s...", itemType.Plural())
			lockEntries := lock.EntriesForType(itemType)
			for name, spec := range deps {
				if err := installItem(ctx, m, lock, clients, string(itemType), name, spec, lockEntries); err != nil {
					return fmt.Errorf("%s %s: %w", itemType, name, err)
				}
			}
		}
	}

	// Install skillsets
	if len(m.Skillsets) > 0 {
		ui.Header("Installing skillsets...")
		for name, spec := range m.Skillsets {
			if err := installSkillset(ctx, name, spec, m, lock, clients); err != nil {
				return fmt.Errorf("skillset %s: %w", name, err)
			}
		}
	}

	if err := manifest.SaveLock(".", lock); err != nil {
		return fmt.Errorf("saving lock file: %w", err)
	}
	fmt.Println("\nLock file updated.")

	return nil
}

func installItem(ctx context.Context, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client, itemType, name string, spec manifest.DependencySpec, lockEntries map[string]manifest.LockedEntry) error {
	regAlias, err := m.ResolveRegistry(spec)
	if err != nil {
		return err
	}

	client, ok := clients[regAlias]
	if !ok {
		return fmt.Errorf("no client for registry %q", regAlias)
	}

	// Check if already installed and up to date
	if !installForce {
		if locked, ok := lockEntries[name]; ok {
			if installer.IsInstalled(".", itemType, name) {
				displayVersion := locked.Version
				if displayVersion == "" {
					displayVersion = "latest"
				}
				ui.Check("%s@%s (%s) — already installed", name, displayVersion, regAlias)
				return nil
			}
		}
	}

	// Resolve version (returns "" for "latest" constraint)
	resolved, err := resolveVersion(ctx, client, itemType, name, spec.Version)
	if err != nil {
		return err
	}

	// Download files (empty version downloads from default branch)
	files, err := client.DownloadFiles(ctx, itemType, name, resolved)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}

	// Install to local project
	hash, err := installer.Install(".", itemType, name, files)
	if err != nil {
		return fmt.Errorf("installing: %w", err)
	}

	// Update lock
	lockVersion := resolved
	if lockVersion == "" {
		lockVersion = "latest"
	}
	lockEntries[name] = manifest.NewLockedEntry(lockVersion, regAlias, hash)

	displayVersion := resolved
	if displayVersion == "" {
		displayVersion = "latest"
	}
	ui.Check("%s@%s (%s)", name, displayVersion, regAlias)
	return nil
}

func installSkillset(ctx context.Context, name string, spec manifest.SkillsetSpec, m *manifest.Manifest, lock *manifest.Lock, clients map[string]registry.Client) error {
	regAlias, err := m.ResolveSkillsetRegistry(spec)
	if err != nil {
		return err
	}

	client, ok := clients[regAlias]
	if !ok {
		return fmt.Errorf("no client for registry %q", regAlias)
	}

	// Check if already fully installed (all members in lock)
	if !installForce {
		if lockedSS, ok := lock.Skillsets[name]; ok {
			allInstalled := true
			for _, member := range lockedSS.Members {
				parts := strings.SplitN(member, "/", 2)
				if len(parts) != 2 {
					continue
				}
				if !installer.IsInstalled(".", parts[0], parts[1]) {
					allInstalled = false
					break
				}
			}
			if allInstalled {
				ui.Check("skillset %s (%d members) — already installed", name, len(lockedSS.Members))
				return nil
			}
		}
	}

	idx, err := client.FetchIndex(ctx)
	if err != nil {
		return fmt.Errorf("fetching registry index: %w", err)
	}

	skillset, found := idx.Skillsets[name]
	if !found {
		return fmt.Errorf("skillset %q not found in registry %q", name, regAlias)
	}

	// Resolve items from manifest if not inline
	if len(skillset.Items) == 0 {
		ssManifest, err := client.FetchSkillsetManifest(ctx, name, skillset.Latest)
		if err != nil {
			return fmt.Errorf("fetching skillset manifest: %w", err)
		}
		skillset.Items = ssManifest.ToSkillsetItems()
	}

	var digestItems []string
	var memberList []string
	for _, item := range skillset.Items {
		itemType := types.ItemType(item.Type)
		entries := idx.EntriesForType(itemType)
		entry, ok := entries[item.Name]
		if !ok {
			ui.Warn("  %s %q not found in registry, skipping", item.Type, item.Name)
			continue
		}

		version := entry.Latest
		lockEntries := lock.EntriesForType(itemType)

		// Skip if already installed (unless --force)
		if !installForce {
			if _, hasLock := lockEntries[item.Name]; hasLock {
				if installer.IsInstalled(".", item.Type, item.Name) {
					lockVersion := version
					if lockVersion == "" {
						lockVersion = "latest"
					}
					digestItems = append(digestItems, fmt.Sprintf("%s/%s/%s", item.Type, item.Name, lockVersion))
					memberList = append(memberList, fmt.Sprintf("%s/%s", item.Type, item.Name))
					continue
				}
			}
		}

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

		digestItems = append(digestItems, fmt.Sprintf("%s/%s/%s", item.Type, item.Name, lockVersion))
		memberList = append(memberList, fmt.Sprintf("%s/%s", item.Type, item.Name))

		displayVersion := version
		if displayVersion == "" {
			displayVersion = "latest"
		}
		ui.Check("  %s %s@%s", item.Type, item.Name, displayVersion)
	}

	lock.Skillsets[name] = manifest.LockedSkillset{
		Registry:    regAlias,
		Digest:      manifest.SkillsetDigest(digestItems),
		Members:     memberList,
		InstalledAt: "",
	}

	return nil
}

func resolveVersion(ctx context.Context, client registry.Client, itemType, name, constraint string) (string, error) {
	// "latest" means unversioned — download from default branch
	if constraint == "latest" {
		return "", nil
	}

	versions, err := client.ListVersions(ctx, itemType, name)
	if err != nil {
		return "", fmt.Errorf("listing versions: %w", err)
	}

	// No tags found — registry doesn't use per-item version tags.
	// Return empty so DownloadFiles fetches from default branch.
	if len(versions) == 0 {
		return "", nil
	}

	best, err := resolver.Resolve(constraint, versions)
	if err != nil {
		return "", err
	}

	return best.String(), nil
}
