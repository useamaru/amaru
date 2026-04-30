---
title: "Flat registry layout, foreign-registry installs, mirror UX, and version cascading"
type: feat
status: active
date: 2026-04-30
deepened: 2026-04-30
---

# Flat registry layout, foreign-registry installs, mirror UX, and version cascading

**Target repo:** `amaru` (`github:useamaru/amaru`). All paths in this plan are repo-relative to amaru's root.

## Overview

Amaru registries currently store all installable content under `.amaru_registry/{skills,commands,agents,context}/`. This plan flips the layout so content lives at the top level (`skills/`, `commands/`, `agents/`, `context/`), matching the convention used by other Claude-Code skill registries (e.g. `vercel-labs/agent-skills`, `jeremylongshore/claude-code-plugins-plus-skills`). Alongside that, the plan ships:

- A `repo migrate` command with two modes: in-place (`amaru repo migrate`) and remote (`amaru repo migrate <url>`, which clones + migrates + pushes a `migration/flat-layout` branch).
- Broader foreign-registry support so amaru can install from non-amaru-native repos (skills, commands, agents).
- A `--cascade` flag on `repo tag` that, in addition to tagging the item itself, bumps the `Latest` of every skillset entry that contains the item.
- Mirror UX: the `mirrors` field already merges; this plan surfaces mirror provenance in `browse`/`list` and verifies the path end-to-end.

## Problem Frame

Amaru's `.amaru_registry/` namespace was introduced so any repository could double as its own registry without colliding with the host repo's own source layout. In practice this has produced friction:

- Other Claude-Code registries use `skills/` at the root; amaru users can't drop their existing repo onto amaru without restructuring.
- Foreign registries are partially supported (`formatVercel` detects `skills/*/SKILL.md`) but only for skills, and only with uppercase `SKILL.md` filenames.
- Authoring a skill via amaru means burying it under a leading-dot directory that some tools (and IDE search) hide by default.
- When a skill bumps version, every skillset that includes it semantically changes, but nothing in amaru helps the maintainer keep those in sync.
- Mirror support exists in code (`Mirrors []string`, `MergeFrom`) but the UX never tells the user where a merged entry came from, so it's effectively invisible.

## Requirements Trace

- **R1.** Amaru-native registries store installable content at the repo root (`skills/`, `commands/`, `agents/`, `context/`), with `amaru_registry.json` still at the root.
- **R2.** Amaru can read the legacy `.amaru_registry/` layout for backward compatibility (decision: read both, write only flat — see Key Technical Decisions).
- **R3.** A new `amaru repo migrate` command converts a local registry from legacy layout to flat layout in place.
- **R4.** `amaru repo migrate <url>` clones a remote registry, migrates it, commits, and pushes back.
- **R5.** Amaru can install skills, commands, and agents from non-amaru registries (no `amaru_registry.json`) that follow the convention `<type>s/<name>/...` with either `SKILL.md`/`COMMAND.md`/`AGENT.md` or lowercase variants.
- **R6.** Mirrors (declared in `amaru_registry.json` via the existing `mirrors` field) work end-to-end against both amaru-native and foreign registries, and mirrored entries are visibly tagged in `amaru browse` and `amaru list`.
- **R7.** A new `--cascade` flag on `amaru repo tag <name> <version>` updates the item's `manifest.json` and the `Latest` field in the index (existing behavior), and additionally cascades a patch-level version bump into every skillset entry that contains the item.

## Scope Boundaries

- Not redesigning the lock-file shape, manifest spec, or version-resolution algorithm.
- Not changing the `.claude/{skills,commands,agents}/` install layout in consumer projects.
- Not adding new auth methods. Mirrors stay unauthenticated for now (token-inheriting auth deferred).
- Not adding ad-hoc mirror declarations in `amaru.json` (deferred — mirrors only come from the registry's own index).
- Not building a generic version-bump engine. `repo tag --cascade` cascades only into skillsets that already include the item.
- Not attempting to keep both layouts in lock-step. Once a registry migrates, it stays migrated. Old consumers running an old amaru against a new registry break — they need to upgrade amaru.

## Context & Research

### Relevant Code and Patterns

- `internal/registry/registry.go:11-44` — `RegistryIndex` already carries an `AmaruVersion` string and a `Mirrors []string` field plus a `MergeFrom` helper.
- `internal/registry/github.go:30-36, 261-303, 322-398, 485-510` — Existing format detection (`formatAmaru` vs `formatVercel`), mirror resolution via `mirrorFactory`, Vercel-mode index synthesis from `skills/*/SKILL.md`, and amaru-mode `DownloadFiles` / `FetchSkillsetManifest` that hardcode `.amaru_registry/<dir>/<name>` paths.
- `internal/scaffold/scaffold.go:18-78` — Scaffolding hardcodes `.amaru_registry/{skills,commands,agents,context,.sparse-profiles}` directory creation and the sparse profile content.
- `cmd/repo_add.go:99`, `cmd/repo_info.go:70,100-101`, `cmd/repo_remove.go:87`, `cmd/repo_tag.go:78`, `cmd/repo_validate.go:92,172`, `cmd/repo.go:63-66` — Every `repo` subcommand builds paths via `filepath.Join(dir, ".amaru_registry", itemType.DirName(), name)`. These are the call-sites that need to consult a layout helper instead of a hardcoded string.
- `internal/ctxdocs/ctxdocs.go:64,94,137` — Context sync also hardcodes `.amaru_registry/context/<project>`. Sparse profile body uses `.amaru_registry/context/<project>/**` (see `scaffold.go:209-220`).
- `cmd/helpers.go:30-71` — `buildClients` already wires a mirror factory into every primary-registry client.
- `internal/registry/registry.go:78-82` — `SkillsetItem` has only `Type`/`Name`, no version — confirms that a member version bump invalidates the skillset semantically without changing the items list.

### Institutional Learnings

- No relevant `docs/solutions/` entries on registry layout or migrations were found in this repo. The plans dated `2026-03-*` cover skillsets, URL parsing, and repo-management commands; layout is untouched.

### External References

- Vercel Labs "agent-skills" repo and Jeremy Longshore's `claude-code-plugins-plus-skills` repo are the conventional shape: `skills/<name>/SKILL.md` (uppercase) at the top level. Detection in this plan accepts both `SKILL.md` and `skill.md` to cover both Anthropic-pattern and amaru-native authoring.
- Anthropic's published Claude Code plugin/skill conventions use uppercase filenames; amaru's authoring template uses lowercase. The format-detection logic should remain liberal on read.

## Key Technical Decisions

- **`amaru_version` is the layout marker, and *only* the layout marker.** Bump it from `"1"` (legacy `.amaru_registry/`) to `"2"` (flat). Reads honor both; writes always emit `"2"`. Future data-shape evolution (new manifest fields, lockfile changes, alternative skillset storage) gets a separate field — provisionally `schema_version` — when first needed. Rationale: collapsing layout and schema onto the same monotonic counter would force an awkward bump on every future change and re-trigger layout detection unnecessarily. Documenting the distinction now prevents that lock-in. Unit 1 calls out unknown layout values as a hard error so older amarus fail loudly against future v3 (or whatever) registries.

- **`c.format` is a `(layout, source)` pair, not a single enum.** `layout` ∈ {`nested`, `flat`} controls path math. `source` ∈ {`indexed`, `synthesized`} controls whether `FetchIndex` parses `amaru_registry.json` or builds an index from a directory walk. The two dimensions are orthogonal: amaru-native v2 is `(flat, indexed)`, foreign repos like `vercel-labs/agent-skills` are `(flat, synthesized)`, legacy amaru is `(nested, indexed)`. Rationale: collapsing them onto one constant would mean every downstream branch re-reads "is this amaru-with-skillsets or foreign-without?" — the orthogonal split keeps each call site narrow.

- **Read both layouts, write only flat.** Selected over hard-break and auto-migrate-on-touch. Rationale: no consumer breaks the day they upgrade amaru; `repo migrate` is the single, explicit off-ramp; the dual-read code is small and lives in one helper. Auto-migrate on `repo` was rejected because it conflates a one-time destructive operation with normal authoring flow. **Removal milestone:** v1 reads emit a stderr deprecation warning starting in the release that ships this plan, and are removed in the second release after `repo migrate <url>` lands in stable. The Documentation section commits to that timeline.

- **Centralize layout-aware paths in a new `internal/scaffold/layout.go`.** Every direct `filepath.Join(dir, ".amaru_registry", ...)` becomes `layout.ItemDir(idx, dir, itemType, name)` (or similar). Single source of truth, easy to test.

- **Foreign-registry detection breadth.** The (`flat`, `synthesized`) source-mode handles registries missing `amaru_registry.json`. Scan `skills/`, `commands/`, and `agents/` and accept either uppercase or lowercase content filenames (`SKILL.md`/`skill.md`, etc.). Foreign registries with no index fall to "latest" semantics — same as today.

- **`repo migrate` is git-aware but not git-noisy.** Local mode: move files, rewrite `amaru_registry.json` (`amaru_version: "2"`) atomically, rewrite sparse profiles with a documented anchored-prefix algorithm (see Unit 3), leave staging/commit to the user; offer `--dry-run` and `--allow-dirty`. Remote mode: clone to a temp dir, run the same in-process migration, `git add -A`, commit with a fixed message, push to a `migration/flat-layout` branch (not the default branch — see next bullet). Both modes are idempotent (re-running on a v2 registry is a no-op with a "already migrated" message). Both modes write a `.amaru_registry/.migrating` journal file before the first rename and remove it on success so a crashed run can be diagnosed.

- **`repo migrate <url>` pushes to a side branch, not the default branch.** Default behavior: create `migration/flat-layout`, push it, print a `gh pr create` next-step. Rationale: pushing a layout migration directly to a registry's `main` races with humans and silently mixes the migration into unrelated work. A side branch gives reviewers a natural rollback point. `--push-to-default` is an explicit opt-in for solo maintainers who don't want a PR. Before push, the command runs `git fetch` and verifies the cloned HEAD is still an ancestor of the remote's branch tip; if it has moved, the command refuses and prints the recovery steps.

- **`repo migrate <url>` uses git, not the GitHub API.** Pushing repo content via the contents API is awkward and rate-limited; cloning gives us a working tree to manipulate. Auth uses whatever git is configured to use on this machine (gh CLI / SSH / credential helper). The plan shells out to `git clone`/`commit`/`push` via a `gitRunner` interface that's mockable in tests, with a 60-second per-invocation timeout to prevent credential-helper prompts from hanging the process indefinitely.

- **`repo tag --cascade` (single command, default off) replaces a separate `repo bump`.** The cascading and non-cascading paths share most of their machinery; splitting them into two commands would create a UX cliff (`tag` vs. `bump --no-cascade` would be indistinguishable to users). Cascade walks `idx.Skillsets` and patch-bumps every skillset entry whose `Items` include the tagged item. Rationale for default off: tagging an item shouldn't surprise-bump skillsets the user didn't open. Users opt in with `--cascade` when they explicitly want the cascade. The cascade only touches the in-index `Latest`; it does not create individual git tags for each skillset.

- **Mirror provenance via a `Source` field on `RegistryEntry` / `SkillsetEntry`, with documented merge semantics.** `Source string \`json:"-"\`` (non-serialized, runtime-only). `MergeFrom` populates it only for entries copied *from* the mirror — primary entries keep `Source = ""`, and primary-wins collisions never overwrite an existing primary's empty `Source`. Auth-aware mirrors and ad-hoc mirror declaration in `amaru.json` are deferred.

- **Self-hosted amaru migrates itself in this plan.** This repo's own `.amaru_registry/skills/amaru-usage/` moves to `skills/amaru-usage/` so amaru continues to ship its own usage skill in the new layout. Done as part of the docs-sync unit, after migrate is implemented and tested. The migration commit must be reviewable as a plain-file-move diff; if `repo migrate` misbehaves at self-host time, an equivalent `git mv` is acceptable and the plan does not prevent the maintainer from using it.

## Open Questions

### Resolved During Planning

- **Should the legacy reader also be migrated to use `layout.go`?** Yes — same helper, parametrized on `amaru_version`. No second code path.
- **Does the `--cascade` flag create git tags for skillsets?** No. `repo tag` only creates the per-item git tag (`skill/foo/<version>`); the cascade only touches in-index `Latest` for skillset entries. `amaru` doesn't currently support skillset version git tags, so adding them isn't part of this plan.
- **Does `repo migrate` rewrite per-item git tags (e.g. `skill/foo/1.0.0` to point at the new path)?** No. Tags reference commits, not paths; nothing breaks. Confirmed by re-reading `cmd/repo_tag.go:91-141` — only the index/manifest are committed, and the tag points at the resulting commit.
- **What gets installed from a foreign registry that has no manifests?** Same as today: files under `<type>s/<name>/` are downloaded as-is, hash is computed, lock entry uses `"latest"` since `ListVersions` returns nil. Description comes from the content file's frontmatter.
- **Sparse-profile rewrite algorithm.** Anchored line-prefix replace: a line matches when, after stripping an optional leading `!` (negation) and any whitespace, it begins with `.amaru_registry/context/<project>/`. That prefix becomes `context/<project>/`. Lines that contain the legacy string elsewhere (mid-path, in comments) are left alone. The first run of the migration writes `<profile>.bak` next to each modified profile so users have a recovery point. A `--skip-sparse-rewrite` flag bypasses the rewrite entirely for users with truly bespoke profiles.
- **Should `repo migrate <url>` push to the default branch?** No — pushes to `migration/flat-layout` by default; `--push-to-default` is opt-in. See Key Technical Decisions for rationale.
- **Should there be a `repo migrate --reverse`?** No. Once the side-branch decision in Unit 4 is in place, rollback is `git revert` of the migration commit (or simply not merging the PR). The Documentation/Operational Notes section spells out the exact runbook for in-place mode.

### Deferred to Implementation

- Exact API for `layout.ItemDir` / `layout.ContextDir` (struct receiver vs. free function) — depends on how cleanly it threads through ctxdocs and the repo subcommands.
- Whether `repo migrate <url>`'s commit message should include a Co-Authored-By trailer — implementer judgment based on whether the project standardizes that.
- **Should the `Client` interface be narrowed to layout-agnostic primitives (e.g. `DownloadDir(relPath)` / `GetFile(relPath)`), pushing path resolution out to callers?** Doing so would prevent every future `Client` implementation from re-implementing v1/v2 forks; today there is only one implementation, so the duplication tax is hypothetical. The implementer should make the call when wiring `layout.go` into `GitHubClient`: if the layout branching inside `DownloadFiles`/`FetchSkillsetManifest` ends up touching more than two call sites or grows beyond a couple of conditional lines, narrow the interface; otherwise leave it as-is and revisit when a second backend appears. Either choice is acceptable for this plan.

## Implementation Units

- [ ] **Unit 1: Layout-aware path resolver and dual-format reads**

**Goal:** Introduce a single source of truth for resolving registry-content paths from an `amaru_registry.json`'s `amaru_version`, and make every reader call through it. After this unit, amaru can read both legacy and flat layouts but still scaffolds/writes legacy layout.

**Requirements:** R1, R2, R5

**Dependencies:** None.

**Files:**
- Create: `internal/scaffold/layout.go`
- Create: `internal/scaffold/layout_test.go`
- Modify: `internal/registry/registry.go` (add helper to compute layout from `AmaruVersion`)
- Modify: `internal/registry/github.go` (replace hardcoded `.amaru_registry/` strings in `DownloadFiles`, `FetchSkillsetManifest`; replace the `c.format` enum with the `(layout, source)` pair; the `synthesized`-source breadth lands in Unit 6, but the dimensional split happens here)
- Modify: `cmd/repo_add.go`, `cmd/repo_info.go`, `cmd/repo_remove.go`, `cmd/repo_tag.go`, `cmd/repo_validate.go`, `cmd/repo.go`
- Modify: `internal/ctxdocs/ctxdocs.go` (context path resolution)
- Test: `internal/scaffold/layout_test.go`, plus regression tests in `internal/registry/github_test.go` and `cmd/repo_test.go`

**Approach:**
- `layout.go` exposes pure helpers: `ItemDir(idx, root, itemType, name)`, `ContextDir(idx, root, project)`, `SparseProfilePath(idx, root, project)`, `SkillsetManifestPath(idx, root, name)`. Each consults `idx.AmaruVersion` (default `"1"` when absent or empty for back-compat).
- `RegistryIndex` gains a small method `LayoutVersion()` returning `1` or `2` to keep callers off magic-string comparisons. Unknown values return an error rather than silently defaulting.
- `c.format` is replaced by a `(layout, source)` pair: `layout` ∈ {`nested`, `flat`}, `source` ∈ {`indexed`, `synthesized`}. `FetchIndex` populates both: success on `amaru_registry.json` → `(layout from amaru_version, indexed)`; 404 on the index but `skills/`-style tree → `(flat, synthesized)`. Downstream callers branch on the dimension they care about and never read both unnecessarily.
- When the loaded layout is `nested`, log a one-line stderr deprecation warning ("registry uses legacy layout — run `amaru repo migrate` before <removal release>") so the v1-removal milestone has a forcing function. The warning is suppressed by `--quiet` (used by the session-start hook) so it doesn't become noise.
- Open implementation choice (see Open Questions): whether to narrow `Client.DownloadFiles` / `FetchSkillsetManifest` to take pre-resolved relative paths, pushing layout branching out of the GitHub client. The implementer makes the call when wiring layout into `GitHubClient`.

**Patterns to follow:**
- `internal/scaffold/index.go` for atomic JSON read/write style.
- `internal/types/types.go` for tiny enum-with-helpers shape.

**Test scenarios:**
- Happy path: helper returns `"<root>/.amaru_registry/skills/foo"` when `amaru_version: "1"`, `"<root>/skills/foo"` when `"2"`.
- Edge case: empty/missing `amaru_version` defaults to `"1"`.
- Edge case: unknown `amaru_version` (e.g. `"99"`) returns an error from the helper rather than silently defaulting.
- Edge case: `(layout, source)` pair correctly classifies (a) v2 index → `(flat, indexed)`, (b) v1 index → `(nested, indexed)`, (c) no index, top-level skills/ → `(flat, synthesized)`, (d) no index and no skills/ → error.
- Integration: `GitHubClient.DownloadFiles` against a mocked v2 index downloads from `skills/foo/`, against a v1 index downloads from `.amaru_registry/skills/foo/`.
- Integration: `repo validate` on a v1 registry still passes; on a v2 registry built by hand also passes (the `repo` commands in this unit only need to *read* v2; writes still target v1 paths).
- Integration: loading a v1 registry emits exactly one stderr deprecation warning per process lifetime; `--quiet` suppresses it.
- Regression: existing `cmd/repo_test.go` cases continue to pass with `.amaru_registry/` writes (we haven't flipped scaffold yet).

**Verification:** All existing tests pass. New `layout_test.go` covers v1, v2, and unknown. `go vet ./...` clean.

- [ ] **Unit 2: Scaffold and `repo` subcommands write flat layout**

**Goal:** Flip every writer (scaffold + `repo add` + `repo tag` + sparse-profile generator) to emit `amaru_version: "2"` and the flat layout. After this unit, `amaru repo init` produces a v2 registry from scratch.

**Requirements:** R1

**Dependencies:** Unit 1.

**Files:**
- Modify: `internal/scaffold/scaffold.go` (drop the `.amaru_registry/` prefix in directory list, sparse-profile body, and root AGENTS.md template)
- Modify: `internal/scaffold/scaffold_test.go`
- Modify: `cmd/repo_add.go` (compute item dir via `layout.ItemDir`)
- Modify: `cmd/repo_tag.go` (same)
- Modify: `cmd/repo.go` (update the "Created: ..." status messages)
- Modify: `cmd/repo_test.go` (fixtures now create v2 layouts)
- Test: `internal/scaffold/scaffold_test.go`, `cmd/repo_test.go`

**Approach:**
- `ScaffoldRepo` creates `skills/`, `commands/`, `agents/`, `context/`, `.sparse-profiles/` at the root (gitkeeps where appropriate).
- `amaru_registry.json` is written with `amaru_version: "2"`.
- `SparseProfile()` returns `context/<project>/**` (no leading dot-dir). `RootAgentsMD()` updates the rendered tree-shape paragraph.
- `repo_add` and `repo_tag` call into `layout.ItemDir(idx, dir, itemType, name)` instead of building the path inline.

**Patterns to follow:**
- The current `ScaffoldRepo` shape — same control flow, just fewer prefixes.

**Test scenarios:**
- Happy path: `ScaffoldRepo({Dir, Project: "myapp"})` creates `skills/`, `commands/`, `agents/`, `context/myapp/{brainstorms,plans,solutions}`, `.sparse-profiles/myapp`, no `.amaru_registry/` directory.
- Happy path: `repo add foo` writes `skills/foo/manifest.json` and `skills/foo/skill.md`.
- Happy path: `repo tag foo 1.0.0` updates `skills/foo/manifest.json` and the index, and the file paths it stages with `git add` are flat-layout paths.
- Edge case: scaffolding into a directory that already contains an `amaru_registry.json` is rejected (or — pick one — overwrites only when `--force`; default behavior matches today).
- Integration: `repo init && repo add bar && repo validate` round-trips with no `.amaru_registry/` references on disk.

**Verification:** Fresh `repo init` produces a tree that exactly matches the README's "Registry Structure" after the docs-sync unit lands. No `.amaru_registry/` paths appear in any test fixture under `cmd/` or `internal/scaffold/`.

- [ ] **Unit 3: `amaru repo migrate` (in-place, local)**

**Goal:** Add the local migration command that converts a v1 layout to v2. Idempotent, with a `--dry-run` flag.

**Requirements:** R3

**Dependencies:** Unit 1.

**Files:**
- Create: `cmd/repo_migrate.go`
- Create: `cmd/repo_migrate_test.go`
- Create: `internal/scaffold/migrate.go` (pure function: takes a directory, runs the move, returns a summary)
- Create: `internal/scaffold/migrate_test.go`

**Approach:**
- Cobra command `repo migrate [<url>] [--dry-run] [--allow-dirty] [--skip-sparse-rewrite]` with no positional arg → in-place mode (this unit). The `<url>` form lands in Unit 4.
- `scaffold.MigrateInPlace(dir, opts)` validates a `.amaru_registry/` exists and is in a recoverable state (see invariant table below), then performs the migration in this strict order: (1) write the `.amaru_registry/.migrating` journal file with a JSON record of intended moves and a timestamp; (2) for each top-level child of `.amaru_registry/` (`skills`, `commands`, `agents`, `context`, `.sparse-profiles`, plus any non-named children — see conflict detection), `os.Rename` to the registry root; (3) rewrite each `.sparse-profiles/<project>` file using the anchored-prefix algorithm (see Open Questions for the exact rule), writing `<profile>.bak` before the first edit; (4) atomically rewrite `amaru_registry.json` with `amaru_version: "2"` using the existing `internal/scaffold/index.go` temp-file-and-rename pattern; (5) remove the now-empty `.amaru_registry/` directory; (6) remove the `.migrating` journal. The journal lives at the registry root *outside* `.amaru_registry/` after step 2 so it survives the dir removal.
- **Pre-flight conflict detection** rejects all of: (a) `.amaru_registry/<dir>/` AND root `<dir>/` both present for any `<dir>`; (b) any symlink at `.amaru_registry/<dir>` or root `<dir>` (resolved via `os.Lstat`, not `os.Stat`); (c) on case-insensitive filesystems, a root `Skills/` that would collide with `skills/` (detected by `os.Lstat` succeeding on the cased and uncased forms); (d) non-named children of `.amaru_registry/` outside the canonical list (e.g. `.DS_Store`, `.bak`, editor swap files) — these are reported but moved verbatim unless `--strict-children` is passed; (e) for in-place mode without `--allow-dirty`, a non-clean working tree (`git status --porcelain` non-empty) — refuse so the user doesn't fold migration noise into an unrelated commit.
- **Crash recovery.** A re-run inspects the journal first. The decision matrix:

  | `.migrating` present | `.amaru_registry/` present | root `skills/` present | Action |
  |---|---|---|---|
  | no | yes | no | normal pre-flight + migrate |
  | no | no | yes | "Already migrated" no-op, exit 0 |
  | no | yes | yes | error: pre-existing collision, ask user to resolve |
  | yes | yes | yes | error: previous run crashed mid-move; print the journal, list which moves remain, hint `git status` |
  | yes | yes | no | error: previous run crashed before any move; remove journal manually and re-run |
  | yes | no | yes | error: journal stale; remove `.migrating` and re-run |
  | no | no | no | error: no registry layout detected |

  The error text for the crashed-mid-move case is exactly: `"migration journal at <path> indicates a previous run did not complete. Inspect the journal, finish or revert the partial moves, remove the journal file, and re-run."` This text is asserted in tests so it stays stable.
- **Sparse-profile rewrite** uses the anchored line-prefix algorithm specified in Open Questions: line begins (after optional `!` and whitespace) with `.amaru_registry/context/<project>/` → replace prefix; otherwise leave alone. Each modified profile gets a one-shot `<profile>.bak` written on first edit. `--skip-sparse-rewrite` bypasses the rewrite.
- **Atomicity.** The `amaru_registry.json` rewrite uses the temp-file-and-rename pattern from `internal/scaffold/index.go`. If the rewrite fails after moves have completed, the journal remains, recovery is "delete `.migrating`, manually set `amaru_version: "2"`, re-run validate."
- **Dry-run** runs the entire pre-flight (so users see conflicts), prints the planned move list, and exits without writing the journal or touching the filesystem.

**Execution note:** Implement test-first — start with table-driven cases for `MigrateInPlace` covering every row of the crash-recovery matrix above, plus the conflict-detection branches. The destructive nature warrants characterization-style coverage before any rename calls go in.

**Patterns to follow:**
- `internal/scaffold/index.go` atomic-write pattern for the index update.
- The repo-subcommand cobra layout in `cmd/repo_*.go`.

**Test scenarios:**
- Happy path: tempdir with `.amaru_registry/skills/foo/` migrates; afterwards `skills/foo/` exists, `.amaru_registry/` is gone, index has `amaru_version: "2"`, sparse profile body uses flat paths, `.migrating` journal is gone.
- Edge case: registry already at v2 → no-op, `MigrateInPlace` returns "already migrated", filesystem unchanged.
- Edge case: registry has no `.amaru_registry/` and no v2 markers → error "no registry layout detected".
- Edge case (conflict — pre-existing collision): both `.amaru_registry/skills/` and root `skills/` exist → error, filesystem unchanged.
- Edge case (conflict — symlink): `.amaru_registry/skills` is a symlink → error, filesystem unchanged.
- Edge case (conflict — case-insensitive FS): root `Skills/` exists, target `skills/` would collide → error.
- Edge case (conflict — extra children): `.amaru_registry/.DS_Store` and `.amaru_registry/notes.bak` exist alongside the named subdirs → reported, moved verbatim by default; with `--strict-children` → error.
- Edge case (working tree dirty): in-place mode with uncommitted changes and no `--allow-dirty` → error before any move; with `--allow-dirty` → proceeds.
- Edge case (sparse profile): `.amaru_registry/.sparse-profiles/myapp` contains a negation line `!.amaru_registry/context/myapp/secrets/**`, a comment, and a non-context line. After migration: only the include and the negation are rewritten (both anchored at line start, optionally after `!`), comment and non-context line untouched, `.bak` exists with the original content.
- Edge case (sparse profile bypass): with `--skip-sparse-rewrite`, profiles are untouched even when they contain rewriteable lines.
- Edge case (dry-run): `--dry-run` prints planned moves, no filesystem changes, no journal written; running again without the flag completes successfully.
- Crash-recovery (matrix row: journal + legacy + flat both present): pre-existing journal with both layouts present → exact error text matches the asserted string; `git status` hint is in stderr.
- Crash-recovery (matrix row: journal + only legacy): pre-existing journal with no flat dirs yet → error "remove journal manually and re-run".
- Crash-recovery (matrix row: stale journal after success): journal present but only flat layout exists → error "journal stale; remove `.migrating` and re-run".
- Error path (atomicity): simulated write failure on `amaru_registry.json` rewrite (after moves completed) → journal remains, helpful error message references the recovery procedure.
- Error path (read-only filesystem during move): returns wrapping error with the path and operation that failed.
- Integration: after migration, `repo validate` succeeds without errors.
- Integration: after migration, the `amaru_version: "2"` value matches the on-disk layout (assert no marker/layout mismatch).
- Integration (CLI): `amaru repo migrate --dry-run` prints planned moves and exits 0; running again without `--dry-run` actually moves them.

**Verification:** Round-trip — scaffold a v1 registry by hand (or via Unit 1 era `ScaffoldRepo`), migrate it, run `repo validate`, run `install` against it from a consumer project, all succeed.

- [ ] **Unit 4: `amaru repo migrate <url>` — clone, migrate, push**

**Goal:** Extend `repo migrate` with the remote variant: clone a registry repo, migrate locally, commit, push back.

**Requirements:** R4

**Dependencies:** Unit 3.

**Files:**
- Modify: `cmd/repo_migrate.go`
- Create: `internal/scaffold/migrate_remote.go`
- Create: `internal/scaffold/migrate_remote_test.go`

**Approach:**
- The cobra command accepts an optional positional `<url>` plus `--push-to-default` and `--branch <name>` flags. With a URL, switch to remote mode.
- Steps: (1) normalize URL via existing `registry.NormalizeURL`; (2) translate canonical `github:org/repo` to a clone URL using whichever protocol the user prefers (default: try SSH, fall back to HTTPS with gh-CLI token if SSH fails); (3) `git clone` into `os.MkdirTemp`, recording the cloned HEAD SHA; (4) `git checkout -b migration/flat-layout` (configurable via `--branch`); (5) call `MigrateInPlace`; (6) `git add -A`; (7) `git commit -m "chore: migrate registry to flat layout (amaru repo migrate)"`; (8) `git fetch origin <default-branch>` and confirm the cloned HEAD SHA is still an ancestor of the fetched ref — refuse with the recovery hint if the remote moved during the migration; (9) `git push -u origin migration/flat-layout`; (10) print a `gh pr create --base <default-branch> --title "Migrate registry to flat layout"` next-step.
- `--push-to-default` opts into pushing the migration commit directly onto the default branch with the same fetch-and-ancestor verification — for solo maintainers who don't want a PR. Off by default.
- Refuse to push when `MigrateInPlace` returned "already migrated" (no commit was created, nothing to push). Exit 0 with an info message. Also refuse if the post-migration `git status` shows nothing changed.
- `--dry-run` clones, migrates, prints the would-be commit and push command, never invokes `git push`.
- **Signal handling.** Install a SIGINT/SIGTERM handler at startup that removes the tempdir before exiting (the temp clone contains credentials in `.git/config` if HTTPS is used — leaking it on Ctrl-C is unacceptable). The handler runs even when `--dry-run` is set.
- **Per-invocation timeout.** Every `gitRunner.Run` call carries a 60-second context timeout to prevent credential-helper prompts from hanging the process indefinitely. Timeouts surface a specific error mentioning the prompt-on-stdin gotcha.
- The user must already have git credentials set up; we don't manage auth. Surface git's stderr verbatim when push fails.

**Execution note:** Mock the `git` invocations behind a small interface (`gitRunner.Run(ctx, args ...string) (stdout, stderr []byte, err error)`) so the migrate logic is testable without a real repo. The interface takes a context so the timeout is enforceable in tests.

**Patterns to follow:**
- `cmd/repo_tag.go:127-152` for shelling out to `git` and surfacing combined output on failure.

**Test scenarios:**
- Happy path: `migrate-remote` against a fake gitRunner clones, checks out `migration/flat-layout`, calls migrate, commits with the expected message, fetches the remote default branch, verifies ancestry, pushes the side branch, prints the `gh pr create` hint. Full git invocation sequence is asserted in order.
- Happy path (`--push-to-default`): same flow but skips the branch creation and pushes onto the default branch.
- Edge case: clone fails → error wraps git's stderr, no further git calls happen, no tempdir leaks (cleanup verified, including SIGINT handler path).
- Edge case: registry is already v2 → no commit, no push, exit 0 with informational output. Tempdir cleaned.
- Edge case: push fails (e.g. permission denied) → error includes git's stderr; tempdir is still cleaned up; no partial state on remote (the side branch wasn't pushed).
- Edge case (remote moved): the fetch step shows the cloned HEAD is no longer an ancestor of the remote's default branch tip → refuse, do not push, print recovery hint pointing at re-cloning.
- Edge case: invalid URL (non-GitHub, non-canonical) → rejected by the existing URL parser before any git work.
- Edge case (clone target is not a registry): cloned repo has no `amaru_registry.json` at root → `MigrateInPlace` returns "no registry layout detected", remote command propagates the error and never attempts a push.
- Edge case (registry nested under a subdirectory): cloned repo has `amaru_registry.json` at `subdir/`, not root → error pointing to the subdir; out of scope to auto-discover.
- Edge case (credential-helper hang): a `git push` invocation that would normally hang on stdin is cancelled at the 60s context timeout and returns a specific error mentioning the prompt gotcha; tempdir cleaned.
- Edge case (SIGINT mid-clone): test injects a SIGINT into the gitRunner's clone step; tempdir is removed by the handler before process exit.
- Edge case (`--dry-run`): no `git push` is ever called; the planned commit message and push target are printed.
- Integration (manual smoke test, not in CI): migrate this repo's own past history on a throwaway branch.

**Verification:** Unit tests with a fake gitRunner cover the four sequencing/error branches. `--dry-run` against any URL never invokes `git push`.

- [ ] **Unit 5: `repo tag --cascade` with skillset version cascade**

**Goal:** Extend `repo tag` with a `--cascade` flag that, in addition to its existing per-item tag/commit work, patch-bumps every skillset entry whose `Items` contain the tagged item.

**Requirements:** R7

**Dependencies:** Units 1 and 2 (so writes target the flat layout).

**Files:**
- Modify: `cmd/repo_tag.go` (add `--cascade`, extract the manifest-and-index-update body into a reusable helper)
- Modify: `cmd/repo_test.go` (or add `cmd/repo_tag_test.go`)
- Modify: `internal/registry/registry.go` (small helper: `SkillsetsContaining(idx, itemType, name) []string`)

**Approach:**
- New flag `--cascade` (default false) on `repo tag`. When set: after the existing manifest + index `Latest` write for the tagged item, walk `idx.Skillsets`; for each entry whose `Items` contain a `{Type, Name}` matching the tagged item, increment that skillset's `Latest` by patch (`X.Y.Z → X.Y.Z+1`; if the skillset has no `Latest`, set it to `0.1.0`). The cascade write happens before the `git add` step so all changes land in the same commit.
- The cascade only touches the in-index `Latest` for skillset entries. It does not create individual git tags for each skillset (`amaru` does not support skillset version git tags today; adding them is out of scope for this plan).
- Without `--cascade`, behavior is identical to today.
- The existing `--push` flag still controls whether the per-item tag is pushed; cascade has no separate push semantics (it rides along on the same commit).

**Patterns to follow:**
- `cmd/repo_tag.go:44-158` end-to-end.
- `internal/registry/registry.go:46-58` for type-keyed entry access.

**Test scenarios:**
- Happy path (cascade off): identical to today's behavior — only the tagged item's `Latest` and `manifest.json` change.
- Happy path (cascade on): tag `skill/foo` from `1.0.0` to `1.1.0` with `--cascade`; manifest version updates, `idx.Skills["foo"].Latest` updates, every skillset whose `Items` contain `{Type:"skill", Name:"foo"}` has its `Latest` patch-bumped, all changes are in the single commit.
- Edge case: skillset with no `Latest` defined → set to `0.1.0`.
- Edge case: skillset with non-semver `Latest` → error with a clear repair hint, no partial writes (the in-memory cascade fails before any disk write).
- Edge case: item doesn't exist in the index → existing error path, unchanged by `--cascade`.
- Edge case: version equals current version → existing error path, unchanged by `--cascade`.
- Edge case: tagged item is referenced by zero skillsets → `--cascade` is a no-op for skillsets, the tag still happens.
- Integration: after `repo tag foo 1.1.0 --cascade`, `repo validate` succeeds, `git tag -l` lists `skill/foo/1.1.0`, and the index diff shows skillset `Latest` bumps.

**Verification:** A round-trip in tests: tag twice with `--cascade` (`1.0.0 → 1.1.0 → 1.2.0`), assert that a skillset starting at `0.1.0` ends at `0.1.2`. `repo tag` without `--cascade` behaves identically to today.

- [ ] **Unit 6: Foreign-registry breadth — commands, agents, lowercase content files**

**Goal:** When `amaru_registry.json` is missing (the `synthesized` source mode introduced in Unit 1), scan `skills/`, `commands/`, and `agents/` (not just `skills/`), and accept either `SKILL.md`/`COMMAND.md`/`AGENT.md` or lowercase variants for description extraction.

**Requirements:** R5

**Dependencies:** Unit 1 (introduces the `(flat, synthesized)` source mode).

**Files:**
- Modify: `internal/registry/github.go` (`fetchVercelIndex` → `fetchSynthesizedIndex`, broaden to all three item types; broaden filename detection)
- Modify: `internal/registry/github_test.go` (mock GitHub responses for a foreign registry with skills + commands + agents)

**Approach:**
- Replace the single `skills/` scan with a loop over `types.AllInstallableTypes()`. A directory missing → contribute nothing, no error. Per-item-type, list the directory, then for each child dir attempt to fetch a description from `<TYPE>.md`, then `<type>.md`, in that order. First hit wins; missing → empty description.
- `DownloadFiles` for the `(flat, synthesized)` mode uses the same `<type>s/<name>/` path math as `(flat, indexed)`; the difference is only that there's no manifest.json or skillset support.
- `ListVersions` for synthesized-source registries returns nil (no per-item tags expected); already handled.
- Skillsets and `FetchSkillsetManifest` are not supported in synthesized mode — return a clear "this registry does not support skillsets" error if a consumer tries.

**Patterns to follow:**
- `fetchVercelIndex` in `internal/registry/github.go:322-398` — preserve the parallel-fetch + semaphore shape.

**Test scenarios:**
- Happy path: foreign registry with `skills/foo/SKILL.md` and `commands/bar/COMMAND.md` and `agents/baz/agent.md` produces an index with all three entries; descriptions come from frontmatter.
- Edge case: a registry with only `skills/` still works and produces an index with empty `Commands` / `Agents` maps.
- Edge case: a child directory has neither `<TYPE>.md` nor `<type>.md` → entry exists with empty description (no error).
- Edge case: a directory contains a non-dir file (e.g. README.md at `skills/`) → ignored.
- Edge case: caller invokes `FetchSkillsetManifest` on a synthesized-source client → returns the "skillsets not supported" error rather than a 404.
- Integration: `amaru add foo --registry foreign` against a Vercel-style mocked registry succeeds for skill, command, and agent.

**Verification:** Existing `formatVercel` tests pass post-rename. A new test fixture mirrors a `claude-code-plugins-plus-skills`-shaped repo (skills + commands) and produces a complete index.

- [ ] **Unit 7: Mirror provenance in `browse` and `list`**

**Goal:** Show users which entries came from which mirror. Verify mirrors work end-to-end against both amaru-native and foreign registries.

**Requirements:** R6

**Dependencies:** Unit 1 (no hard dependency, but the mirror tests benefit from the v2 layout being readable).

**Files:**
- Modify: `internal/registry/registry.go` (track `Source` on entries in the merged index — a small `string` field on `RegistryEntry` and `SkillsetEntry`, populated by `MergeFrom`)
- Modify: `internal/registry/github.go` (set `Source` to the mirror's normalized URL when merging)
- Modify: `cmd/browse.go` and `cmd/list.go` (render the source annotation when non-empty)
- Modify: `internal/registry/github_test.go` (end-to-end mirror test using two mock clients)

**Approach:**
- `RegistryEntry` and `SkillsetEntry` gain a non-serialized `Source string` field (omit from JSON via tag `json:"-"`). Default empty = primary registry.
- `MergeFrom(other, otherURL)` semantics, documented and tested:
  - When `other` has an entry that the receiver does not: copy the entry, set `Source = otherURL`.
  - When both have an entry of the same name: receiver wins (existing behavior); the receiver's `Source` is *not* modified — a primary entry stays primary even if a mirror happens to have the same name.
  - When the receiver has an entry that `other` doesn't: untouched, `Source` remains whatever it was before this merge call (which is `""` for primary, or the URL of an earlier mirror in chained-mirror scenarios).
- This means: after merging through any number of mirrors, `Source = ""` is exactly the set of entries the primary registry shipped, and a non-empty `Source` is the URL of the *first* mirror in iteration order that contributed the entry. Chained mirrors are not deduplicated by URL — if the same mirror is reachable through two paths, the first path wins.
- The signature change from `MergeFrom(other)` to `MergeFrom(other, otherURL)` is internal-only (the call site is `github.go:288-300`); update that one call site to pass the mirror's URL.
- `browse` output adds a trailing tag for non-empty sources: `[main ← github:vercel-labs/agent-skills]`.
- `list` output adds the same annotation in the existing `[main]` column.
- Add an explicit integration test using a fake `mirrorFactory` that returns a second mocked client, asserting the merged index has expected `Source` values for unique-to-mirror, primary-wins-collision, and primary-only entries.

**Patterns to follow:**
- `cmd/browse.go` and `cmd/list.go` for column/output formatting.
- `internal/registry/github_test.go` mock-client patterns.

**Test scenarios:**
- Happy path: primary index merges with mirror; entries unique to the mirror have `Source = mirrorURL`, primary entries have `Source = ""`.
- Edge case: name collision (primary and mirror both have `skill/foo`) — primary wins, primary's `Source` remains `""` (asserted explicitly).
- Edge case: chained mirrors (primary lists mirror A, A lists mirror B). After both merges, an entry only present in B has `Source = <A's URL>` — the *first* mirror that contributed the entry, not the original origin.
- Edge case: mirror unreachable — primary still resolves, no error surfaces to the user (matches today's silent-skip behavior in `github.go:288-300`), but a debug log records the failure.
- Output: `browse` and `list` show the annotation when present, omit it when absent.
- Integration: `amaru install` of a mirror-only entry actually downloads files from the mirror's URL.

**Verification:** New end-to-end mirror test passes. Manual smoke test against a tiny throwaway primary that mirrors `vercel-labs/agent-skills` shows correct provenance.

- [ ] **Unit 8: Documentation sync and self-hosted skill migration**

**Goal:** Update README, CLAUDE.md, AGENTS.md, the `amaru-usage` skill, and migrate this repo's own self-hosted skill to v2 — completing the layout cutover for amaru itself.

**Requirements:** R1, R3, R4, R5, R6, R7

**Dependencies:** Units 1–7.

**Files:**
- Modify: `README.md` (Registry Structure section, Quick Start, Commands list to include `migrate` and `tag --cascade`)
- Modify: `CLAUDE.md` (Key Conventions: registry layout description, `repo` subcommand list)
- Modify: `AGENTS.md` (architecture diagram, Key Data Flow entries for migrate and the cascade flag)
- Move: `.amaru_registry/skills/amaru-usage/` → `skills/amaru-usage/` via the new `repo migrate` (run in-place inside this repo)
- Modify: `skills/amaru-usage/skill.md` (post-migration) — add `migrate`, `bump`, and the foreign-registry note; reflect the flat layout
- Modify: `amaru_registry.json` — `amaru_version: "2"` (touched by the migrate run)
- Test: none — this is a docs/data unit.

**Approach:**
- After Units 1–5 are merged, run `amaru repo migrate` in this repo, commit the file moves, then update the docs to describe the new state.
- The migration commit must be reviewable as a plain file-move diff. If `repo migrate` misbehaves at self-host time, an equivalent `git mv` invocation is acceptable — the goal is the layout cutover for self-hosting, not exercising the new command on the self-host.
- Update Quick Start in README.md to mention `amaru repo migrate` for upgraders and `amaru repo tag --cascade` for maintainers.
- Update the Registry Structure tree in README.md to show the flat layout.
- Update CLAUDE.md "Registry layout" bullet, the `repo` command list, the v1 deprecation timeline, and both rollback runbooks.
- Update `amaru-usage/skill.md` to teach Claude Code about the new commands, the dual-layout read story, and that running an older amaru against a v2 registry will fail with an explicit "unknown amaru_version" error.

**Test expectation:** none — this unit is documentation and a data move. Verification is by manual review and `repo validate` running clean against the migrated self-hosted skill.

**Verification:** `repo validate` clean; `grep -r .amaru_registry .` returns only doc references that explicitly call out the legacy layout for backward-compat context; the README tree matches reality.

## System-Wide Impact

- **Interaction graph:** `repo` subcommands all share the new `layout.go` helper; `install`, `update`, and `check` go through `registry.Client.DownloadFiles`, which now consults the layout helper. Context sync (`internal/ctxdocs/ctxdocs.go`) also threads through the layout helper for the `context/<project>` path.
- **Error propagation:** `MigrateInPlace` errors surface verbatim from `cmd/repo_migrate.go`. Remote-migrate errors wrap git's stderr at the point of failure. Both modes guarantee no partial state when they error before the rename step; after a partial rename failure, the error message names the affected paths.
- **State lifecycle risks:** Migrations are filesystem-level renames. A crashed migration can leave a repo in a hybrid state (some dirs moved, some not). The plan addresses this by ordering moves deterministically and by adding the "both layouts present" check that aborts before the first rename. Re-running the command on a half-migrated tree refuses to act and tells the user which paths to resolve.
- **API surface parity:** `cmd/repo_test.go` fixtures change shape (v1 → v2). External consumers running an old amaru against a v2 registry will fail format detection — that's by design and called out in the migration release notes.
- **Integration coverage:** The mirror test in Unit 7 is the first end-to-end coverage of the merge path, which has lived in code without a test since the field was introduced. `repo migrate` round-trips (scaffold → migrate → validate → install) become the integration coverage for the layout cutover.
- **Unchanged invariants:** `amaru.json` and `amaru.lock` shapes are unchanged. The `.claude/{skills,commands,agents}/` install layout in consumer projects is unchanged. Per-item version git tags (`skill/foo/1.0.0`) are unchanged in name and meaning. `Authenticator` interface and auth methods are unchanged.

## Risks & Dependencies

| Risk | Mitigation |
|------|------------|
| Half-migrated registry on a crashed `repo migrate` | (1) Pre-flight collision check rejects symlinks, case-insensitive collisions, both-layouts-present, and (in-place) dirty working tree; (2) `.amaru_registry/.migrating` journal records intended moves before the first rename and is removed on success; (3) re-runs consult the journal and emit one of the explicit recovery messages in Unit 3's matrix; (4) recovery hint text is asserted in tests so it stays stable across releases. |
| `amaru_registry.json` rewrite fails after moves complete (marker-vs-layout mismatch) | Atomic temp-file-and-rename using the existing `internal/scaffold/index.go` pattern. Journal remains on failure so the user knows recovery is "delete `.migrating`, set `amaru_version: "2"`, run validate." Test scenario simulates the failure. |
| `repo migrate <url>` pushes a bad commit to `main` | Default pushes to `migration/flat-layout` side branch, prints `gh pr create` next-step. `--push-to-default` is opt-in. Pre-push `git fetch` + ancestry check refuses if the remote moved during migration. Fixed commit message stays revert-friendly. |
| `repo migrate <url>` SIGINT leaks a tempdir with credentials | SIGINT/SIGTERM handler removes the tempdir before exit, runs even in `--dry-run`. Test case asserts cleanup on signal. |
| Credential-helper hangs `git push` waiting on stdin | 60s per-invocation context timeout via the `gitRunner` interface; timeout error mentions the prompt-on-stdin gotcha. |
| Sparse-profile rewrite corrupts user-customized profiles | Anchored line-prefix algorithm (matches only at line start, after optional `!`); `.bak` written on first edit; `--skip-sparse-rewrite` escape hatch. Test scenarios cover negation lines, comments, and embedded-but-not-prefix matches. |
| Foreign-registry detection picks up unrelated `skills/` directories in arbitrary repos | Only triggers when `amaru_registry.json` is absent. Foreign registries with no skill content surface as empty indexes — same behavior as today. |
| `repo tag --cascade` silently bumps skillsets the maintainer didn't intend to | Cascade is opt-in (default off), logged per skillset, and rolls back on any per-skillset error before any disk write. Users who don't want cascading don't pay for it. |
| Older amarus break against v2 registries | Release notes call this out; legacy `amaru` versions get a clean "unknown amaru_version" error from Unit 1's strict-unknown handling rather than a confusing 404. |
| v1 read path becomes permanent dead code | Stderr deprecation warning emitted on every v1 load (suppressible via `--quiet`). Removal milestone declared in Documentation/Operational Notes: v1 reads are removed in the second release after `repo migrate <url>` ships in stable. |
| Mirror auth assumption (none) bites someone | Documented in CLAUDE.md and surfaced in `--verbose` mirror failures; auth-aware mirrors are a deliberately deferred follow-up. |

## Documentation / Operational Notes

- README.md, CLAUDE.md, AGENTS.md, and the `amaru-usage` skill all update in Unit 8.
- A short migration note belongs in the GitHub release: "v2 registry layout. Run `amaru repo migrate` to upgrade existing registries. Older amarus will refuse to install from v2 registries."
- This plan ships behind a normal version bump; no feature flag needed because the layout marker (`amaru_version`) is the gate.
- **v1 read-path removal milestone.** Concrete commitment: v1 reads are removed in the second release after `repo migrate <url>` lands in stable. The release that ships this plan logs a stderr deprecation warning on every v1 load (suppressed by `--quiet`); the next release continues to read v1; the release after removes the v1 path entirely. Document this timeline in CLAUDE.md so it survives a maintainer handoff.
- **Rollback runbook for `repo migrate <url>`.** Because Unit 4 pushes a side branch by default, rollback is "don't merge the PR." For maintainers who used `--push-to-default`: (1) `git revert <migration-sha>` on the registry repo to restore the legacy layout in a new commit; (2) `git push` the revert; (3) consumers running an older amaru pick up the reverted state on their next `amaru install` or `amaru check`; (4) consumers who already migrated their `amaru.json` to depend on a v2-only amaru release will need to either pin to a pre-revert SHA or roll forward by re-running `repo migrate` after fixing whatever forced the rollback. The runbook should appear in CLAUDE.md alongside the migration command docs.
- **Rollback runbook for `repo migrate` (in-place).** `git revert` of the migration commit restores the legacy layout. The journal file (`.amaru_registry/.migrating`) is not committed (it lives outside `.amaru_registry/` after the first rename and gets `.gitignore`d via the migration commit). If a half-migrated state was committed by accident, `git reset --hard <pre-migration-sha>` is the recovery; the plan does not provide a forward-only un-migrate command.

## Sources & References

- Existing implementations referenced: `internal/registry/github.go`, `internal/scaffold/scaffold.go`, `cmd/repo_*.go`, `internal/ctxdocs/ctxdocs.go`, `internal/registry/registry.go`.
- Related prior plans in this repo: `docs/plans/2026-03-05-feat-repo-management-commands-plan.md` (introduced `repo` subcommands and the `.amaru_registry/` convention this plan revises).
- External convention examples: `vercel-labs/agent-skills`, `jeremylongshore/claude-code-plugins-plus-skills` (both use `skills/<name>/SKILL.md` at the top level).
