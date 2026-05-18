# amaru (npm wrapper)

npm distribution of [amaru](https://github.com/useamaru/amaru) — the Go CLI
that manages skills, commands, and agents for Claude Code.

## Usage

Run without installing:

```bash
npx amaru init
npx amaru install
```

Or install globally:

```bash
npm install -g amaru
amaru --help
```

## How it works

On `npm install`, the package's `postinstall` hook downloads the matching
prebuilt binary from
[github.com/useamaru/amaru/releases](https://github.com/useamaru/amaru/releases)
for your platform (linux/darwin/windows × amd64/arm64) and drops it under
`bin/`. The `amaru` bin entry is a tiny Node launcher that execs the binary
and forwards argv, stdio, and exit code.

## Environment variables

- `AMARU_SKIP_DOWNLOAD=1` — skip the postinstall download. Useful in CI when
  you want the npm package installed but will fetch the binary another way.
- `AMARU_VERSION=<x.y.z>` — override the version tag fetched from GitHub
  Releases. Defaults to this package's own version.
- `AMARU_REPO=<owner>/<repo>` — override the GitHub repo. Defaults to
  `useamaru/amaru`.

## Supported platforms

Linux and macOS (x64, arm64) extract via the system `tar`. Windows uses the
`tar.exe` that ships with Windows 10 (1809) and later to unpack the `.zip`
release.

If your platform isn't supported by the prebuilts, build from source — see
the [main README](https://github.com/useamaru/amaru#install).
