# cmam

A tiny, polished package manager for my Go CLI tools on Linux. It installs
prebuilt, SHA-256-verified release binaries from GitHub into `~/.local/bin`
— no sudo, no compiling, with a pretty terminal UI.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/cmerk2021/cmam/main/install.sh | sh
```

Or build from source (Go 1.22+):

```sh
make install   # builds and copies cmam to ~/.local/bin
```

## Commands

| Command | What it does |
| --- | --- |
| `cmam install <app>[@version] [app...]` | Install the latest or a specific version |
| `cmam update [app]` | Update one app, or all installed apps |
| `cmam uninstall <app>` | Remove an app (asks unless `-y`) |
| `cmam list [--json]` | Installed apps + available updates |
| `cmam search [query] [--json]` | Browse or search the registry |
| `cmam info <app> [--json]` | Details, latest version, install status |
| `cmam self-update` | Update cmam itself |
| `cmam doctor` | Health diagnostics with fix hints |
| `cmam clean [--dry-run]` | Clear the download cache |
| `cmam version` | Version, commit, build date |
| `cmam completion bash\|zsh\|fish` | Shell completions |

Global flags: `--no-color`, `--verbose`, `--yes`. `install`, `update`, and
`search` accept `--refresh` to force a registry refetch.

## How it works

```
cmam install wgo
  1. Resolve   look up wgo in the registry (cmerk2021/cmam-registry),
               resolve the latest release tag of its repo
  2. Download  <repo>/releases/download/v1.2.0/wgo_1.2.0_linux_<arch>.tar.gz
               (+ checksums.txt) into ~/.cache/cmam/downloads
  3. Verify    SHA-256 against checksums.txt — never skipped
  4. Extract   just the `wgo` binary, safely (no traversal/symlinks)
  5. Install   atomic rename into ~/.local/bin, backup + rollback
  6. Record    ~/.local/share/cmam/installed.json
```

- **Registry**: one TOML per app in
  [`cmam-registry`](https://github.com/cmerk2021/cmam-registry); the CLI
  fetches the generated `index.json` over raw HTTPS (cached for 1 hour with
  ETag revalidation, stale cache used offline), falling back to the GitHub
  contents API.
- **Releases**: resolved via the `releases/latest` redirect (no API quota),
  falling back to the REST API. Pre-releases and drafts are skipped.
- **UI**: Bubble Tea step list with spinners and a progress bar on a TTY;
  plain line output when piped or in CI. `--json` output is always clean
  JSON on stdout.
- Concurrency-safe via a lock file; Ctrl+C aborts cleanly and removes
  partial downloads.

## Adding an app to the registry

Create `apps/<name>.toml` in the registry repo and push:

```toml
name = "wgo"
description = "Watch files and run commands"
repo = "cmerk2021/wgo"
```

A GitHub Action validates it and regenerates `index.json`. That's it.

## Setting up releases for a new app

Copy the two files from [`templates/app-release/`](templates/app-release/)
into the app repo (`.goreleaser.yaml` to the root, `release.yml` to
`.github/workflows/`), then:

```sh
git tag v1.0.0 && git push --tags
```

This produces the assets cmam expects:
`<name>_<version>_linux_{amd64,arm64}.tar.gz` + `checksums.txt`, with the
binary named `<name>` at the archive root.

## Environment variables

| Variable | Effect |
| --- | --- |
| `GITHUB_TOKEN` | Sent on GitHub API calls; raises the rate limit |
| `NO_COLOR` | Disables colored output (also `--no-color`) |
| `CMAM_HOME` | Overrides every cmam directory (mainly for tests) |
| `CMAM_NO_UPDATE_CHECK` | Disables the daily background self-update check |

## Paths

| Purpose | Location |
| --- | --- |
| Binaries | `~/.local/bin` |
| State | `$XDG_DATA_HOME/cmam/installed.json` (default `~/.local/share/cmam`) |
| Cache | `$XDG_CACHE_HOME/cmam/{registry,downloads}` (default `~/.cache/cmam`) |
| Lock | `$XDG_STATE_HOME/cmam/lock` (default `~/.local/state/cmam`) |

## Troubleshooting

**"command not found" after installing** — `~/.local/bin` isn't on your
PATH. Run `cmam doctor`; it prints the exact line to add for your shell,
e.g. `export PATH="$HOME/.local/bin:$PATH"` in `~/.bashrc`.

**Rate limit errors** — unauthenticated GitHub API calls are limited to
60/hour. `export GITHUB_TOKEN=<a personal access token>` and retry. Most
operations (registry index, latest-tag resolution, downloads) avoid the API
entirely, so this is rare.

**Stale registry** — cmam shows "using cached registry from Nh ago" when
offline. Force a refetch with `cmam search --refresh`.

**A different `<app>` runs than the one installed** — another binary earlier
in PATH shadows it; `cmam doctor` detects and names it.

## Development

```sh
make build      # build ./cmam with version info
make test       # go vet + go test ./...
make lint       # golangci-lint run
make snapshot   # goreleaser snapshot build
```

Repo layout: `cmd/` (Cobra commands), `internal/registry` (manifest fetch +
cache), `internal/github` (release resolution + downloads),
`internal/installer` (verify/extract/atomic install), `internal/state`,
`internal/selfupdate`, `internal/doctor`, `internal/ui` (Reporter interface,
Bubble Tea + plain implementations, styles). `registry-seed/` contains the
initial contents for the cmam-registry repo.

## License

MIT
