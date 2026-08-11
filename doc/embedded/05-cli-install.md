# Installing from the command line

Add one addon to a project without opening this TUI.

```
gdaddon install owner/repo
```

That downloads the addon's latest release, records it in the project's manifest, and
installs the dependencies it declares — and theirs, and so on. Only that closure is
touched: an unrelated entry sitting uninstalled in your manifest stays uninstalled.

It's the targeted counterpart to Actions ▸ Install All + Deps, which applies the whole
manifest.

## Naming the addon

The repo is `owner/repo`, or `host/owner/repo` for a host other than github.com — the
same shorthand a `plugin.cfg` `deps` entry uses. An optional `@tag` picks a release;
without one you get the latest non-prerelease. A leading `v` is optional on either side,
so `@1.2.0` finds a release tagged `v1.2.0`.

```
gdaddon install brohd11/Godot-Script-Tabs
gdaddon install brohd11/Godot-Script-Tabs@v0.1.4
gdaddon install codeberg.org/someone/their-addon
```

## Where things go

The project root is the git toplevel, or whatever `--root` says. The manifest is found
by walking down from there, and an empty one is created at the root if there isn't one —
so this works in a brand-new project.

The install location is worked out from the downloaded package, exactly as it is in the
TUI: a repo whose root holds a `plugin.cfg` is installed whole, a repo shipping an
`addons/` folder has its plugin folders mirrored under the project's `addons/`, and
anything else is located by its config files. An addon can override this by declaring
`dir="addons/whatever"` in its own `plugin.cfg` — the opt-in that makes a
submodule-shaped repo land somewhere other than `addons/<repo-name>`.

## Branches

A release is the default. To track a branch instead, install it as a live git clone:

```
gdaddon install owner/repo --clone         # the remote's default branch
gdaddon install owner/repo@dev --clone     # the dev branch
```

The checked-out branch is recorded as the entry's `tag`, so it reads as a clone rather
than as branch drift on the next refresh. Updating a clone is git's job, not gdaddon's —
use the Git actions, or a terminal.

## The rest of the flags

- `--asset <substring>` — pick the release asset by name. Only needed when a release
  ships several uploaded builds and gdaddon can't tell which one you want; without it,
  an ambiguous release is an error listing the candidates.
- `--name <name>` — record the entry under a name other than the one derived from the
  repo. Needed when that name is already taken by a different repo.
- `--no-deps` — install the addon alone and leave its declared dependencies to you.
- `--root <dir>` — the project root, when it isn't the git toplevel.

## Not to be confused with

`gdaddon self-install` installs the gdaddon *binary* — it has nothing to do with Godot
addons. Likewise `gdaddon self-update`.
