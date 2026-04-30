package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/useamaru/amaru/internal/scaffold"
	"github.com/useamaru/amaru/internal/ui"

	"github.com/spf13/cobra"
)

var (
	repoMigrateDryRun            bool
	repoMigrateAllowDirty        bool
	repoMigrateSkipSparseRewrite bool
	repoMigrateStrictChildren    bool
	repoMigratePushToDefault     bool
	repoMigrateBranch            string
	repoMigrateProtocol          string
)

var repoMigrateCmd = &cobra.Command{
	Use:   "migrate [<url>]",
	Short: "Migrate a registry from the legacy .amaru_registry/ layout to the flat v2 layout",
	Long: `Convert a registry from the legacy nested layout (.amaru_registry/{skills,...}) to
the v2 flat layout (skills/, commands/, agents/, context/, .sparse-profiles/ at the root).

In-place mode (no argument): migrates the registry in the current directory. Stages
file moves, rewrites sparse profiles using an anchored-prefix algorithm, and bumps
amaru_version to "2". Leaves git staging/commit to the user.

Idempotent — running on an already-migrated registry is a no-op. A crash leaves
a .migrating journal at the registry root so a re-run can diagnose the half state.

Remote mode (with <url>): clones the registry to a temp directory, runs the same
in-place migration, commits, and pushes to a 'migration/flat-layout' side branch
(or the default branch with --push-to-default). Pre-push the command verifies
the cloned HEAD is still the remote tip so a concurrent push doesn't get
overwritten. Each git invocation is bounded by a 60s timeout to prevent
credential prompts from hanging the process.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return runRepoMigrateRemote(cmd.Context(), args[0])
		}
		return runRepoMigrate()
	},
}

func init() {
	repoMigrateCmd.Flags().BoolVar(&repoMigrateDryRun, "dry-run", false, "Print planned moves without changing the filesystem (or pushing, in remote mode)")
	repoMigrateCmd.Flags().BoolVar(&repoMigrateAllowDirty, "allow-dirty", false, "Skip the working-tree-clean check")
	repoMigrateCmd.Flags().BoolVar(&repoMigrateSkipSparseRewrite, "skip-sparse-rewrite", false, "Leave .sparse-profiles/* files untouched")
	repoMigrateCmd.Flags().BoolVar(&repoMigrateStrictChildren, "strict-children", false, "Refuse to migrate when .amaru_registry/ contains non-canonical children")
	repoMigrateCmd.Flags().BoolVar(&repoMigratePushToDefault, "push-to-default", false, "Push the migration commit directly onto the default branch (remote mode only)")
	repoMigrateCmd.Flags().StringVar(&repoMigrateBranch, "branch", "", "Override the side branch name (remote mode only; defaults to migration/flat-layout)")
	repoMigrateCmd.Flags().StringVar(&repoMigrateProtocol, "protocol", "", "Clone protocol: ssh (default) or https")
	repoCmd.AddCommand(repoMigrateCmd)
}

func runRepoMigrateRemote(ctx context.Context, url string) error {
	opts := scaffold.RemoteMigrateOptions{
		MigrateOptions: scaffold.MigrateOptions{
			DryRun:            repoMigrateDryRun,
			AllowDirty:        repoMigrateAllowDirty,
			SkipSparseRewrite: repoMigrateSkipSparseRewrite,
			StrictChildren:    repoMigrateStrictChildren,
		},
		Branch:        repoMigrateBranch,
		PushToDefault: repoMigratePushToDefault,
		CloneProtocol: repoMigrateProtocol,
	}
	res, err := scaffold.MigrateRemote(ctx, url, opts)
	if err != nil {
		return err
	}

	if res.InPlaceResult != nil && res.InPlaceResult.Status == scaffold.MigrationStatusAlreadyMigrated {
		ui.Check("Remote registry %s is already at v2 (flat layout) — nothing to push.", res.URL)
		return nil
	}

	if repoMigrateDryRun {
		ui.Check("Dry run — would push commit to branch %q on %s.", res.Branch, res.URL)
		fmt.Printf("  Local clone retained for inspection: %s\n", res.CloneDir)
		return nil
	}

	if res.PushedToDefault {
		ui.Check("Migrated %s and pushed to default branch (commit %s).", res.URL, shortSHA(res.CommitSHA))
	} else {
		ui.Check("Migrated %s and pushed branch %q (commit %s).", res.URL, res.Branch, shortSHA(res.CommitSHA))
		fmt.Println("\n  Next step — open a PR:")
		fmt.Printf("    gh pr create --repo %s --head %s --title \"Migrate registry to flat layout\"\n",
			strings.TrimPrefix(res.URL, "github:"), res.Branch)
	}
	return nil
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}

func runRepoMigrate() error {
	opts := scaffold.MigrateOptions{
		DryRun:            repoMigrateDryRun,
		AllowDirty:        repoMigrateAllowDirty,
		SkipSparseRewrite: repoMigrateSkipSparseRewrite,
		StrictChildren:    repoMigrateStrictChildren,
	}
	res, err := scaffold.MigrateInPlace(".", opts)
	if err != nil {
		return err
	}
	switch res.Status {
	case scaffold.MigrationStatusAlreadyMigrated:
		ui.Check("Registry already at v2 (flat layout) — nothing to do.")
		return nil
	case scaffold.MigrationStatusDryRun:
		fmt.Println("Dry run — the following moves would be performed:")
		for _, m := range res.PlannedMoves {
			fmt.Printf("  %s -> %s\n", m.From, m.To)
		}
		if len(res.NonCanonicalMoves) > 0 {
			fmt.Println("\nNon-canonical children that would move verbatim:")
			for _, m := range res.NonCanonicalMoves {
				fmt.Printf("  %s -> %s\n", m.From, m.To)
			}
		}
		fmt.Println("\nNo files were changed. Re-run without --dry-run to apply.")
		return nil
	case scaffold.MigrationStatusCompleted:
		ui.Check("Migrated to v2 flat layout (%d move(s)).", len(res.PlannedMoves))
		if len(res.SparseRewritten) > 0 {
			ui.Check("Rewrote %d sparse profile(s); originals preserved as .bak files.", len(res.SparseRewritten))
		}
		fmt.Println("\n  Next steps:")
		fmt.Println("    1. Review the diff: git status && git diff")
		fmt.Println("    2. Stage and commit: git add -A && git commit -m \"chore: migrate registry to flat layout\"")
		fmt.Println("    3. Push: git push")
		return nil
	default:
		return fmt.Errorf("unexpected migration status: %v", res.Status)
	}
}
