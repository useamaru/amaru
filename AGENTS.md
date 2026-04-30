# Amaru — Agent Navigation

## Architecture

```
amaru (CLI entry point)
├── cmd/           — Cobra commands: init, add, install, update, check, list, browse, ignore, context, repo
├── internal/
│   ├── manifest/  — amaru.json + amaru.lock read/write (Manifest, Lock, DependencySpec)
│   ├── registry/  — GitHub API client, RegistryIndex, SkillsetEntry, authentication; Layout (v1 nested vs v2 flat) + path math; deprecation warning for v1 reads
│   ├── installer/ — Write files to .claude/{skills,commands,agents}/, compute content hashes
│   ├── checker/   — Compare lock against registries: detect updates + local drift
│   ├── resolver/  — Semver constraint resolution (^, ~, exact) + version classification
│   ├── types/     — ItemType enum (skill, command, agent) + shared helpers
│   ├── ui/        — Terminal formatting: colors, tables, headers, check/warn/error marks
│   ├── ctxdocs/   — Sparse-checkout context docs from registry (dual-layout-aware)
│   ├── hooks/     — Install/manage git hooks for context sync
│   ├── scaffold/  — Registry repository scaffolding (writes v2); MigrateInPlace + MigrateRemote + GitRunner abstraction
│   └── vcs/       — VCS backend detection (Sapling vs Git)
└── main.go        — Entry point
```

## Key Data Flow

1. **Add**: `cmd/add.go` → `registry.Client.FetchIndex()` → `manifest.SetDep()` → `registry.Client.DownloadFiles()` → `installer.Install()` → `manifest.SaveLock()`
2. **Install**: `cmd/install.go` → for each dep: `resolver.Resolve()` → `DownloadFiles()` → `Install()` → `SaveLock()`
3. **Update**: `cmd/update.go` → `resolver.Resolve()` finds best compatible version → downloads + installs if newer
4. **Check**: `internal/checker/checker.go` → compares locked versions against registry, detects hash drift
5. **Skillsets**: `cmd/add.go:runAddSkillset()` → validates all members → installs each → records digest in `lock.Skillsets`
6. **Repo Add**: `cmd/repo_add.go` → `scaffold.FindRegistryRoot()` → `scaffold.LoadLocalIndex()` → `registry.LayoutFor(idx)` → `scaffold.ItemManifestFor()` → write files at `layout.ItemDir(...)` → `scaffold.SaveLocalIndex()`
7. **Repo Tag (with optional --cascade)**: `cmd/repo_tag.go` → validate item exists → update manifest.json + index Latest → if `--cascade`: pre-validate every affected skillset's `Latest` is semver, then patch-bump each → `scaffold.SaveLocalIndex()` → git add + commit + tag
8. **Repo Validate**: `cmd/repo_validate.go` → `scaffold.LoadLocalIndex()` → resolve layout → walk `layout.TypeDir(...)` → cross-reference entries vs filesystem
9. **Repo Remove**: `cmd/repo_remove.go` → check skillset deps → remove from index → delete `layout.ItemDir(...)` → `scaffold.SaveLocalIndex()`
10. **Repo Migrate (in-place)**: `cmd/repo_migrate.go` → `scaffold.MigrateInPlace(dir, opts)` → classify state via 7-row recovery matrix → write `.migrating` journal → rename `.amaru_registry/<dir>` → `<dir>` → rewrite sparse profiles (anchored prefix, with `.bak`) → atomic `amaru_registry.json` rewrite (`amaru_version: "2"`) → remove journal
11. **Repo Migrate (remote)**: `cmd/repo_migrate.go` (URL form) → `scaffold.MigrateRemote(ctx, url, opts)` → mkdtemp + signal-cleanup handler → `gitRunner` clone → record HEAD → checkout side branch (default `migration/flat-layout`) → `MigrateInPlace` → `git add -A` + commit → fetch + ancestry check → push → print PR hint
12. **Mirror merge**: `registry.GitHubClient.FetchIndex()` → if index has `Mirrors[]`, fetch each via `mirrorFactory` → `index.MergeFrom(mirrorIdx, mirrorURL)` stamps each new entry's runtime-only `Source` field with the mirror URL → `cmd/browse.go` and `cmd/list.go` annotate output when `Source != ""`
13. **Synthesized-source discovery (foreign registries)**: `registry.GitHubClient.fetchSynthesizedIndex()` → for each item type, `scanSynthesizedType()` lists `<type>s/` and probes each child for `<TYPE>.md`/`<type>.md`. Hits become flat-named items. Misses fall back to `scanSynthesizedNamespace()` which lists one level deeper and probes each grandchild — hits become items named `<category>/<grandchild>`. Recursion stops at depth 1.
14. **Cross-registry skillset install**: `cmd/install.go:installSkillset()` → resolve skillset's home alias → for each item, resolve effective alias (item.Registry || home), look up the alias in the consumer's manifest (error if missing), fetch that alias's index (cached per skillset run), download via the per-item client, and lock with the per-item alias.

## Important Types

- `manifest.Manifest` — parsed amaru.json (registries, skills, commands, agents)
- `manifest.Lock` — parsed amaru.lock (locked entries + skillsets)
- `manifest.DependencySpec` — version constraint + optional registry + optional group
- `registry.RegistryIndex` — parsed amaru_registry.json from remote (entries + skillsets, includes AmaruVersion)
- `registry.RegistryEntry` / `registry.SkillsetEntry` — entries carry a runtime-only `Source string \`json:"-"\`` populated by `MergeFrom` for mirror-contributed entries
- `registry.SkillsetItem` — `{Type, Name, Registry}`; non-empty `Registry` selects a different consumer-side registry alias for that single member (cross-registry skillsets)
- `registry.Layout` — `LayoutNested` (v1) vs `LayoutFlat` (v2); methods compute filesystem and slash-path locations of items, context, sparse profiles, skillset manifests
- `registry.Client` — interface for FetchIndex, ListVersions, DownloadFiles, FetchSkillsetManifest
- `scaffold.MigrateOptions` / `RemoteMigrateOptions` / `MigrationResult` / `MigrationStatus` — migration API surface
- `scaffold.GitRunner` — interface for `git` invocations; mocked in `migrate_remote_test.go` so the remote-migrate flow can be tested end-to-end without a real network
- `types.ItemType` — "skill" | "command" | "agent"
