# gdaddon

`gdaddon` is a terminal addon manager for Godot. It tracks the plugins a project
depends on in a small YAML file, then installs, updates, version-pins, and archives
them for you — from GitHub/Codeberg releases, the Godot Asset Library, raw `.zip`
URLs, or live git clones.

Run it with no arguments inside (or above) a Godot project to launch the TUI:

```bash
gdaddon                # TUI; git root auto-detected, manifest found by walking from it
gdaddon /path/to/proj  # TUI with an explicit project root
```

There are also non-interactive modes for scripting and CI (mutually exclusive):

```bash
gdaddon --list         # print the manifest's install status, then exit
gdaddon --install      # install/update everything in the manifest, then exit
gdaddon --update       # update installed addons to their latest release, then exit
```

`--list` also has a machine-parseable mode for tools (e.g. a Godot editor plugin)
that drive `gdaddon` and render their own UI from its output:

```bash
gdaddon --list --json                  # print status as a JSON array, then exit
gdaddon --list --json --check-updates   # also resolve each addon's update state (network)
```

`--json` emits one object per manifest entry with stable, snake_case keys: `name`,
`state` (`missing`/`installed`/`mismatch`/`unversioned`/`invalid`), `kind`
(`package`/`clone`/`submodule`), `path` (manifest-relative), `full_path` (absolute
install dir), `local_version`, `pinned_version`, `tag`, `url`, `update`
(`unknown`/`current`/`available`), `latest_tag`, and `missing_deps` (an array of
`{repo_id, tag, url}`, empty when satisfied). It's always valid JSON — an empty
manifest prints `[]`.

Everything in `--json` is computed locally and instantly **except** `update`/
`latest_tag`, which require a network round-trip per addon and so stay `"unknown"`
unless you pass `--check-updates`. `--json`/`--check-updates` are modifiers on
`--list` and are ignored without it.

The TUI is organized into tabs, each one a domain: **Project**, **Global**,
**Archive**, **Search**, and **Actions**. Everything below maps onto those tabs.

---

## The project manifest

The manifest is the heart of gdaddon: a per-project `addon_manifest.yml` that records
which addons the project uses, where each one installs, and the exact version it's
pinned to. Check it into source control and a fresh clone of your game can have every
addon reinstalled at the right version with one command.

Each addon is a top-level block:

```yaml
MyAddon:
    url: https://github.com/user/repo.git   # or a release .zip, or a raw .zip
    path: addons/my_addon                    # install dir, relative to the project root
    version: "1.2.3"                         # the plugin.cfg version currently installed
    tag: "v1.2.3"                            # the release tag it was installed from
```

- **`url`** — where the package comes from. A `.git` url clones; a release/raw `.zip`
  url downloads and unzips. Different url forms for the same repo (a `.git`, a release
  asset, a generated source archive) are recognized as the same repo, so you never get
  duplicates.
- **`path`** — the install directory, relative to the project root. You usually don't
  need to set this by hand. gdaddon derives it from the package's own
  `plugin.cfg`/`version.cfg` (an author-declared `dir=` key, else `addons/<name>`) and
  writes the resolved value back on install. Set it explicitly only when the layout is
  hard to infer — for example submodule-style repos where the plugin folder name isn't
  obvious. An explicit `path` always wins.
- **`version`** — the author-controlled version string read from the installed
  `plugin.cfg`. If it already matches, install is skipped.
- **`tag`** — the release tag the package came from. This is the release *identity*
  and is what dependency specs match against. It can diverge from `version` (authors
  don't always keep them in sync), which is why dependency resolution uses `tag`.

The file is editable by hand or entirely through the TUI (each plugin's **Edit
Manifest** action). The **Project** tab lists every entry with its live state:
`not installed`, `installed v…`, `installed (no version pinned)`, or
`⚠ manifest pins X, installed Y` when the on-disk version drifts from the pin. Rows
are also flagged `⚠ [update]` when a newer release exists and `⚠ [missing deps]` when
the addon declares dependencies that aren't satisfied.

---

## Setting up a project

Start the program in your project's root, or anywhere within if it is a git repo.

It will search for a `addon_manifest.yml/yaml` up to 5 directories deep from the root. 

If no manifest exists, the TUI still launches with an empty Project list. Use
**Actions → Create Manifest** to create one.

From there you populate it by adding plugins (Search / New Plugin), by scanning for
plugins already on disk, or by importing a set — all covered below.

---

## Scanning for existing repos and plugins

If you already have addons sitting in your project — installed by hand or cloned — gdaddon
can find them and bring them under management.

**Actions → Scan installed** walks the project root (up to 4 levels, skipping
dotfolders like `.godot`/`.git`) for every top-level plugin folder, i.e. any directory
containing a `plugin.cfg` or `version.cfg`. It stops descending once it finds one, so a
nested sub-addon is reported as part of its parent rather than on its own. The scan
lists only plugins that **aren't already tracked** by a manifest entry's path.

For each found plugin it tries to suggest a source url if possible:

- A **standalone git checkout** (its own `.git` directory) contributes its `origin`
  remote (ssh `git@host:owner/repo` is normalized to `https://host/owner/repo`) and its
  currently checked-out branch. These prefill the track form's **clone** toggle and
  branch, so the entry can keep tracking that branch as a live working copy.
- An author-declared `source=` key in the plugin's `.cfg` is used when there's no git
  remote. This is not standard practice, but it is supported

If neither is available, you can copy and paste the source in.

**Submodules are deliberately skipped** — the parent repo manages those, and a
submodule's `.git` is a pointer file, so gdaddon won't mistake it for a standalone
checkout or resolve it to the parent project's remote.

---

## Adding plugins

There are two ways to add an addon to a project (or to your global library).

### Search

The **Search** tab queries a Godot addon source and hands a chosen result straight into
the add flow with its repo url prefilled. Built-in sources:

- **GitHub** — repository search (`api.github.com/search/repositories`).
- The **Godot Asset Store** - the new asset store is supported, you can either add the release packages
  or you can track the github repo if available.
- **Godot Asset Library** — the older asset libary is still available, filtered to addons and (when
  detectable) your project's Godot version. This is essentially github with extra filters/
- **Codeberg** — repository search.

Sources are config-driven (see [Configuration](#configuration)) — adding a new REST backend in `config.yml` makes
it appear in the source selector with no code changes.

### New Plugin form

**Actions → New Plugin** opens a form for adding an addon by url directly:

- **URL** (required) — a repo url, release `.zip`, or raw `.zip`. It's normalized to a
  canonical repo url.
- **Name** (optional) — derived from the url if left blank.
- **Path** (optional) — derived on install if left blank.
- **Add to** — a toggle between **Project** (the current manifest) and **Global** (your
  cross-project library, see below).

Adding only records the entry; it doesn't fetch anything. Both Search and New Plugin
reject a url whose repo is already present (matched by repo identity, not by the exact
url string), so you won't end up with the same addon twice under two url forms.

---

## Installing

Installing happens from a plugin's **Install / update** action (per addon) or
**Actions → Install/Update All** (everything in the manifest).

When you install a single addon, gdaddon fetches the repo's version listing and lets you
pick exactly what to install:

- a tagged **release** and one of its assets (an uploaded `.zip`, or the host's
  generated *Source code.zip* that every release offers),
- a **branch** HEAD (downloaded as a snapshot `.zip`),
- a branch installed as a **live git clone** (keeps `.git`, so you can develop against
  it and `git pull` updates yourself), or
- a locally **archived** copy (see [Archiving](#archiving)).

gdaddon downloads/clones, derives the install directory from the package's own
`plugin.cfg`/`version.cfg`, unpacks it into place, and then pins the result back into
the manifest — url, resolved path, installed version, and release tag. A clone records
the canonical `.git` url and tracked branch instead of a version, so re-cloning targets
the right branch.

**Install/Update All** runs the whole manifest non-interactively: each entry is
installed if missing, updated if its pin changed, and skipped if it's already at the
target version. This is the same work as `gdaddon --install` from the command line — the
one-button "set up this project's addons" operation.

---

## Resolving dependencies

Addons can declare their own dependencies, and gdaddon reads them so a plugin's
requirements come along with it.

An installed addon declares dependencies in its `plugin.cfg`:

```ini
[plugin]
name="My Plugin"

deps=["owner/repo@v1.0.0", "other/repo"]
```

Each spec is `owner/repo` with an optional `@tag` (host defaults to `github.com`). When
an addon is installed, gdaddon reads its `deps`, resolves each against your manifest, and
flags the addon `⚠ [missing deps]` if any required dependency is absent. Matching uses
the dependency's **`tag`** with a semver `>=` comparison — so a manifest entry at
`v1.2.0` satisfies a `@v1.0.0` requirement.

To act on it:

- **Per addon → Get dependencies** reads that plugin's declared deps and *adds* the
  missing ones to the manifest — tag-pinned when the spec carries a tag, repo-only (for
  you to pin later) when it doesn't. It only adds; it doesn't install. Run **Install
  All** afterward to fetch them.
- **Install All + Deps** loops the whole cycle: install everything, read newly installed
  plugins' deps, add and install the missing ones, repeat until nothing installable
  remains. This resolves transitive dependency chains in one action.

A tagless dependency lands in the manifest as a bare repo entry so you can open it and
pin the version you actually want.

---

## Updates

gdaddon checks for newer releases of your installed addons and can apply them.

**Update checking** fetches each addon's release listing and compares the installed
identity to the latest non-prerelease (falling back to the newest release if all are
prereleases). The result decorates the Project rows with `⚠ [update]`.

- An addon pinned to a specific release asset is *current* when that asset belongs to the
  latest release, *outdated* when it belongs to an older one.
- A bare repo/clone url (e.g. a scanned install with no asset to match) falls back to a
  semver `>=` comparison of the installed tag/version against the latest tag.
- Branch-tracked installs and live clones follow a moving HEAD with no release tag to
  compare against, so they're never flagged — you update those with `git` yourself.
- Anything unfetchable, releaseless, or with an uncomparable version (a date stamp, no
  version) stays **unknown** rather than showing a false update.

**Applying updates** preserves the *kind* of asset each addon currently tracks — if you
installed the uploaded `.zip`, the update grabs the new release's uploaded `.zip`; if you
tracked the source archive, it grabs that. (When the exact asset name can't be matched it
falls back to the always-present generated source archive, so an update can still
proceed.) Each updated entry is re-pinned in the manifest with the new url/path/version/
tag. A single addon failing is reported and skipped so the rest still update.

Update everything at once via **Actions → Install/Update All** (interactive, with a
confirm listing the plan) or `gdaddon --update` (non-interactive). Update checks run
concurrently across addons, so a slow host doesn't stall the whole scan.

---

## The global library

The **Global** tab is a cross-project plugin library at `~/.gdaddon/plugins.yml` —
manifest-shaped, usually url-only entries. The `~/.gdaddon` folder is
git-committable, so you can sync your library across machines.

- **Add to Global**: in the New Plugin form, set the **Add to** toggle to *Global*. From
  a project addon, use the per-plugin **Export to Global** action — it strips the url down
  to the canonical repo url (dropping a release/archive-specific url) and carries the
  install path along as a remembered default.
- **Use from Global**: select a plugin in the Global tab to open its submenu and import it
  into the current project's manifest.

This makes it easy to add commonly used plugins, one by one. For multiple, however...

### Sets

Where the global library is a flat list of individual addons, **sets** are named,
reusable groups of plugins you can drop into a project in a single action.

Sets live under `~/.gdaddon/sets/<name>.yml`, each a manifest-shaped file. Manage them in
**Actions → Sets**:

- **New set** — create an empty set, or seed it from the current project (a verbatim copy
  of the project manifest, preserving every entry's url/path/version). Names can't contain
  path separators and won't overwrite an existing set.
- **Plugins** — view and manage the plugins in a set; pin specific versions per member.
- **Add entry** — pull a plugin from your global list into the set.
- **Import to Project** — add every plugin in the set to the current project's manifest in
  one action (then **Install All** to fetch them).
- **Delete set** — remove the set file. Installed plugins are never touched.

Sets are version-aware: a member can carry its own pinned version, so importing a set
reproduces a known-good combination rather than just a list of names.

---

## Archiving

If you are worried about a repo dissapearing, but don't want to fork every plugin: The
**archive** can keep a local copy of any package's `.zip` so you can still reinstall it
later. Archived packages live under `~/.gdaddon/archive`. This can be configured to if you
wanted to host these in a separate repo, or if you wanted the .gdaddon folder in your dotfiles
without the archive bloating things.

- **Archive a package**: from a project addon's **Archive** action, browse the repo's
  versions and save a local copy of the one you want.
- **Install from the archive**: archived versions surface back into the normal version
  picker as `… (archived)` entries with local-file urls, so installing an archived copy is
  the same flow as any other install. When an upstream fetch fails entirely, gdaddon falls
  back to showing the archive-only listing, so a delisted addon is still installable.
  Installing from the archive keeps the entry's canonical repo url in the manifest (not the
  machine-specific archive path).
- **Browse and prune**: the **Archive** tab enumerates every archived repo and version. You
  can remove a single archived package or, from a project addon's Remove flow, delete a
  repo's whole archive. Emptied folders are pruned automatically.

A human-readable `index.yml` is regenerated alongside the archive for reference, though
gdaddon reads the directory tree directly.

---

## Configuration

On first run gdaddon writes `~/.gdaddon/config.yml`. It controls:

- **`archive_dir`** — where archived packages are stored (default `~/.gdaddon/archive`; a
  leading `~` is expanded).
- **`current_theme`** — the TUI color theme (also changeable via **Actions → Theme**).
- **search/VCS sources** — per-host rules for searching, listing releases/branches, and
  building source-archive urls. GitHub and Codeberg ship as defaults and double as worked
  examples; add a block for another host (or another search backend) and it's picked up
  automatically.

`~/.gdaddon/` also holds your global plugin list (`plugins.yml`), your sets (`sets/`), and
the archive — the whole directory is git-committable if you want to sync your gdaddon setup
across machines.

---

## Installing the binary

Prebuilt binaries are under Releases. On macOS, downloaded binaries may carry quarantine
status that blocks execution — clear it with:

```bash
xattr -dr com.apple.quarantine path/to/gdaddon
```

Building from source avoids that entirely. `make` cross-compiles all targets; `go build -o
build/mac-arm64/gdaddon .` builds for the current platform. The `install_unix.sh` script
symlinks the built binary into `~/.local/bin/gdaddon` so you can run `gdaddon` from any
project (update `GO_DIR` inside it if your repo path differs).
