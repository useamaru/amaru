# Amaru — Development Guide

## Overview

Amaru is a Go CLI tool that manages skills, commands, and agents for Claude Code. It connects projects to centralized GitHub-hosted registries via a manifest file (`amaru.json`) and lock file (`amaru.lock`).

## Build & Test

```bash
go build ./...          # Build all packages
go test ./...           # Run all tests
go vet ./...            # Static analysis
```

No special setup needed — standard Go toolchain.

## Project Structure

```
cmd/              # Cobra CLI commands (one file per command)
internal/
  checker/        # Compares lock vs registry (update detection, drift detection)
  ctxdocs/        # Context documentation sync (sparse checkout)
  hooks/          # Git hook management (post-checkout, post-commit)
  installer/      # File installation + hash computation
  manifest/       # amaru.json and amaru.lock parsing/saving
  registry/       # GitHub API client, registry types, authentication
  resolver/       # Semver constraint resolution
  scaffold/       # Registry repo scaffolding, templates, local index I/O, name validation
  types/          # Shared type definitions (ItemType)
  ui/             # Terminal output formatting (colors, tables)
  vcs/            # VCS backend detection (Git vs Sapling)
```

## Key Conventions

- **Item types**: `skill`, `command`, `agent` — defined in `internal/types/types.go`
- **Skillsets**: Registry-defined groups that expand to individual items on install. Not an item type — they live in the registry index and lock file only.
- **Version constraints**: Follow npm conventions (`^`, `~`, exact). `"latest"` is a special non-semver constraint for unversioned items.
- **Registry URLs**: Canonical form is `github:org/repo`. Multiple formats accepted (SSH, HTTPS, bare domain) and normalized on init.
- **Registry layout**: `amaru_registry.json` at repo root with `amaru_version: "2"` (flat) — content lives at the root in `skills/`, `commands/`, `agents/`, `context/`, `.sparse-profiles/`. amaru still reads the legacy v1 layout (`.amaru_registry/<dir>/...`) for backward compatibility — `amaru repo migrate` converts v1 registries to v2 in place. The `amaru_version` field is the layout marker only; future data-shape evolution will use a separate `schema_version` field. Path resolution lives in `internal/registry/layout.go` (the `Layout` type) — never hardcode `.amaru_registry/` in new code.
- **Self-hosted registries**: Any repo can be its own registry — this repo ships an `amaru-usage` skill via `amaru_registry.json` + `skills/amaru-usage/` at the root.
- **Item folders (cosmetic)**: A `RegistryEntry` may declare `Folder` (e.g. `"dev"`), in which case the source lives at `<typedir>/<folder>/<name>/` instead of `<typedir>/<name>/`. Folder is purely organizational — it does **not** become part of the item name, git tag (`skill/<name>/<version>`), or install path (`.claude/skills/<name>/`). Path resolution uses `registry.ItemSubPath(entry.Folder, name)` as the subpath argument to `Layout.ItemDir` / `Layout.RelativeItemPath`. Authoring: `amaru repo add <name> --folder <folder>`. `GitHubClient.DownloadFiles` reads the folder from a cached `FetchIndex` — keep `FetchIndex` idempotent within a client lifetime so the implicit lookup is free.
- **v1 deprecation timeline**: When loading a v1 registry over the network, `GitHubClient.FetchIndex` emits a one-line stderr deprecation warning (suppressed by `--quiet`). The v1 read path is committed to be removed in the second release after `repo migrate <url>` ships in stable; everything new should be v2 only.
- **DependencySpec**: Marshals as shorthand string when only version is set, full object when registry or group is present.
- **Lock entries**: Store resolved version, registry alias, content hash, and timestamp.
- **Registry management**: `amaru repo` subcommands (`add`, `remove`, `tag`, `info`, `validate`, `migrate`) modify `amaru_registry.json` and the registry's content directories. The index file is the source of truth for what's published; git tags are the source of truth for item versions. All `repo` commands require CWD to contain `amaru_registry.json`. `repo tag --cascade` patch-bumps every skillset whose Items contain the tagged item (in-index `Latest` only — no per-skillset git tag).
- **Cross-registry skillsets**: a `SkillsetItem` may set an optional `registry` field to source the member from a different registry alias. `cmd/install.go:installSkillset` resolves per-item alias (falls back to the skillset's home alias when empty), validates that the consumer has the alias configured in `amaru.json`, and caches per-registry indexes so each alias is fetched at most once per skillset install. The lock file's skillset digest now encodes the source alias (`type/name/version@alias`) so a member moving registries invalidates the digest. Authoring syntax: `repo add ... --type skillset --items "skill/foo,command/bar@platform"`.
- **Foreign-registry shapes**: synthesized-source registries (no `amaru_registry.json`) support two layouts: flat (`skills/<name>/SKILL.md`) and one-level-nested (`skills/<category>/<name>/SKILL.md` — Google-style). Detection is in `internal/registry/github.go:scanSynthesizedType`. Nested item names contain a single `/`; the install path treats them as deeper directory targets. Skillsets and per-item version tags are not supported on synthesized sources.
- **Migration runbook**:
  - In-place: `amaru repo migrate` (idempotent, with `--dry-run`, `--allow-dirty`, `--skip-sparse-rewrite`, `--strict-children`). A `.migrating` journal at the registry root records intent before any rename and protects against half-migrated state. Sparse profiles are rewritten via an anchored line-prefix algorithm; originals are preserved as `.bak` files.
  - Remote: `amaru repo migrate <url>` clones to a tempdir, runs the in-place migration, and pushes a `migration/flat-layout` side branch by default. Pre-push the command verifies the cloned HEAD still matches the remote tip. `--push-to-default` opts into pushing the migration commit directly onto the default branch (skips the PR step); use only for solo-maintained registries.
  - Rollback: don't merge the PR (or `git revert <migration-sha>` if `--push-to-default` was used). The migration commit is a plain file-move diff so revert is one command.
- **Documentation sync**: When adding or changing CLI commands, always update README.md, AGENTS.md, CLAUDE.md, and the `amaru-usage` skill to reflect new capabilities.

## Testing Patterns

- Tests use `t.TempDir()` for filesystem isolation
- Registry client is mocked via `mockRegistryClient` implementing `registry.Client`
- Table-driven tests preferred (see `github_test.go`, `manifest_test.go`)
- No external service calls in tests — everything is mocked or uses local fixtures

## Code Style

- Standard Go formatting (`gofmt`)
- All user-facing messages, code, comments, and docs in English
- Error wrapping with `fmt.Errorf("context: %w", err)` pattern
- Cobra commands: one file per command in `cmd/`, registered via `init()`
