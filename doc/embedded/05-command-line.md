# The command line

Everything gdaddon does outside this TUI is a subcommand, one per job.

```
gdaddon install --all         install everything the manifest lists, plus declared deps
gdaddon install owner/repo    install one addon, plus its declared deps
gdaddon list                  print the manifest's install status
gdaddon update-addons         update installed addons to their latest release
gdaddon update                update the gdaddon binary itself
gdaddon repos -- <command>    run a command in every git repo under the project
```

The one pair worth learning: **`update` is the gdaddon binary, `update-addons` is the
Godot addons.** `update` behaves the same here as in every other brohd11 app.

Each project-scoped command takes the project root as an optional argument
(`gdaddon list /path/to/proj`), defaulting to the git toplevel, else the current
directory. `install` takes it as `--root` instead, because its argument is the repo.

## install

`gdaddon install owner/repo` downloads the source of that addon's latest release, records it in the
project's manifest, and installs the dependencies it declares — and theirs, and so on.
Only that closure is touched: an unrelated entry sitting uninstalled in your manifest
stays uninstalled. `--all` does the whole manifest instead; that's the equivalent of
Actions ▸ Install All + Deps.

The repo is `owner/repo`, or `host/owner/repo` for a host other than github.com — the
same spec an addon's `require` entry uses, so see [Dependencies](03-dependencies.md) for
the grammar. An optional `@tag` installs that tag's
source; append `/release` for its uploaded release asset instead, which a GDExtension
needs. Without a tag you get the latest published non-prerelease, which `@latest` also
names explicitly. A leading `v` is optional on either side, so `@1.2.0` finds `v1.2.0`;
exact names take precedence. Tags without releases never become automatic update
candidates.

```
gdaddon install brohd11/Godot-Script-Tabs
gdaddon install brohd11/Godot-Script-Tabs@v0.1.4
gdaddon install brohd11/Godot-Script-Tabs@latest
gdaddon install brohd11/my-ext/release@v1.0.0
gdaddon install brohd11/my-ext/release@v1.0.0:*-linux.zip
gdaddon install codeberg.org/someone/their-addon
gdaddon install brohd11/Godot-Script-Tabs/clone@dev
gdaddon install --all
gdaddon install --all --no-deps       # the manifest's own entries only
```

When a release ships several uploads, pick one with a `:<glob>` on the spec, or with
`--asset`, which matches a substring of the name.

Naming one addon creates a manifest if the project has none, so it works in a
brand-new project. `--all` needs one to already exist.

### Where things go

The install location is worked out from the downloaded package, exactly as it is in the
TUI: a repo whose root holds a `plugin.cfg` is installed whole, a repo shipping an
`addons/` folder has its plugin folders mirrored under the project's `addons/`, and
anything else is located by its config files. A namespace folder under `addons/`
(`addons/addon_lib/tree_sitter_gd/`) is kept, so the addon is the folder holding the
config file, not the level above it. An addon can override all of this by declaring
`dir="addons/whatever"` — or `path="addons/whatever"` — in its own `plugin.cfg`, the
opt-in that makes a submodule-shaped repo land somewhere other than `addons/<repo-name>`.

### Branches

To work on an addon rather than use it, append `/clone`: it is installed as a live git
checkout, `@ref` naming the branch:

```
gdaddon install owner/repo/clone          # the remote's default branch
gdaddon install owner/repo/clone@dev      # the dev branch
```

Without `/clone`, `@main` names a Git tag and does not select a branch. Only a typed
install can ask for a clone; an addon's `require` list can't — see
[Dependencies](03-dependencies.md). A repo can keep these lines in a "for development"
section of its readme, so setting up to work on it is one paste.

The checked-out branch is recorded as the entry's `tag`, so it reads as a clone rather
than as branch drift on the next refresh. Updating a clone is git's job, not gdaddon's —
use the Git actions, or a terminal.

### The rest of the flags

- `--asset <substring>` — pick the release asset by name. Only needed when a release
  ships several uploaded builds and gdaddon can't tell which one you want; without it,
  an ambiguous release is an error listing the candidates.
- `--name <name>` — record the entry under a name other than the one derived from the
  repo. Needed when that name is already taken by a different repo.
- `--no-deps` — skip declared dependencies, in either form.
- `--root <dir>` — the project root, when it isn't the git toplevel.

## list

`gdaddon list` prints each manifest entry's state, local version and pin. Everything it
reads is local and instant.

- `--json` emits a JSON array with stable snake_case keys, for tools that drive gdaddon
  and render their own UI (the companion Godot EditorPlugin does this).
- `--updates` additionally checks each addon for a newer release. This is the only part
  that touches the network, and it works with or without `--json` — the plain table
  grows an `update=` column.

## update-addons

Resolves an update plan for every installed addon with a newer release and installs
them. A locked entry is never updated, and an addon whose release ships several packages
is skipped with a note — there's no way to choose between them without a user, so those
stay a TUI job.
