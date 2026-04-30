---
description: This skill should be used when managing Claude Code skills, commands, or agents with amaru. It applies when the user asks to "add a skill", "install skills", "check for updates", "browse available skills", "update skills", or any amaru CLI workflow.
allowed-tools: Bash(amaru:*)
---

# Amaru — Skills, Commands & Agents Manager

Use the `amaru` CLI to manage Claude Code skills, commands, and agents from centralized GitHub registries.

## Core Workflow

```bash
amaru init                              # Set up amaru.json with registry URLs
amaru browse                            # Discover available items
amaru add <name>                        # Add a skill (default type)
amaru add <name> --type command         # Add a command
amaru add <name> --type agent           # Add an agent
amaru add <name> --type skillset        # Add a skillset (expands to individual items)
amaru install                           # Install/sync everything from manifest
amaru check                             # Check for updates and local drift
amaru update [name]                     # Update to latest compatible versions
amaru update --skillset <name>          # Update all members of a skillset
amaru list                              # Show installed items with status
```

## Key Concepts

- **Manifest** (`amaru.json`): Declares registries, version constraints, and dependencies. Committed to the repo.
- **Lock file** (`amaru.lock`): Resolved versions, content hashes, skillset digests. Committed for reproducibility.
- **Version constraints**: `^1.0.0` (minor+patch), `~1.0.0` (patch only), `1.0.0` (exact), `latest` (unversioned, hash-tracked).
- **Skillsets**: Registry-defined groups that expand to individual items on install. Use `--type skillset` to add.
- **Registries**: GitHub repos with `amaru_registry.json` at root and items in top-level `skills/`, `commands/`, `agents/` directories (the v2 flat layout). Older registries use a legacy `.amaru_registry/{skills,...}` layout — amaru reads both, but emits a stderr deprecation warning when loading v1; use `amaru repo migrate` to upgrade.
- **Foreign registries**: amaru can also install from non-amaru-native repos that follow either a flat shape (`skills/<name>/SKILL.md`) or a one-level-nested shape (`skills/<category>/<name>/SKILL.md` — Google-style). Examples: `vercel-labs/agent-skills` (flat), `google/skills` (nested), `jeremylongshore/claude-code-plugins-plus-skills`. Those repos have no `amaru_registry.json`, no skillsets, and no per-item version tags, so installs use the `latest` constraint and content-hash drift detection. Nested item names contain a single `/` (e.g. `cloud/bigquery`).
- **Cross-registry skillsets**: a skillset's items may carry an optional `"registry": "<alias>"` field to source that member from a different registry the consumer has configured in their `amaru.json`. Authoring syntax for `amaru repo add --type skillset --items`: `"skill/foo,command/bar@platform"`. Older amarus drop the `registry` field on read and warn-and-skip cross-registry members; tell the user to upgrade amaru if they hit that.
- **Mirrors**: a registry's `amaru_registry.json` may declare a `mirrors: ["github:other/repo"]` array. Mirror entries merge into the primary index (primary wins on collision). `amaru browse` and `amaru list` annotate mirror-contributed entries with their source URL.

## When to Use Each Command

| Situation | Command |
|-----------|---------|
| Starting a new project | `amaru init` then `amaru browse` |
| Adding a specific skill | `amaru add <name>` |
| New team member onboarding | `amaru install` (reads existing manifest) |
| Checking if anything is outdated | `amaru check` or `amaru check --quiet` |
| Updating everything | `amaru update` |
| Updating one skillset | `amaru update --skillset <name>` |
| Accepting local edits to a skill | `amaru ignore <name>` |
| Setting up shared docs | `amaru context init` |
| Creating a new registry | `amaru repo init` |
| Adding a skill to a registry | `amaru repo add <name>` |
| Publishing a version | `amaru repo tag <name> <version>` |
| Checking registry health | `amaru repo validate` |
| Listing registry contents | `amaru repo list` |
| Removing unused items | `amaru repo remove <name>` |
| Viewing item details | `amaru repo info <name>` |
| Releasing + cascading into skillsets | `amaru repo tag <name> <version> --cascade` |
| Upgrading a registry's layout | `amaru repo migrate` (in-place) |
| Upgrading someone else's registry | `amaru repo migrate <url>` (clone + side-branch + push, prints `gh pr create`) |

## Registry URL Formats

`amaru init` accepts any GitHub URL format:
- `github:org/repo` (canonical)
- `git@github.com:org/repo.git` (SSH)
- `https://github.com/org/repo`
- `github.com/org/repo` (bare domain)

All formats are normalized automatically.

## Files Managed

| Path | Purpose |
|------|---------|
| `amaru.json` | Manifest (version constraints, registries) |
| `amaru.lock` | Lock file (resolved versions, hashes) |
| `.claude/skills/<name>/` | Installed skills |
| `.claude/commands/<name>/` | Installed commands |
| `.claude/agents/<name>/` | Installed agents |

## Context Documentation

```bash
amaru context init    # Sparse-clone context docs from registry
amaru context sync    # Pull latest context
amaru context push    # Push local changes back
```

Context docs are synced to `docs/context/` (configurable) and auto-sync via git hooks.

## Registry Authoring

Manage items in a registry repository:

```bash
amaru repo init /path/to/registry       # Scaffold empty v2 registry (flat layout)
amaru repo add my-skill                  # Create new skill with templates
amaru repo add my-cmd --type command     # Create new command
amaru repo add my-agent --type agent     # Create new agent
amaru repo add pack --type skillset --items "skill/my-skill,command/my-cmd"
amaru repo list                          # Show all items
amaru repo validate                      # Check consistency
amaru repo tag my-skill 1.0.0           # Tag version + update index
amaru repo tag my-skill 1.1.0 --cascade # ...and patch-bump every skillset that contains this item
amaru repo info my-skill                 # Show item details
amaru repo remove old-skill              # Remove from registry
amaru repo migrate                       # Upgrade local registry from .amaru_registry/ → flat (v1 → v2)
amaru repo migrate github:org/repo       # Clone, migrate, push migration/flat-layout branch + print PR command
```

### Migration safety

- `amaru repo migrate` is idempotent — running it on an already-flat registry is a no-op.
- A `.migrating` journal file is written at the registry root before any rename and removed on success. If a run crashes mid-migration, re-running prints an exact recovery message.
- Sparse profiles are rewritten via an anchored line-prefix algorithm; originals are preserved as `.bak` files. Use `--skip-sparse-rewrite` for hand-customized profiles.
- Remote-mode default behavior is to push a `migration/flat-layout` side branch. `--push-to-default` opts into pushing onto the remote's default branch. Pre-push the command verifies the cloned HEAD still matches the remote tip — a moved remote refuses the push. Each git invocation is bounded by a 60s timeout.

Older amarus running against a v2 registry get a clean "unknown amaru_version" error rather than a confusing 404. Tell the user to upgrade amaru if you see that.
