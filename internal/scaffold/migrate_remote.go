package scaffold

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/useamaru/amaru/internal/registry"
)

// DefaultMigrationBranch is the side branch the remote migrate pushes to
// by default. Avoids forcing a layout migration directly onto a registry's
// default branch where it can race with concurrent work.
const DefaultMigrationBranch = "migration/flat-layout"

// RemoteMigrateOptions controls MigrateRemote behavior.
type RemoteMigrateOptions struct {
	MigrateOptions

	// Branch is the side branch to create and push. Empty means DefaultMigrationBranch.
	Branch string
	// PushToDefault pushes onto the cloned default branch instead of a side branch.
	PushToDefault bool
	// CommitMessage overrides the default migration commit message.
	CommitMessage string
	// Runner injects a GitRunner. Defaults to NewRealGitRunner.
	Runner GitRunner
	// CloneProtocol selects "ssh" or "https". Empty = SSH with HTTPS fallback.
	CloneProtocol string
}

// RemoteMigrateResult summarizes a remote migration run.
type RemoteMigrateResult struct {
	URL           string
	Branch        string // branch that was pushed (or would be on dry-run)
	CloneDir      string
	CommitSHA     string                  // empty if no commit was created
	InPlaceResult *MigrationResult        // result of the embedded in-place migration
	Pushed        bool
	PushedToDefault bool
}

// MigrateRemote clones a registry repo, runs MigrateInPlace, and pushes the
// result to a side branch (or the default branch with --push-to-default).
//
// Auth uses whatever git is configured to use on this machine (gh CLI, SSH,
// or credential helper). The function installs a SIGINT/SIGTERM handler that
// removes the tempdir before exit so a Ctrl-C doesn't leak credentials in
// .git/config. Each git invocation is bounded by GitInvocationTimeout.
func MigrateRemote(ctx context.Context, url string, opts RemoteMigrateOptions) (*RemoteMigrateResult, error) {
	canonical, err := registry.NormalizeURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid registry URL: %w", err)
	}
	cloneURL := buildCloneURL(canonical, opts.CloneProtocol)

	runner := opts.Runner
	if runner == nil {
		runner = NewRealGitRunner()
	}
	branch := opts.Branch
	if branch == "" {
		branch = DefaultMigrationBranch
	}
	commitMsg := opts.CommitMessage
	if commitMsg == "" {
		commitMsg = "chore: migrate registry to flat layout (amaru repo migrate)"
	}

	cloneDir, err := os.MkdirTemp("", "amaru-migrate-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	cleanup := installSignalCleanup(cloneDir)
	defer cleanup()

	res := &RemoteMigrateResult{URL: canonical, Branch: branch, CloneDir: cloneDir}

	// 1. Clone (shallow not used — we need full history for ancestry checks).
	if _, err := runGit(ctx, runner, "", "clone", cloneURL, cloneDir); err != nil {
		return res, fmt.Errorf("clone failed: %w", err)
	}

	// 2. Record the cloned default-branch tip so we can verify it hasn't moved before push.
	clonedHead, err := runGit(ctx, runner, cloneDir, "rev-parse", "HEAD")
	if err != nil {
		return res, fmt.Errorf("recording clone HEAD: %w", err)
	}
	defaultBranch, err := runGit(ctx, runner, cloneDir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return res, fmt.Errorf("recording default branch: %w", err)
	}

	// 3. Decide whether to checkout a side branch.
	if !opts.PushToDefault {
		if _, err := runGit(ctx, runner, cloneDir, "checkout", "-b", branch); err != nil {
			return res, fmt.Errorf("creating side branch: %w", err)
		}
	}

	// 4. Run the in-place migration.
	migRes, err := MigrateInPlace(cloneDir, opts.MigrateOptions)
	if err != nil {
		return res, fmt.Errorf("migration failed: %w", err)
	}
	res.InPlaceResult = migRes

	if migRes.Status == MigrationStatusAlreadyMigrated {
		// Nothing to commit or push.
		return res, nil
	}

	// 5. Stage and commit.
	if _, err := runGit(ctx, runner, cloneDir, "add", "-A"); err != nil {
		return res, fmt.Errorf("git add -A: %w", err)
	}

	// Refuse to push if nothing actually changed (defensive — MigrateInPlace
	// only reports Completed when something moved).
	if status, _ := runGit(ctx, runner, cloneDir, "status", "--porcelain"); status == "" && migRes.Status == MigrationStatusCompleted {
		return res, fmt.Errorf("internal error: migration reported Completed but git status is clean")
	}

	if opts.DryRun {
		// Dry run still ran the migration locally inside the tempdir; print plan, don't push.
		return res, nil
	}

	if _, err := runGit(ctx, runner, cloneDir, "commit", "-m", commitMsg); err != nil {
		return res, fmt.Errorf("git commit: %w", err)
	}
	commitSHA, err := runGit(ctx, runner, cloneDir, "rev-parse", "HEAD")
	if err != nil {
		return res, fmt.Errorf("recording commit SHA: %w", err)
	}
	res.CommitSHA = commitSHA

	// 6. Pre-push fetch + ancestry check on the default branch.
	if _, err := runGit(ctx, runner, cloneDir, "fetch", "origin", defaultBranch); err != nil {
		return res, fmt.Errorf("git fetch origin %s: %w", defaultBranch, err)
	}
	remoteTip, err := runGit(ctx, runner, cloneDir, "rev-parse", "origin/"+defaultBranch)
	if err != nil {
		return res, fmt.Errorf("reading origin/%s: %w", defaultBranch, err)
	}
	if remoteTip != clonedHead {
		return res, fmt.Errorf(
			"remote %s has moved since clone (cloned %s, now %s) — re-run amaru repo migrate against the latest state",
			defaultBranch, shortSHA(clonedHead), shortSHA(remoteTip))
	}

	// 7. Push.
	pushTarget := branch
	if opts.PushToDefault {
		pushTarget = defaultBranch
	}
	if _, err := runGit(ctx, runner, cloneDir, "push", "-u", "origin", pushTarget); err != nil {
		return res, fmt.Errorf("git push origin %s: %w", pushTarget, err)
	}
	res.Pushed = true
	res.PushedToDefault = opts.PushToDefault
	return res, nil
}

// buildCloneURL converts a canonical github:org/repo URL into a clone URL.
// Selects SSH by default (works with gh CLI auth and SSH keys); HTTPS when
// requested explicitly.
func buildCloneURL(canonical, protocol string) string {
	// canonical is "github:org/repo" — strip the prefix.
	const prefix = "github:"
	body := strings.TrimPrefix(canonical, prefix)
	switch protocol {
	case "https":
		return "https://github.com/" + body + ".git"
	case "ssh", "":
		return "git@github.com:" + body + ".git"
	default:
		// Unknown protocol — fall back to HTTPS for safety.
		return "https://github.com/" + body + ".git"
	}
}

// installSignalCleanup arranges to remove cloneDir if the process is
// interrupted by SIGINT or SIGTERM. Returns a deferred cleanup function the
// caller must invoke on normal exit (idempotent — safe to double-call).
func installSignalCleanup(cloneDir string) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})

	go func() {
		select {
		case <-sigCh:
			_ = os.RemoveAll(cloneDir)
			os.Exit(130) // 128 + SIGINT
		case <-done:
			return
		}
	}()

	return func() {
		signal.Stop(sigCh)
		close(done)
		_ = os.RemoveAll(cloneDir)
	}
}

func shortSHA(sha string) string {
	if len(sha) > 8 {
		return sha[:8]
	}
	return sha
}
