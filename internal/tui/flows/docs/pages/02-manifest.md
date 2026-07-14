# Manifest format

`addon_manifest.yml` is the source of truth: one entry per addon, in plain YAML.

## An entry

```
my_addon:
  url: https://example.com/addon.zip
  path: addons/my_addon
  version: "1.2.3"
  tag: "v1.2.3"
```

- `url` — a `.zip` to download, or a `.git` repo to clone
- `path` — where it installs, relative to the project root
- `version` — the version in the addon's own `plugin.cfg`; if it already matches, the
  install is skipped
- `tag` — the release the addon was installed from. This is the release identity, and
  it's what dependency specs are matched against

`version` and `tag` can disagree, and that's fine: `version` is whatever the addon author
typed into `plugin.cfg`, while `tag` is the release you actually took.

You can edit the file by hand. gdaddon rewrites entries a line at a time and leaves your
comments and formatting alone.

## Optional keys

- `commit: "abc1234…"` — a branch snapshot pinned to this exact commit
- `lock: true` — pin the version and stop reporting updates for it
- `suppress_deps: ["owner/repo"]` — declared dependencies to ignore (see the
  Dependencies page)

`path` may be omitted. gdaddon then derives one — `addons/<name>`, unless the package's
`plugin.cfg` declares its own `dir=`, which wins — and writes the result back into the
manifest on install.

## Zip or git

A `.zip` url is downloaded and unpacked. Nothing else happens; it's a plain snapshot.

A `.git` url can be installed two ways, and gdaddon asks which when you pick a branch:

- Clone (the default) — a real `git clone`, `.git` and all. Recorded as `kind: clone`
  with `tag: <branch>`. It stays a git checkout that you can pull, branch, and commit in.
  gdaddon won't overwrite it.
- Package — resolve the branch's current HEAD to a commit, download that commit's
  archive, and record `commit: <sha>`. A clone without git's baggage: reproducible,
  because the sha is written down, but frozen.

A commit-pinned package always reads as `installed` — a snapshot with no `.git` can't be
re-verified, so the recorded pin is trusted — and it reports no updates, because a frozen
commit has no newer release to compare against. Move it to a release tag when you want
updates again.

## Branch drift

For a clone or submodule, gdaddon reads the branch actually checked out. When it differs
from the recorded `tag`, the row reads `branch changed`. That's information, not damage:
either switch the checkout back, or use the addon's Update branch record action to write
the branch you're now on into the manifest.
