# Release setup for CMAM-managed apps

Copy these two files into any Go CLI repo to make its releases installable
with `cmam install <name>`:

1. `.goreleaser.yaml` → the repo root.
2. `release.yml` → `.github/workflows/release.yml`.

Requirements:

- The module's **main package is at the repo root** (adjust `builds[].main`
  if it lives in `./cmd/<name>`).
- The binary name defaults to the repo name (GoReleaser's `ProjectName`),
  which must match the app name in the CMAM registry.

Then cut a release:

```sh
git tag v1.0.0
git push --tags
```

The workflow builds `linux/amd64` and `linux/arm64`, producing exactly the
assets CMAM expects:

```
<name>_1.0.0_linux_amd64.tar.gz
<name>_1.0.0_linux_arm64.tar.gz
checksums.txt
```

This naming convention (binary named `<name>` at the archive root, version
without a leading `v` in filenames, sha256 `checksums.txt`) is what the CMAM
client assumes — don't rename the templates' `name_template` values.
