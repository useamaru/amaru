# Amaru

**Skills, commands & agents manager.**

amaru connects your projects to centralized registries hosted on GitHub — managing skills, commands, and agents. It tracks versions, detects local drift, warns about updates, syncs shared context documentation, and keeps your whole team in sync — all through a simple manifest file.

> *The name **amaru** comes from the mythical Andean serpent — a symbol of transformation and connection between worlds. The tool connects centralized knowledge (registries) with local context (projects).*

---

## Install

**With npx** (no install — runs the latest published version):

```bash
npx amaru init
npx amaru install
```

**With npm** (global install):

```bash
npm install -g amaru
amaru --help
```

The npm package is a thin wrapper around the Go binary — it downloads the
prebuilt release for your platform from GitHub Releases on `npm install`. See
[`npm/README.md`](npm/README.md) for environment variables (`AMARU_VERSION`,
`AMARU_SKIP_DOWNLOAD`, etc.).

**Quick install** (Linux/macOS, no Node required):

```bash
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/' | sed 's/aarch64/arm64/')
curl -fsSL "https://github.com/useamaru/amaru/releases/latest/download/amaru_${OS}_${ARCH}.tar.gz" | tar xz
sudo mv amaru /usr/local/bin/
```

**From GitHub Releases** (all platforms including Windows):

Download the binary for your platform from [github.com/useamaru/amaru/releases](https://github.com/useamaru/amaru/releases).

**With Go:**

```bash
go install github.com/useamaru/amaru@latest
```

**From source:**

```bash
git clone https://github.com/useamaru/amaru.git
cd amaru
go build -o amaru .
```

## Quick Start

```bash
# 1. Initialize a manifest in your project
amaru init

# 2. Discover what's available
amaru browse

# 3. Add skills, commands, and agents
amaru add research
amaru add dev/bootstrap --type command
amaru add code-reviewer --type agent

# 3b. Or add a whole skillset at once
amaru add starter-pack --type skillset

# 4. Install everything
amaru install

# 5. Set up shared context documentation
amaru context init

# 6. Check for updates later
amaru check
```

## How It Works

amaru manages two files at the root of your project:

| File | Purpose | Committed? |
|---|---|---|
| `amaru.json` | Manifest — declares registries, skills, commands, agents, skillsets, and context config | Yes |
| `amaru.lock` | Lock — resolved versions, hashes, skillset digests, timestamps for reproducibility | Yes |

Skills are installed to `.claude/skills/`, commands to `.claude/commands/`, and agents to `.claude/agents/`, matching the Claude Code convention.

### Manifest (`amaru.json`)

```jsonc
{
  "version": "1.0.0",
  "registries": {
    "main": {
      "url": "github:acme-org/acme-skills",
      "auth": "github"
    },
    "platform": {
      "url": "github:acme-org/platform-skills",
      "auth": "github"
    }
  },
  "skills": {
    "research": { "version": "^1.0.0", "registry": "main" },
    "plan": { "version": "^1.0.0", "registry": "main" },
    "deploycheck": { "version": "^1.0.0", "registry": "platform" }
  },
  "commands": {
    "dev/bootstrap": { "version": "^2.0.0", "registry": "main" }
  },
  "agents": {
    "code-reviewer": { "version": "^1.0.0", "registry": "main" }
  },
  "context": {
    "registry": "main",
    "project": "my-app",
    "path": "docs/context"
  }
}
```

**Shorthand** — when you have a single registry, skip the `registry` field:

```jsonc
{
  "version": "1.0.0",
  "registries": {
    "main": { "url": "github:acme-org/acme-skills", "auth": "github" }
  },
  "skills": {
    "research": "^1.0.0",
    "plan": "^1.0.0"
  },
  "agents": {
    "code-reviewer": "^1.0.0"
  }
}
```

**Version ranges** follow npm conventions: `^` (minor + patch), `~` (patch only), exact (`1.2.3`).

## Commands

### `amaru init`

Interactive setup — creates `amaru.json` with your registries configured.

```
$ amaru init
Registry URL (ex: github:org/skills-repo): git@github.com:acme-org/acme-skills.git
  → normalized to: github:acme-org/acme-skills
Registry alias [acme]: acme
Auth method (github/token/none) [github]: github
Add another registry? (y/N): N

amaru.json created. Run `amaru browse` to see available skills.
```

Accepts any GitHub URL format — SSH (`git@github.com:org/repo.git`), HTTPS (`https://github.com/org/repo`), `ssh://`, `http://`, bare domain (`github.com/org/repo`), or the canonical shorthand (`github:org/repo`). All formats are normalized automatically.

### `amaru install [--force]`

Resolves versions, downloads files from registries, writes to `.claude/`, and generates the lock file.

```
$ amaru install
Authenticating registries...
  ✓ main (github:acme-org/acme-skills) — via gh CLI

Installing skills...
  ✓ research@1.0.3 (main)
  ✓ plan@1.0.1 (main)

Installing commands...
  ✓ dev/bootstrap@2.0.0 (main)

Installing agents...
  ✓ code-reviewer@1.0.0 (main)

Lock file updated.
```

Idempotent — won't reinstall if the lock already matches. Use `--force` to override.

### `amaru check [--quiet]`

Compares your lock against the registries. Reports available updates and local drift.

```
$ amaru check
⚠ Updates available:
  research: 1.0.3 → 1.1.0 (minor) [main]
  compound: 1.1.0 → 2.0.0 (MAJOR — breaking) [main]

⚠ Drift detected (locally edited):
  plan: hash local b2c3d4 ≠ central a1b2c3 (v1.0.1) [main]

✓ 7 skills/commands up to date
```

Use `--quiet` for the compact box format (designed for session-start hooks):

```
╭──────────────────────────────────────────────────╮
│ 🐍 amaru: 2 update(s) available                  │
│   research 1.0.3 → 1.1.0 [main]                 │
│   compound 1.1.0 → 2.0.0 (MAJOR) [main]         │
│                                                  │
│   Run `amaru update` to update                   │
╰──────────────────────────────────────────────────╯
```

Results are cached for 4 hours so it doesn't slow down your session startup.

### `amaru update [name]`

Updates to the latest version compatible with your declared ranges.

```
$ amaru update research
  ✓ Updating research: 1.0.3 → 1.1.0 (minor) [main]

Lock file updated.
```

Without arguments, updates everything.

### `amaru update --skillset <name>`

Updates all members of a skillset. Detects new members added to the skillset in the registry and installs them automatically.

```
$ amaru update --skillset starter-pack
Updating skillset "starter-pack" (4 members)...
  ✓ Updating research: 1.0.3 → 1.1.0 (minor) [main]
  ✓ Added command deploy-check@1.0.0 (new member)

Skillset "starter-pack": 1 updated, 1 added.
```

The skillset digest in the lock file tracks whether any member versions have changed since the last install.

### `amaru list`

Shows what's installed, with status and origin.

```
$ amaru list
Skills:
  research      1.0.3  ✓ up-to-date    [main] (via starter-pack)
  plan          1.0.1  ⚠ 1.0.2 avail   [main] (via starter-pack)
  deploycheck   1.0.0  ✓ up-to-date    [platform]

Commands:
  dev/bootstrap 2.0.0  ✓ up-to-date    [main]

Agents:
  code-reviewer 1.0.0  ✓ up-to-date    [main]
```

Items installed via a skillset show their group provenance.

### `amaru add <name> [--type <type>] [--registry <alias>]`

Adds a skill, command, agent, or skillset to the manifest and installs it in one step.

```bash
amaru add research                         # add a skill (default)
amaru add dev/bootstrap --type command     # add a command
amaru add code-reviewer --type agent       # add an agent
amaru add deploycheck --registry platform  # specify registry
amaru add starter-pack --type skillset     # add all items in a skillset
```

If `--registry` is omitted and there are multiple registries, amaru searches all of them.

**Skillsets** are registry-defined groups of skills, commands, and agents. Adding a skillset expands it to individual items in your manifest, each tagged with its origin group. Items that are already in your manifest are skipped.

**Unversioned items** (no git tags in the registry) are tracked with the `"latest"` constraint — amaru downloads from the default branch and uses content hashing instead of semver for change detection.

### `amaru browse [--registry <alias>]`

Discover what's available across your configured registries.

```
$ amaru browse
[main] github:acme-org/acme-skills
  Skills:
    research      1.0.3  [dev, core]      Search codebase and return compressed context
    plan          1.0.1  [dev, core]      Create plans with code snippets
  Commands:
    dev/bootstrap 2.0.0  [dev, setup]     Project bootstrap
  Agents:
    code-reviewer 1.0.0  [dev, review]    Review code changes with context
  Skillsets:
    starter-pack  (3 items) [onboarding]  Essential skills for new projects

[platform] github:acme-org/platform-skills
  Skills:
    deploycheck   1.0.0  [platform]       Verify deploy prerequisites
```

### `amaru ignore <name>` / `amaru unignore <name>`

Accept local drift for a specific item — `amaru check` will stop warning about hash mismatches for it.

```bash
amaru ignore plan        # accept local edits
amaru unignore plan      # re-enable drift warnings
```

### `amaru context init`

Sets up shared context documentation for your project. Sparse-clones the context directory from your registry into `.claude/.amaru-context/` and symlinks it to `docs/context`.

```bash
$ amaru context init
Cloning context for project "my-app"...
  ✓ Sparse checkout from main registry
  ✓ Symlinked to docs/context
  ✓ Added .claude/.amaru-context/ to .gitignore
  ✓ Installed git hooks (post-checkout, post-commit)
```

This gives your project access to shared brainstorms, plans, and solutions from the centralized registry.

### `amaru context sync`

Pulls the latest context documentation from the registry.

```bash
amaru context sync
```

This runs automatically via the post-checkout git hook after branch switches.

### `amaru context push`

Stages and pushes local context changes back to the centralized registry.

```bash
amaru context push
```

The post-commit git hook auto-pushes when it detects changes to context files.

### `amaru context path`

Prints the local context directory path.

```bash
$ amaru context path
docs/context
```

### `amaru repo init <path> --project <name>`

Scaffolds a new registry repository with the standard directory structure.

```bash
$ amaru repo init /path/to/registry --project my-app
Creating registry at /path/to/registry...
  ✓ registry.json
  ✓ AGENTS.md
  ✓ skills/
  ✓ commands/
  ✓ agents/
  ✓ context/my-app/
  ✓ .sparse-profiles/my-app
```

The generated structure includes `AGENTS.md` navigation files and a per-project context directory with `brainstorms/`, `plans/`, and `solutions/` subdirectories.

After scaffolding, use `amaru repo add` to create items and `amaru repo tag` to version them.

## Authentication

amaru supports three auth methods per registry:

| Method | `auth` value | How it works |
|---|---|---|
| **GitHub CLI** | `"github"` | Uses your existing `gh` auth. **Recommended.** |
| **Token** | `"token"` | Reads `AMARU_TOKEN_<ALIAS>` env var. Good for CI/CD. |
| **Public** | `"none"` | No auth. For public registries. |

For token auth, the env var name is derived from the registry alias in uppercase:

```bash
# For a registry aliased as "platform":
export AMARU_TOKEN_PLATFORM="ghp_xxxxxxxxxxxx"
```

## Registry Structure

A registry is just a GitHub repo with this layout (v2 — flat):

```
my-skills-registry/
├── amaru_registry.json    # Package index (auto-updated by CI; amaru_version: "2")
├── AGENTS.md              # Root navigation + registry structure
├── .sparse-profiles/      # Sapling sparse checkout profiles
│   └── my-app
├── skills/
│   ├── research/
│   │   ├── skill.md       # The skill content
│   │   ├── manifest.json  # Metadata + version
│   │   └── examples/      # Optional
│   └── plan/
│       ├── skill.md
│       └── manifest.json
├── commands/
│   └── dev/
│       └── bootstrap/
│           ├── command.md
│           └── manifest.json
├── agents/
│   └── code-reviewer/
│       ├── agent.md
│       └── manifest.json
└── context/
    └── my-app/
        ├── AGENTS.md      # Per-project navigation
        ├── brainstorms/
        ├── plans/
        └── solutions/
```

This shape matches other Claude Code skill registries (e.g. `vercel-labs/agent-skills`, `jeremylongshore/claude-code-plugins-plus-skills`), so any such repo is droppable as an amaru registry without restructuring. amaru still reads the legacy `.amaru_registry/`-prefixed (v1) layout for backward compatibility — run `amaru repo migrate` to upgrade an old registry to v2 (see Commands).

**Folders** — skills/commands/agents can optionally be organized into folders for source-tree readability:

```
skills/
├── research/                # flat — addressed as "research"
└── dev/                     # folder (cosmetic, not part of the name)
    └── bootstrap/           # addressed as "bootstrap" (NOT "dev/bootstrap")
        ├── manifest.json
        └── skill.md
```

Folders are purely a registry-side organization aid. The folder name never appears in the item's identity:

- Item name in `amaru.json`: `bootstrap` (not `dev/bootstrap`)
- Git tag: `skill/bootstrap/1.0.0` (no folder)
- Install path: `.claude/skills/bootstrap/` (no folder)
- Index key: `bootstrap`, with a `"folder": "dev"` field on the entry

Create a folder-organized item with `amaru repo add <name> --folder <folder>`. Existing flat items keep working unchanged.

Versions are tracked via git tags: `skill/research/1.0.3`, `command/dev/bootstrap/2.0.0`, `agent/code-reviewer/1.0.0`.

Skillsets are defined in `amaru_registry.json`:

```jsonc
{
  "skillsets": {
    "starter-pack": {
      "description": "Essential skills for new projects",
      "tags": ["onboarding"],
      "items": [
        { "type": "skill", "name": "research" },
        { "type": "skill", "name": "plan" },
        { "type": "command", "name": "dev/bootstrap" },
        // Cross-registry member — sourced from registry alias "platform",
        // which the consumer must have configured in their amaru.json.
        { "type": "command", "name": "deploy", "registry": "platform" }
      ]
    }
  }
}
```

**Cross-registry skillsets** let one registry's skillset pull members from other registries the consumer has configured. The optional `registry` field on a member overrides the skillset's home registry; omit it to source from the home registry (the default). When authoring with `amaru repo add ... --type skillset`, use the `type/name@<alias>` syntax: `--items "skill/research,command/deploy@platform"`.

**Foreign-registry layouts** (no `amaru_registry.json`) work in two shapes:

- *Flat:* `skills/<name>/SKILL.md` — every direct child of `skills/` is a skill (e.g. `vercel-labs/agent-skills`).
- *Nested:* `skills/<category>/<name>/SKILL.md` — items live one level deeper, addressed by `<category>/<name>` (e.g. `google/skills` with `skills/cloud/bigquery`, `skills/data/schema-design`).

Detection is automatic: if a top-level child of `skills/` has a `SKILL.md`/`skill.md`, it is treated as a flat skill; otherwise it is treated as a category and its grandchildren are probed. The same rule applies to `commands/` and `agents/`. Recursion stops at one level — anything deeper requires the registry to ship `amaru_registry.json` with explicit entries.

amaru accesses registries through the GitHub API for installable items. For context sync, it uses sparse checkout via Sapling (preferred) or Git.

## VCS Support

amaru supports two version control backends for context sync:

| Backend | Detection | Sparse checkout method |
|---|---|---|
| **Sapling** | `sl` on PATH | `sl clone --enable-profile` with sparse profiles |
| **Git** | Fallback | `git clone --filter=blob:none --no-checkout` + `git sparse-checkout set` |

Sapling is preferred when available — it handles sparse checkouts of large registries more efficiently. amaru auto-detects the available backend.

## Hooks

`amaru context init` installs two git hooks automatically:

- **post-checkout** — runs `amaru context sync` after branch switches
- **post-commit** — detects context file changes and auto-pushes via `amaru context push`

Hooks are idempotent (safe to re-install) and fail silently — they never block your workflow.

### Session Start Hook

To get automatic update warnings when you start a Claude Code session, add a hook:

```bash
# .claude/hooks/session-start.sh
#!/bin/bash
if [ -f "amaru.json" ]; then
  amaru check --quiet 2>/dev/null
fi
```

## Registry Management

Manage items in a registry repository with `amaru repo` subcommands. These commands operate on the local registry (the directory containing `amaru_registry.json`).

### `amaru repo add <name> [--type skill|command|agent|skillset]`

Create a new item in the registry with template files and update the index.

```bash
$ amaru repo add research
  ✓ Created skill "research"
  Directory: skills/research/
  Content:   skills/research/skill.md

$ amaru repo add deploy --type command -d "Deploy to production"
  ✓ Created command "deploy"

$ amaru repo add bootstrap --folder dev
  ✓ Created skill "bootstrap"
  Directory: skills/dev/bootstrap/
  Content:   skills/dev/bootstrap/skill.md

$ amaru repo add starter-pack --type skillset --items "skill/research,command/deploy"
  ✓ Created skillset "starter-pack" with 2 items
```

Pass `--folder <name>` to organize an item under a subdirectory — the folder is cosmetic (not part of the item's name, tag, or install path). See "Folders" in the Registry Structure section.

### `amaru repo remove <name> [--type skill|command|agent|skillset] [--force]`

Remove an item from the registry index and delete its files. Blocks if the item is referenced by a skillset (use `--force` to override).

```bash
$ amaru repo remove old-skill
  ✓ Removed skill "old-skill" from registry
```

### `amaru repo list [--type skill|command|agent|skillset] [--json]`

List all items in the local registry, grouped by type.

```bash
$ amaru repo list
Skills (2)
  research     v1.2.0   Tools for deep codebase research
  amaru-usage  latest   How to use amaru CLI

Commands (0)

Agents (0)

Skillsets (0)
```

### `amaru repo validate`

Check registry consistency — verifies the index matches the filesystem, manifests are valid, and skillset members exist. Exits non-zero on errors (for CI).

```bash
$ amaru repo validate
Validating registry at .
  ✓ skills/research — OK
  ✗ skills/broken — manifest.json not found
  ! skills/orphan — orphaned directory (not in index)

Errors: 1  Warnings: 1  OK: 1
```

### `amaru repo tag <name> <version> [--type skill|command|agent] [--push]`

Tag a new version of an item — updates `manifest.json` and the index, then creates an annotated git tag.

```bash
$ amaru repo tag research 1.0.0
  ✓ Tagged skill "research" as v1.0.0
  Tag: skill/research/1.0.0

  To push: git push --follow-tags
```

Pass `--cascade` to also patch-bump every skillset whose `Items` include the tagged item:

```bash
$ amaru repo tag research 1.1.0 --cascade
  ✓ Tagged skill "research" as v1.1.0
  Tag: skill/research/1.1.0
  ✓ Cascaded patch bumps into 2 skillset(s): starter-pack, power-pack
```

The cascade only updates the skillset's `Latest` in the index — it does not create per-skillset git tags. A skillset whose `Latest` was unset starts at `0.1.0` on the first cascade. The cascade aborts cleanly (with no partial writes) if any affected skillset's `Latest` is non-empty but not valid semver.

### `amaru repo migrate [<url>]`

Convert a registry from the legacy nested layout (`.amaru_registry/{skills,...}`) to the v2 flat layout (`skills/`, `commands/`, `agents/`, `context/`, `.sparse-profiles/` at the root).

**In-place** (no argument) — migrates the registry in the current directory. Idempotent: a v2 registry is a no-op. A `.migrating` journal at the registry root protects against half-migrated state if the run crashes; sparse profiles are rewritten with an anchored-prefix algorithm and originals preserved as `.bak`.

```bash
$ amaru repo migrate --dry-run
Dry run — the following moves would be performed:
  .amaru_registry/skills -> skills
  .amaru_registry/commands -> commands
  .amaru_registry/agents -> agents
  .amaru_registry/context -> context
  .amaru_registry/.sparse-profiles -> .sparse-profiles
…

$ amaru repo migrate
  ✓ Migrated to v2 flat layout (5 move(s)).
  ✓ Rewrote 1 sparse profile(s); originals preserved as .bak files.

  Next steps:
    1. Review the diff: git status && git diff
    2. Stage and commit: git add -A && git commit -m "chore: migrate registry to flat layout"
    3. Push: git push
```

Flags:
- `--dry-run` — print planned moves without changing anything.
- `--allow-dirty` — proceed even if `git status` is non-empty (default: refuse).
- `--skip-sparse-rewrite` — leave `.sparse-profiles/*` files untouched (for hand-customized profiles).
- `--strict-children` — refuse to migrate if `.amaru_registry/` contains non-canonical children (e.g. `.DS_Store`); without the flag, those children move verbatim.

**Remote** (`amaru repo migrate <url>`) — clones the registry to a temp directory, runs the in-place migration, commits with a fixed message, and pushes. By default the migration commit lands on a `migration/flat-layout` side branch and the command prints a `gh pr create` next-step. Pre-push the command verifies the cloned HEAD is still the remote tip; if the remote moved during migration, it refuses. Each git invocation is bounded by a 60-second timeout to prevent credential prompts from hanging the process.

```bash
$ amaru repo migrate github:acme-org/acme-skills
  ✓ Migrated github:acme-org/acme-skills and pushed branch "migration/flat-layout" (commit 3f9a2c1d).

  Next step — open a PR:
    gh pr create --repo acme-org/acme-skills --head migration/flat-layout --title "Migrate registry to flat layout"
```

Additional remote-mode flags:
- `--push-to-default` — push the migration commit directly onto the default branch instead of a side branch (skips the PR step).
- `--branch <name>` — override the side branch name.
- `--protocol ssh|https` — clone protocol (default ssh).

### `amaru repo info <name> [--type skill|command|agent]`

Show detailed information about a specific item in the registry.

```bash
$ amaru repo info research
Name:        research
Type:        skill
Version:     latest (unversioned)
Description: Tools for deep codebase research
Author:      barelias
Tags:        research, tooling
Files:       skill.md
```

## Self-Hosted Registry

This repo is its own registry — it ships an `amaru-usage` skill that teaches Claude Code how to use amaru. Any tool can do the same: add `amaru_registry.json` and a top-level `skills/` (or `commands/`, `agents/`) directory to your repo.

```bash
# In any project:
amaru init                    # Use github:useamaru/amaru as the registry URL
amaru add amaru-usage         # Install the amaru-usage skill
```

## License

MIT
