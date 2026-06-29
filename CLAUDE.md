# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`gdaddon` — a Go CLI (built with [Cobra](https://github.com/spf13/cobra)) for
browsing and installing Godot addons from a YAML manifest (`.zip` or `.git`, with
version pinning via `plugin.cfg`).

The root command is the addon installer itself (TUI by default, flags for
non-interactive runs):
- no args → launches the interactive TUI (browse, pick versions/branches/assets, install/update)
- `--install` → runs the non-interactive install (inspect manifest + install/update all), then exits
- `--list` → prints the manifest's install status without changing anything, then exits
- `--update` → updates installed addons to their latest release non-interactively, then exits

(`--install`/`--list`/`--update` are mutually exclusive.)

There is one subcommand, `repos`, a standalone CLI utility (no TUI) for running a
shell command across every git checkout nested under a directory — handy for the
submodule/addon repos a Godot project accumulates (see `cmd/repos.go`).


## Build commands

```bash
# Build for current platform only
go build -o build/mac-arm64/gdaddon .

# Cross-compile all targets (mac-arm64, mac-x86_64, linux, windows)
make

# Clean build artifacts
make clean
```

Build outputs go to `build/<platform>/gdaddon[.exe]`.

## Running

```bash
gdaddon                # TUI; git root auto-detected, manifest found by walking from it
gdaddon /godot/proj    # TUI with an explicit project root (manifest still discovered by the scan)
gdaddon --install      # non-interactive install of everything in the discovered manifest
gdaddon --list         # print the manifest's install status (state/local/pinned), then exit
gdaddon --update       # non-interactive update of installed addons to their latest release
```

### `repos` subcommand

Run a shell command in every git repo nested under a directory (the top-level repo
is excluded; submodules — `.git` file form — are included):

```bash
gdaddon repos                          # list every nested repo (base-relative paths)
gdaddon repos --dirty                  # list only repos with uncommitted changes
gdaddon repos -- git status -s         # run in each repo; header + output only when non-empty
gdaddon repos --raw -- git fetch       # live-stream each repo's output under its header
gdaddon repos --dirty -- git pull      # restrict to repos with uncommitted changes
gdaddon repos -C /path --depth 3 -- pwd
gdaddon repos -- "git log --oneline | head -1"   # pipes work (quoted), run via sh -c
```

With no command it lists matching repo paths; with a command (after `--`, optional)
it runs it in each. Flags: `-C/--dir` (scan root, default cwd), `--raw` (stream vs
the default capture), `--dirty`, `--depth` (default 5).

If no manifest is found, the TUI still launches with an empty Project list; use
Actions → Create manifest to bootstrap one (it must land within the manifest-walk
depth of the project root so the scan rediscovers it).

## Addon manifest format

```yaml
my_addon:
  url: https://example.com/addon.zip   # or https://github.com/user/repo.git
  path: addons/my_addon                # relative to Godot project root
  version: "1.2.3"                     # optional; skips install if already matches plugin.cfg
  tag: "v1.2.3"                        # optional; the release tag installed from (what dependency specs match)
```

An installed addon may declare its own dependencies in its `plugin.cfg`
(`deps=["owner/repo@v1.0.0", "owner/repo"]` — host defaults to github.com,
tag optional). The per-addon **Get dependencies** TUI action reads them and adds the
missing ones to the manifest (tag-pinned, or repo-only when tagless) without
installing; `Install All` then installs them. `version` (the author-controlled
plugin.cfg version) can diverge from `tag` (the release identity), so dependency
matching uses `tag` with semver `>=`.

An addon may also declare an installer-specific `dir="addons/x"` key in its
`plugin.cfg`/`version.cfg` (project-root-relative). The manifest stays the source of
truth: an explicit manifest `path` always wins, but when `path` is empty and the
install dir is being *derived*, a `dir=` key overrides the default `addons/<name>`
derivation (see `installDir` in cfg.go, applied by `resolveInstall` in resolve.go).
The derived path is then recorded back into the manifest on install.

## Architecture

```
main.go              — calls cmd.Execute()
cmd/
  root.go            — the root `gdaddon` cobra command: TUI by default, --install for non-interactive
  repos.go           — the `repos` subcommand: run a shell command in every nested git repo (uses addon.FindGitRepos / addon.HasUncommittedChanges)
  paths.go           — resolveRoot (project-root arg / git-root detection; manifest is discovered by the TUI context scan, see appctx.Ctx.Scan)
internal/
  addon/             — manifest parsing, install state (Inspect), Install/InstallAll, addon-config version read, manifest Update/AddEntry, plugin.cfg dependency parsing + semver matching (deps.go), nested-repo walk (FindGitRepos) + dirty check (HasUncommittedChanges), ~/.gdaddon global list
  source/            — config-driven version resolution from a URL (resolver.go/parse.go): per-host VCS rules from config.yml (releases, branches, source archives; RepoID), github.com/codeberg.org as defaults, git-clone fallback for ruleless hosts
  archive/           — local package archive (~/.gdaddon/archive or config.yml archive_dir): store/list package zips (List per repo, Repos for all), remove (RemoveRepo / Remove by path), merge into a listing
  config/            — ~/.gdaddon/config.yml (archive_dir, search sources, per-host VCS rules); `Ensure` dumps defaults on first run
  restrule/          — generic config-driven REST query engine used by `source` to talk to host APIs
  gitcred/           — git credential/token resolution for clones
  search/            — addon search (Godot Asset Store + configured sources); backs the Search tab
  store/             — Asset Store URL detection/backend used by search/install
  tui/               — bubbletea front-end (see internal/tui/doc.go)
    tui.go           — thin wiring: Run builds Shared chrome + the tab set, hands them to the router
    appctx/          — the domain↔framework seam: gdaddon's Ctx (ManifestPath/ProjectRoot) on Shared.App, the Header renderer, and the Project/Global/Archive refresh targets
    tabs/<domain>/   — one package per top-level tab (project, global, archive, actions, search): its root screen, flow screens, and the builders that wire components to features
    flows/<name>/    — domain-aware flow screens shared by >1 tab (e.g. newplugin)
bubblestack/         — the reusable TUI framework, its OWN module (github.com/brohd11/bubblestack, replace => ./bubblestack); imports no gdaddon package
  core/              — Shared state (consumer context behind App any, recovered via App[T]; optional Chrome = header closure + status line + pluggable Output pane, each toggleable and gateable per-screen via ChromeMasker/FullscreenMask; plus a router-drawn breadcrumb bar under the tab strip, built each frame from the live stack via the optional Crumber interface — CrumbLabel(short bool) — and RenderBreadcrumb), Router over a screen stack, nav commands that return a core.Action (Push/Pop/Replace/ResetToRoot/ShowTab, plus Seq to group several), Screen (Update returns (Screen, core.Action): Action bundles a control Msg the router applies synchronously and an async Cmd; Async wraps a cmd-only Action, the zero Action is a no-op) + optional interfaces (incl. Overlayer — a popup drawn over the screen below it; Composite/PopupBox in overlay.go do the ANSI-aware compositing), router messages (PropagateAll broadcast with opaque payload to every Receiver, streaming TaskEvent with opaque Payload), list/help/style helpers
  components/        — reusable, context-agnostic pieces configured by closures (Item self-dispatching list row; PickerScreen, DialogScreen (a confirm box, or a modal overlay when its Overlay flag is set — composited by core.PopupBox), LoadingScreen, TaskScreen, FormScreen; LogPane = default core.Output) — they name no domain type. TaskScreen (streaming work) and LoadingScreen (fetch spinner) each own a context.WithCancel and let esc abort the in-flight work — their work closures (TaskScreen's RunFunc, LoadingScreen's Run) take that ctx as their first arg, so a cancellable closure threads it into its network/process call
```

### TUI design goals

The TUI was restructured for scalability around three ideas:

- **Tabs are domains.** Each top-level tab is its own package under `tui/tabs/`
  (`project`, `global`, `archive`, `actions`, `search`) owning its root screen and flows.
  Adding a feature area means adding a tab package, not editing a monolith.
- **Domains share `components` to simplify logic.** Reusable screens
  (picker/confirm/loading/popup/streaming-task) live in `bubblestack/components`, are
  context-agnostic, and are configured by closures the tab supplies — so a tab
  composes flows from shared pieces instead of reimplementing list/confirm/task
  plumbing. The framework (`bubblestack/core` + `bubblestack/components`) is a
  standalone module that names no domain type, so it can be reused by another tool;
  gdaddon's domain state rides on `core.Shared.App` and is read back via
  `appctx.Of(sh)`.
- **List rows carry their own behavior (`components.Item`).** Every list is built
  from `components.Item` values, each holding its own `Pick func(*core.Shared)
  (tea.Msg, tea.Cmd)` closure (a control message the router applies synchronously
  and/or an async cmd). A `PickerScreen` runs the selected row's `Pick` on enter, so a
  menu of mixed commands needs no per-row `kind` enum, no `switch`, and — for a
  pushed screen — no `Update` method at all (building the rows *is* the flow). An
  inert row (placeholder / disabled) is just an `Item` with a nil `Pick`. Tab
  roots still own an `Update` (quit-on-`q`, notifications, output pane) but just
  forward `components.RootUpdate`'s `(tea.Msg, tea.Cmd)` pair. Domain values carried
  *through* a flow (e.g. `versionItem`, `globalItem`) stay plain payload structs
  captured inside the closures, not used as list rows.

Dependency direction is strictly `core ← components ← appctx ← flows/* ← tabs/* ←
tui`: `core` names no concrete screen (reaches them via optional interfaces like
`Receiver`/`PopStopper`), `core` and `components` name no domain type (closures
only; context behind `Shared.App`), `appctx` is the single leaf binding the domain
to the framework, and tabs never import each other. That acyclic layering — across
the `bubblestack` module boundary for `core`/`components` — is what lets the screens
live in separate packages. See `internal/tui/doc.go` for the full contract and how
to add a tab.

Key packages/functions:
- `addon.Inspect(manifest, root)` — parses the manifest and computes each entry's local state (missing/installed/mismatch/…). url-only entries (no path yet) read as missing.
- `addon.Install` / `addon.InstallAll` — fetch (`.zip` download / `.git` clone / **local `.zip` path** for archived packages), derive the install dir from the package's `plugin.cfg`/`version.cfg` (`internal/addon/cfg.go`), and report progress via a callback. `Install` returns the resolved path+version. Both take a leading `ctx context.Context` (as do `UpdateAll`/`InstallAllDeps`) — cancelling it aborts the in-flight download/clone (HTTP request + `git clone` are context-bound), which is how the TUI's task-abort works.
- `addon.UpdateEntry` / `addon.AddEntry` — rewrite a manifest entry's url/path/version in place (empty url/path leaves that line untouched) / append a new entry (deduped by `source.RepoID`).
- `source.AvailableVersions` / `source.Branches` / `source.RepoID` — configured-host releases (uploaded `.zip`s + a generated source archive), branch-HEAD archives, and canonical repo identity, driven by per-host VCS rules from config.yml (github.com/codeberg.org as defaults).
- `archive.Archive` / `archive.List` / `archive.Repos` / `archive.Merge` — save a downloaded asset zip (ctx-first, so the archive task's abort cancels the download), read one repo's archived packages back as "(archived)" releases (local-file URLs), enumerate every archived repo (the Archive tab), and fold them into a `source.Listing` (with archive-only fallback when the upstream fetch fails).
- `archive.RemoveRepo` / `archive.Remove` — delete a repo's whole archive (used by Global → Remove "+ archive"), or one archived package by its local path, pruning emptied folders (the Archive tab).

## Installing the binary

`install_unix.sh` symlinks the built binary into `~/.local/bin/gdaddon`. Update
`GO_DIR` inside the script if your repo path differs.
