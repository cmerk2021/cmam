# cmam-registry

The app registry for [cmam](https://github.com/cmerk2021/cmam). Each app is
one small TOML manifest in `apps/`, and a GitHub Action regenerates
`index.json` (which the cmam CLI fetches) on every push to `main`.

## Adding an app (~30 seconds)

1. Create `apps/<name>.toml`:

   ```toml
   name = "wgo"
   description = "Watch files and run commands"
   repo = "cmerk2021/wgo"
   ```

   Rules:
   - `name` must match the filename and be lowercase `[a-z0-9-]+`.
   - `description` and `repo` must be non-empty; `repo` is `owner/name`.
   - Keep values as simple `key = "string"` pairs.

2. Commit and push to `main`. The `index` workflow validates the manifest
   and commits the regenerated `index.json`. If validation fails, the
   workflow fails and `index.json` is left untouched.

The app's repository must publish releases with the CMAM asset convention —
see `templates/app-release/` in the cmam repo.

`apps/example.toml` is a template and is excluded from the index.

## Layout

```
apps/<name>.toml          one manifest per app
index.json                generated — do not edit by hand
tools/genindex/           validator + index generator (stdlib only)
.github/workflows/index.yml
```

Regenerate locally with:

```sh
go run ./tools/genindex
```
