# Manifest format

`addon_manifest.yml` is the source of truth: one entry per addon, in plain YAML.

## An entry

```
github.com/owner/my_addon:
  name: "My Addon"
  url: https://example.com/addon.zip
  path: addons/my_addon
  version: "1.2.3"
  tag: "v1.2.3"
```

- `name` — what the addon calls itself. A label: it's what you see in lists and
  messages, and nothing is looked up by it
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

## Keys and names

The key — the line at column 0 — is the entry's identity. New entries are keyed by the
addon's repo, `<host>/<owner>/<repo>`, so re-adding the same addon finds the entry it
already has instead of making a second one, whatever the addon has renamed itself to
since.

Keys you write yourself are just as valid. `my_addon:` works exactly as it always did,
nothing is rewritten, and you can key an entry however you like — which is how you track
two checkouts of one repo side by side.

`name` is separate from all that, and it's the part you read. gdaddon fills it in on
install from the addon's own `plugin.cfg`, if that file declares one, and never
overwrites a name that's already there — so type your own and it stays. Clear it and
lists fall back to the last segment of the key. The Edit Manifest action sets both.

## Optional keys

- `commit: "abc1234…"` — a branch snapshot pinned to this exact commit
- `lock: true` — pin the version and stop reporting updates for it
- `suppress_deps: ["owner/repo"]` — declared dependencies to ignore (see the
  Dependencies page)

`path` may be omitted. gdaddon then derives one — `addons/` plus the key's last segment,
unless the package's `plugin.cfg` declares its own `dir=` (or `path=`), which wins — and
writes the result back into the manifest on install. The `name` never decides a folder:
an addon calling itself `My Addon` still installs to `addons/my_addon`.

A package that nests its plugin under a namespace folder
(`addons/addon_lib/tree_sitter_gd/`) derives the full path, namespace and all. The addon
is the folder holding the config file, never the level above it.

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
