package scaffold

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/useamaru/amaru/internal/registry"
)

// JournalFile is the name of the migration crash-recovery journal at the
// registry root. It is created before the first rename and removed on
// successful completion. A re-run inspects it to diagnose half-migrated trees.
const JournalFile = ".migrating"

// canonicalChildren is the set of children of .amaru_registry/ that the
// migration knows how to relocate. Anything else is considered a non-named
// child and is moved verbatim unless StrictChildren is set.
var canonicalChildren = []string{
	"skills",
	"commands",
	"agents",
	"context",
	".sparse-profiles",
	"skillsets",
}

// MigrationStatus describes the outcome of MigrateInPlace.
type MigrationStatus int

const (
	// MigrationStatusUnknown is the zero value; should never appear in a returned result.
	MigrationStatusUnknown MigrationStatus = iota
	// MigrationStatusAlreadyMigrated means the registry was already at v2.
	MigrationStatusAlreadyMigrated
	// MigrationStatusCompleted means the migration ran and changed the filesystem.
	MigrationStatusCompleted
	// MigrationStatusDryRun means the migration would have run but --dry-run was set.
	MigrationStatusDryRun
)

// MigrateOptions controls MigrateInPlace behavior.
type MigrateOptions struct {
	// DryRun reports planned moves without changing the filesystem.
	DryRun bool
	// AllowDirty disables the working-tree-clean check (for repos with
	// in-progress edits the user explicitly wants to migrate alongside).
	AllowDirty bool
	// SkipSparseRewrite leaves .sparse-profiles/* files untouched.
	SkipSparseRewrite bool
	// StrictChildren refuses to migrate when .amaru_registry/ contains
	// non-canonical children (e.g. .DS_Store, stray .bak files).
	StrictChildren bool
}

// MigrationMove records a rename the migration performed (or planned).
type MigrationMove struct {
	From string `json:"from"` // relative to the registry root
	To   string `json:"to"`
}

// MigrationResult summarizes what MigrateInPlace did.
type MigrationResult struct {
	Status              MigrationStatus
	PlannedMoves        []MigrationMove
	NonCanonicalMoves   []MigrationMove // children of .amaru_registry/ outside the canonical set
	SparseRewritten     []string        // profile paths whose body was rewritten
	SparseBackupsWriten []string        // .bak files written
	IndexBumped         bool            // amaru_registry.json was rewritten
}

// journalRecord is the on-disk shape of the .migrating file.
type journalRecord struct {
	StartedAt    string          `json:"started_at"`
	PlannedMoves []MigrationMove `json:"planned_moves"`
}

// MigrateInPlace converts a v1 (nested) registry at dir to v2 (flat) in place.
// Idempotent: a v2 registry returns MigrationStatusAlreadyMigrated unchanged.
//
// Pre-flight rejects symlinks, case-insensitive collisions, both-layouts-present,
// and (unless AllowDirty) a dirty git working tree. A journal file at the
// registry root records intent before the first rename and is removed on
// success — a crashed re-run inspects the journal to give an actionable error.
func MigrateInPlace(dir string, opts MigrateOptions) (*MigrationResult, error) {
	dir = filepath.Clean(dir)

	idxPath := filepath.Join(dir, registryIndexFile)
	if _, err := os.Stat(idxPath); err != nil {
		return nil, fmt.Errorf("no %s in %s — not a registry", registryIndexFile, dir)
	}

	state, err := classifyState(dir)
	if err != nil {
		return nil, err
	}

	if action := state.recoveryAction(); action != "" {
		return nil, errors.New(action)
	}

	if state.flatPresent && !state.nestedPresent && !state.journalPresent {
		return &MigrationResult{Status: MigrationStatusAlreadyMigrated}, nil
	}

	// Pre-flight conflict checks (only run when we plan to migrate).
	if err := preflightConflicts(dir); err != nil {
		return nil, err
	}

	// Working-tree-clean check (only when in-place and not --allow-dirty).
	if !opts.AllowDirty {
		if err := requireCleanWorkingTree(dir); err != nil {
			return nil, err
		}
	}

	planned, nonCanonical, err := planMoves(dir, opts.StrictChildren)
	if err != nil {
		return nil, err
	}

	result := &MigrationResult{
		PlannedMoves:      planned,
		NonCanonicalMoves: nonCanonical,
	}

	if opts.DryRun {
		result.Status = MigrationStatusDryRun
		return result, nil
	}

	// Write journal before any rename.
	if err := writeJournal(dir, planned); err != nil {
		return nil, fmt.Errorf("writing migration journal: %w", err)
	}

	// Perform moves. Order: canonical first, then non-canonical (preserves
	// debuggability — if we crash, the canonical layout exists fully or not at all).
	for _, m := range planned {
		fromAbs := filepath.Join(dir, m.From)
		toAbs := filepath.Join(dir, m.To)
		if err := os.Rename(fromAbs, toAbs); err != nil {
			return nil, fmt.Errorf("moving %s -> %s: %w (journal preserved at %s for recovery)",
				m.From, m.To, err, filepath.Join(dir, JournalFile))
		}
	}

	// Sparse-profile rewrite (after .sparse-profiles itself has moved to the root).
	if !opts.SkipSparseRewrite {
		rewritten, backups, err := rewriteSparseProfiles(dir)
		if err != nil {
			return nil, fmt.Errorf("rewriting sparse profiles: %w (journal preserved for recovery)", err)
		}
		result.SparseRewritten = rewritten
		result.SparseBackupsWriten = backups
	}

	// Atomic index rewrite to flip amaru_version to "2".
	if err := bumpIndexToV2(dir); err != nil {
		return nil, fmt.Errorf("rewriting %s: %w (journal preserved for recovery — manual fix: set amaru_version to \"2\")",
			registryIndexFile, err)
	}
	result.IndexBumped = true

	// Remove the now-empty .amaru_registry/ directory (if it survives).
	_ = os.Remove(filepath.Join(dir, registry.NestedRoot))

	// Remove journal last — at this point the migration is complete.
	if err := os.Remove(filepath.Join(dir, JournalFile)); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("removing migration journal: %w", err)
	}

	result.Status = MigrationStatusCompleted
	return result, nil
}

// migrationState is the (journal, nested, flat) tuple from the recovery matrix.
type migrationState struct {
	journalPresent bool
	nestedPresent  bool
	flatPresent    bool
}

func classifyState(dir string) (migrationState, error) {
	st := migrationState{}

	if _, err := os.Lstat(filepath.Join(dir, JournalFile)); err == nil {
		st.journalPresent = true
	}

	if info, err := os.Lstat(filepath.Join(dir, registry.NestedRoot)); err == nil && info.IsDir() {
		st.nestedPresent = true
	}

	for _, child := range canonicalChildren {
		if info, err := os.Lstat(filepath.Join(dir, child)); err == nil && info.IsDir() {
			st.flatPresent = true
			break
		}
	}

	return st, nil
}

// recoveryAction returns a non-empty error message when the current state is
// recoverable-by-user-action. Empty string means the state is OK to proceed
// (either ready-to-migrate or already-migrated; the caller decides which).
func (s migrationState) recoveryAction() string {
	switch {
	case s.journalPresent && s.nestedPresent && s.flatPresent:
		return fmt.Sprintf(
			"migration journal at .migrating indicates a previous run did not complete. " +
				"Inspect the journal, finish or revert the partial moves, remove the journal file, and re-run.")
	case s.journalPresent && s.nestedPresent && !s.flatPresent:
		return "migration journal exists but no flat directories were created — previous run crashed before any move. " +
			"Remove .migrating manually and re-run."
	case s.journalPresent && !s.nestedPresent && s.flatPresent:
		return "migration journal is stale (registry is already flat). Remove .migrating and re-run if needed."
	case s.journalPresent && !s.nestedPresent && !s.flatPresent:
		return "migration journal exists but the registry has neither layout. Remove .migrating manually."
	case !s.journalPresent && s.nestedPresent && s.flatPresent:
		return "both legacy (.amaru_registry/) and flat (skills/, commands/, ...) directories exist. " +
			"Resolve the collision manually before running migrate."
	case !s.journalPresent && !s.nestedPresent && !s.flatPresent:
		return "no registry layout detected (neither .amaru_registry/ nor top-level skills/commands/agents/context/.sparse-profiles found)"
	}
	return ""
}

// preflightConflicts rejects symlinks at any candidate path and case-insensitive
// collisions on macOS/Windows. Called only when classifyState returned a
// "ready to migrate" tuple (nested without flat, no journal).
func preflightConflicts(dir string) error {
	for _, child := range canonicalChildren {
		nested := filepath.Join(dir, registry.NestedRoot, child)
		flat := filepath.Join(dir, child)

		if info, err := os.Lstat(nested); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to migrate: %s is a symlink (resolve manually first)", filepath.Join(registry.NestedRoot, child))
		}
		// Note: we already know flat isn't present as a directory (classifyState
		// returned ready-to-migrate). But it could still be a symlink or file.
		if info, err := os.Lstat(flat); err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("refusing to migrate: target %s is a symlink", child)
			}
			if !info.IsDir() {
				return fmt.Errorf("refusing to migrate: target %s exists as a non-directory", child)
			}
		}

		// Case-insensitive collision detection: on macOS/Windows the FS may
		// already have <Dir>/ at root which would collide with skills/ etc.
		if collides, name := caseInsensitiveCollision(dir, child); collides {
			return fmt.Errorf("refusing to migrate: case-insensitive collision between target %s and existing %s", child, name)
		}
	}

	// Also reject a symlink at .amaru_registry/ itself.
	if info, err := os.Lstat(filepath.Join(dir, registry.NestedRoot)); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to migrate: %s is a symlink (resolve manually first)", registry.NestedRoot)
	}

	return nil
}

// caseInsensitiveCollision returns true if the registry root contains a
// directory whose lowercase name equals child but whose actual name differs.
// This catches macOS/Windows cases where Skills/ would collide with skills/.
func caseInsensitiveCollision(dir, child string) (bool, string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, ""
	}
	target := strings.ToLower(child)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == child {
			continue // exact match, not a collision
		}
		if strings.EqualFold(name, target) {
			return true, name
		}
	}
	return false, ""
}

// requireCleanWorkingTree refuses if `git status --porcelain` returns output.
// If dir is not inside a git repo, this is a no-op.
func requireCleanWorkingTree(dir string) error {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		// Not a git repo — nothing to check.
		return nil
	}
	cmd = exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("checking working tree: %w", err)
	}
	if strings.TrimSpace(string(out)) != "" {
		return errors.New("working tree has uncommitted changes — commit/stash first or pass --allow-dirty")
	}
	return nil
}

// planMoves enumerates the rename operations needed to flip the layout.
// Children outside canonicalChildren are returned separately so the caller can
// decide whether to refuse (StrictChildren) or move them verbatim.
func planMoves(dir string, strictChildren bool) ([]MigrationMove, []MigrationMove, error) {
	nestedAbs := filepath.Join(dir, registry.NestedRoot)
	entries, err := os.ReadDir(nestedAbs)
	if err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", registry.NestedRoot, err)
	}

	canonical := map[string]bool{}
	for _, c := range canonicalChildren {
		canonical[c] = true
	}

	var moves []MigrationMove
	var nonCanonical []MigrationMove
	for _, e := range entries {
		from := registry.NestedRoot + "/" + e.Name()
		to := e.Name()
		m := MigrationMove{From: from, To: to}
		if canonical[e.Name()] {
			moves = append(moves, m)
		} else {
			nonCanonical = append(nonCanonical, m)
		}
	}

	if strictChildren && len(nonCanonical) > 0 {
		var names []string
		for _, m := range nonCanonical {
			names = append(names, m.From)
		}
		sort.Strings(names)
		return nil, nil, fmt.Errorf("refusing to migrate: %s contains non-canonical children: %s (run without --strict-children to move them verbatim)",
			registry.NestedRoot, strings.Join(names, ", "))
	}

	// Sort canonical moves deterministically so the journal is stable.
	sort.Slice(moves, func(i, j int) bool { return moves[i].From < moves[j].From })
	sort.Slice(nonCanonical, func(i, j int) bool { return nonCanonical[i].From < nonCanonical[j].From })

	// Append non-canonical to the actual move list (they still need to move).
	moves = append(moves, nonCanonical...)
	return moves, nonCanonical, nil
}

func writeJournal(dir string, planned []MigrationMove) error {
	rec := journalRecord{
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		PlannedMoves: planned,
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, JournalFile), data, 0644)
}

// rewriteSparseProfiles applies the anchored-prefix rewrite to every file in
// .sparse-profiles/ at the registry root (which has just been moved out of
// .amaru_registry/). Lines beginning (after optional `!` and whitespace) with
// `.amaru_registry/context/<project>/` get the prefix replaced with
// `context/<project>/`. The first rewrite of any profile writes a `.bak`
// alongside the original.
func rewriteSparseProfiles(dir string) ([]string, []string, error) {
	profilesDir := filepath.Join(dir, ".sparse-profiles")
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	var rewritten []string
	var backups []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(profilesDir, e.Name())
		original, err := os.ReadFile(path)
		if err != nil {
			return rewritten, backups, fmt.Errorf("reading %s: %w", path, err)
		}
		newContent, changed := rewriteSparseProfileBody(string(original))
		if !changed {
			continue
		}
		bakPath := path + ".bak"
		if _, err := os.Stat(bakPath); os.IsNotExist(err) {
			if err := os.WriteFile(bakPath, original, 0644); err != nil {
				return rewritten, backups, fmt.Errorf("writing backup %s: %w", bakPath, err)
			}
			backups = append(backups, bakPath)
		}
		if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
			return rewritten, backups, fmt.Errorf("writing %s: %w", path, err)
		}
		rewritten = append(rewritten, path)
	}
	return rewritten, backups, nil
}

// rewriteSparseProfileBody applies the anchored line-prefix rule. Returns
// the new body and whether anything changed.
func rewriteSparseProfileBody(body string) (string, bool) {
	const oldPrefix = ".amaru_registry/context/"
	const newPrefix = "context/"

	lines := strings.Split(body, "\n")
	changed := false
	for i, line := range lines {
		// Identify the first non-whitespace character; allow optional leading `!`.
		trimmed := strings.TrimLeft(line, " \t")
		negated := strings.HasPrefix(trimmed, "!")
		body := trimmed
		if negated {
			body = strings.TrimLeft(trimmed[1:], " \t")
		}
		if !strings.HasPrefix(body, oldPrefix) {
			continue
		}
		// Reconstruct the prefix portion that came before `body` in `line`.
		prefixLen := len(line) - len(body)
		newLine := line[:prefixLen] + newPrefix + body[len(oldPrefix):]
		lines[i] = newLine
		changed = true
	}
	if !changed {
		return body, false
	}
	return strings.Join(lines, "\n"), true
}

// bumpIndexToV2 atomically rewrites amaru_registry.json with amaru_version: "2",
// preserving every other field. Uses the existing temp-and-rename pattern.
func bumpIndexToV2(dir string) error {
	idx, err := LoadLocalIndex(dir)
	if err != nil {
		return err
	}
	idx.AmaruVersion = "2"
	return SaveLocalIndex(dir, idx)
}

// LayoutFromIndex is a thin convenience around registry.LayoutFor for callers
// that already loaded the index via LoadLocalIndex. Kept here for symmetry
// with the migrate package's API surface.
func LayoutFromIndex(idx *registry.RegistryIndex) (registry.Layout, error) {
	return registry.LayoutFor(idx)
}
