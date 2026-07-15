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
  (`--json` modifier emits a JSON array for tools to parse; `--check-updates` adds the
  network-bound update state to that JSON — see Running)
- `--update` → updates installed addons to their latest release non-interactively, then exits

(`--install`/`--list`/`--update` are mutually exclusive; `--json`/`--check-updates`
are modifiers on `--list`, not modes.)

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
gdaddon --list --json  # same, as a JSON array for machine consumption (e.g. a Godot plugin UI)
gdaddon --list --json --check-updates  # JSON + per-addon update state (network)
gdaddon --update       # non-interactive update of installed addons to their latest release
gdaddon self-update             # update gdaddon itself to the latest release (see below)
gdaddon self-update --check --json  # report {current,latest_tag,available} for the Godot plugin
```

`--list --json` (`printListJSON` in `cmd/root.go`) emits one `listEntryJSON` object per
manifest entry with stable snake_case keys: `name`, `state`
(`missing`/`installed`/`mismatch`/`unversioned`/`branch_changed`/`invalid`), `kind`
(`package`/`clone`/`submodule`), `path`, `full_path`, `local_version`, `pinned_version`,
`tag`, `commit` (a branch package's pinned HEAD sha; `""` otherwise), `live_branch`
(a git checkout's current branch; `""` for non-git entries), `url`,
`lock` (bool; version-pinned, no update alerts),
`update` (`unknown`/`current`/`available`), `latest_tag`,
`ahead`/`behind` (a git checkout's divergence from its upstream; `0` otherwise),
`missing_deps`
(array of `{repo_id, tag, url}`). Always valid JSON (`[]` when empty). Everything is
local/instant except `update`/`latest_tag`, which stay `"unknown"` unless
`--check-updates` is passed (then each entry calls the network-bound `addon.CheckUpdate`).
`ahead`/`behind` are local too — they read the remote-tracking refs git already has — so
they're only as current as that checkout's last `git fetch`; a `--list` never fetches.
`missing_deps` comes from `addon.MissingDeps` (local; deps absent from the manifest,
excluding any the declaring entry's `suppress_deps` ignores).

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
  commit: "abc1234…"                   # optional; a branch package pinned to this HEAD sha (url is that commit's archive)
  suppress_deps: ["owner/repo"]        # optional; declared deps (canonical repo-ids) to ignore in the warning / "Add all"
```

A git **branch** install offers two modes (TUI confirm): **Clone** (default —
`git clone`, keeps `.git`, records `kind: clone` + `tag: <branch>`) or **Package**
(the second option). Package resolves the branch's current HEAD to a commit sha,
downloads that commit's archive (`.../archive/<sha>.zip`), and records `commit: <sha>`
so the snapshot is reproducible — a "clone without git's utility". A commit-pinned
entry reads as `installed` (the `.git`-less snapshot can't be re-verified, so the
recorded pin is trusted) and `CheckUpdate` returns `unknown` (a frozen snapshot has no
semver latest to compare). The pin comes from per-host `commit_archive_url` +
`branches.commit_path` in config/sources.yml (github/codeberg defaults); a config lacking those
(a host without the rules, or a sources.yml predating them) degrades to the old floating
branch-HEAD archive with no commit recorded — regen sources.yml to pick up new defaults.

An installed addon may declare its own dependencies in its `plugin.cfg`
(`deps=["owner/repo@v1.0.0", "owner/repo"]` — host defaults to github.com,
tag optional). The per-addon **Dependencies** TUI action (shown whenever the installed
plugin declares any deps) opens a screen listing every declared dep with its
*install-aware* status — `[installed]`/`[not installed]`/`[missing]`/`[outdated]`/
`[suppressed]`. From it: **Add all missing** adds the manifest-absent (non-suppressed)
deps (tag-pinned, or repo-only when tagless) without installing — `Install All` then
installs them — a per-dep submenu adds just one, and `s` (or the submenu) **suppresses**
a dep. Suppression persists as an inline `suppress_deps: ["owner/repo"]` list (canonical
repo-ids) on the *declaring* plugin's manifest entry, written by `addon.SetSuppressDeps`
(same single-line writer as `lock`/`commit`); a suppressed dep never contributes to the
warning nor is added by "Add all". `version` (the author-controlled plugin.cfg version)
can diverge from `tag` (the release identity), so dependency matching uses `tag` with
semver `>=`.

The "missing deps" row marker now stays until a declared (non-suppressed) dep is
actually *installed*, not merely present in the manifest: `addon.MissingDeps` is the
manifest-presence subset (what "Add all" adds), while `addon.DepStatuses` (backed by the
inspected project) is the install-aware form that drives the warning and the
Dependencies screen (cached as `appctx.Ctx.DepStatuses`).

An addon may also declare an installer-specific `dir="addons/x"` key in its
`plugin.cfg`/`version.cfg` (project-root-relative). The manifest stays the source of
truth: an explicit manifest `path` always wins, but when `path` is empty and the
install dir is being *derived*, a `dir=` key overrides the default `addons/<name>`
derivation (see `installDir` in cfg.go, applied by `resolveInstall` in resolve.go).
The derived path is then recorded back into the manifest on install.

Derivation preserves the package's own directory levels: a plugin folder's path
*relative to the addons anchor* is its path under the project's `addons/`, so
`addon_lib/my_addon/version.cfg` installs to `addons/addon_lib/my_addon`, namespace
folder and all. The anchor is the package's `addons/` folder when it ships one, else
the package root (a package with no `addons/` folder is assumed to *be* one). A zip's
single top-level folder is stripped only when it's a wrapper rather than a level of
that layout — a host-generated `repo-tag/` (by url), the addon folder itself, an asset
pack's root, or a release asset's version-stamped `MyPlugin-1.2.3/` (see `isWrapperDir`
in fetch.go); an unrecognized level is assumed to be the author's and kept.

## Architecture

```
main.go              — calls cmd.Execute()
cmd/
  root.go            — the root `gdaddon` cobra command: TUI by default, --install for non-interactive
  repos.go           — the `repos` subcommand: run a shell command in every nested git repo (uses addon.FindGitRepos / addon.HasUncommittedChanges)
  paths.go           — resolveRoot (project-root arg / git-root detection; manifest is discovered by the TUI context scan, see appctx.Ctx.Scan)
internal/
  addon/             — manifest parsing, install state (Inspect), Install/InstallAll, addon-config version read, manifest Update/AddEntry, plugin.cfg dependency parsing + semver matching (deps.go), git checkouts (read-only in gitscan.go: FindGitRepos, HasUncommittedChanges, GitSyncStatus, GitChanges, GitFetch/FetchAll; mutating/streaming in gitops.go: GitStream + GitStatus/GitPull/GitPush/GitCommit), ~/.gdaddon global list
  source/            — config-driven version resolution from a URL (resolver.go/parse.go): per-host VCS rules from config/sources.yml (releases, branches, source archives; RepoID), github.com/codeberg.org as defaults, git-clone fallback for ruleless hosts
  archive/           — local package archive (~/.gdaddon/archive or config/config.yml archive_dir): store/list package zips (List per repo, Repos for all), remove (RemoveRepo / Remove by path), merge into a listing
  config/            — ~/.gdaddon/config/ split into config.yml (archive_dir, theme, last search source — Load) and sources.yml (search sources + per-host VCS rules — LoadSources); `Ensure` dumps both defaults on first run, each file the source of truth once present
  restrule/          — generic config-driven REST query engine used by `source` to talk to host APIs
  gitcred/           — git credential/token resolution for clones
  search/            — addon search (Godot Asset Store + configured sources); backs the Search tab
  store/             — Asset Store URL detection/backend used by search/install
  installer/         — `gdaddon install`/`uninstall`: copy the running binary to system/user/home, PATH/elevation (InstallFrom for an explicit source, CurrentDest for the running binary's location)
  selfupdate/        — `gdaddon self-update`: check gdaddon's own repo for a newer release (source.AvailableVersions + addon.SemverGE) and download+install it via installer
  quarantine/        — Actions ▸ Dequarantine Addons: `Clear` walks <root>/addons removing com.apple.quarantine (x/sys/unix.Lremovexattr) and returns counts. Hidden dirs are pruned — an addon's .git is thousands of mode-0444 objects that can't own the attribute and only answer EACCES. darwin-only; `quarantine_other.go` is the non-macOS stub
  tui/               — bubbletea front-end (see internal/tui/doc.go)
    tui.go           — thin wiring: Run builds Shared chrome + the tab set, hands them to the router
    appctx/          — the domain↔framework seam: gdaddon's Ctx (ManifestPath/ProjectRoot) on Shared.App, the Header renderer, and the Project/Global/Archive refresh targets
    tabs/<domain>/   — one package per top-level tab (project, global, archive, actions, search): its root screen, flow screens, and the builders that wire components to features
    flows/<name>/    — domain-aware flow screens shared by >1 tab (e.g. newplugin, docs)
bubblestack/         — the reusable TUI framework, its OWN module (github.com/brohd11/bubblestack, replace => ./bubblestack); imports no gdaddon package
  core/              — Shared state (consumer context behind App any, recovered via App[T]; optional Chrome = header closure + status line + pluggable Output pane, each toggleable and gateable per-screen via ChromeMasker/FullscreenMask; plus a router-drawn breadcrumb bar under the tab strip, built each frame from the live stack via the optional Crumber interface — CrumbLabel(short bool) — and RenderBreadcrumb), Router over a screen stack, nav commands that return a core.Action (Push/Pop/Replace/ResetToRoot/ShowTab, plus Seq to group several), Screen (Update returns (Screen, core.Action): Action bundles a control Msg the router applies synchronously and an async Cmd; Async wraps a cmd-only Action, the zero Action is a no-op) + optional interfaces (incl. Overlayer — a popup drawn over the screen below it; Composite/PopupBox in overlay.go do the ANSI-aware compositing), router messages (PropagateAll broadcast with opaque payload to every Receiver, streaming TaskEvent with opaque Payload), list/help/style helpers
  components/        — reusable, context-agnostic pieces configured by closures (Item self-dispatching list row; PickerScreen, DialogScreen (a confirm box, or a modal overlay when its Overlay flag is set — composited by core.PopupBox), LoadingScreen, TaskScreen, FormScreen, DocScreen (a scrollable read-only text page: a viewport under an optional title bar, its body supplied by a `Render(width) string` closure re-run on resize, so the caller owns formatting and DocScreen owns only scrolling); LogPane = default core.Output, with a wrap render mode (`w`, via the optional core.Wrapper capability) that folds long lines the viewport would otherwise clip at the pane edge) — they name no domain type. TaskScreen (streaming work) and LoadingScreen (fetch spinner) each own a context.WithCancel and let esc abort the in-flight work — their work closures (TaskScreen's RunFunc, LoadingScreen's Run) take that ctx as their first arg, so a cancellable closure threads it into its network/process call
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

### Docs & onboarding

The manual ships inside the binary: `internal/tui/flows/docs/` embeds `pages/*.md`
(`//go:embed`) and renders them in a `components.DocScreen`. **Adding a page is dropping
a numbered `.md` into `pages/`** — no code change. The filename orders it (`embed.FS`
reads sorted), its first `# ` heading is the title (breadcrumb + index row), and the
first line under that heading is the index description. `render.go` is a deliberately
partial markdown reader (headings / bullets / fenced code / inline code / re-flowed
paragraphs) styled from the live theme — no renderer dependency, so pages repaint on a
theme switch like everything else.

It's a `flows/` package because two layers reach it: **Actions ▸ Docs** (`docs.Index()`)
and `tui.Run`, which shows the first-run welcome popup. "First run" is *`~/.gdaddon` did
not exist*, sampled by `isFirstRun` in `cmd/root.go` **before** `config.Ensure` creates it
(`Ensure`'s created-paths return would also fire for someone who merely deleted one config
file) and passed to `tui.Run(projectRoot, version, firstRun)`. The popup rides
`bubblestack.Config.Init` next to `appctx.SelfUpdateCheckCmd` — `docs.WelcomeCmd` returns
a `core.Push` as a message, which the router applies. Enter `Replace`s the popup with the
docs index (so esc from the index lands on the tab root, not back on the popup); esc
dismisses. Non-interactive runs (`--install`/`--list`/`--update`) return before `tui.Run`,
so their output is untouched.

Key packages/functions:
- `addon.Inspect(manifest, root)` — parses the manifest and computes each entry's local state (missing/installed/mismatch/…). url-only entries (no path yet) read as missing. For git checkouts (clone/submodule) it reads the live checked-out branch (`gitCheckedOutBranch`, exposed on `Status.LiveBranch`) and reports `StateBranchChanged` when it differs from the recorded `tag` — branch drift, reconciled by re-recording the tag via the per-addon **Update branch record** action. A present git checkout is never touched by `Install All` (it skips clones/submodules whether unversioned or drifted).
- `addon.Install` / `addon.InstallAll` — fetch (`.zip` download / `.git` clone / **local `.zip` path** for archived packages), derive the install dir from the package's `plugin.cfg`/`version.cfg` (`internal/addon/cfg.go`), and report progress via a callback. `Install` returns the resolved path+version. Both take a leading `ctx context.Context` (as do `UpdateAll`/`InstallAllDeps`) — cancelling it aborts the in-flight download/clone (HTTP request + `git clone` are context-bound), which is how the TUI's task-abort works.
- `addon.GitSyncStatus` / `addon.GitFetch` / `addon.FetchAll` (gitscan.go) — a git checkout's
  divergence from its upstream (`GitSync{Ahead, Behind, Tracking}`), a ctx-bound `git fetch`,
  and the fan-out that fetches every present clone/submodule in an inspected manifest. The
  split matters: **`GitSyncStatus` is a local read** of the remote-tracking refs git already
  has (a `git rev-list`, same cost class as `HasUncommittedChanges`), so it rides the
  synchronous `appctx.Ctx.refreshGitChecks` pass next to `GitDirty` and populates `Ctx.GitSync`
  on every refresh — but it can only *see* new upstream commits after a fetch. **`GitFetch` is
  the only network call**, so it's bound to an explicit key (Project tab `f` →
  `tabs/project/fetch.go`, an async cmd broadcasting `fetchDone`, mirroring `checkUpdatesCmd`)
  and never runs on its own. Both `GitFetch` and the clone paths set `GIT_TERMINAL_PROMPT=0`
  so a repo with uncached credentials fails fast instead of hanging the TUI on an invisible
  auth prompt. The counts surface as the `behind origin N` / `ahead N` row markers alongside
  `uncommitted changes`; behind-ness is a *flag*, not an `addon.State` (a checkout can be
  behind, dirty, and branch-drifted at once).
- `addon.GitStream` / `GitStatus` / `GitPull` / `GitPush` / `GitCommit` (gitops.go) — the git
  operations behind the per-addon **Git** submenu (`tabs/project/git.go`: status, fetch, pull,
  push, commit — a `components.NewStayTask` per op, streaming git's output to the log via
  `Reporter`, then broadcasting `gitRefresh` so the row markers settle). `GitStream` is the
  primitive: it runs git with `gitEnv()` (`GIT_TERMINAL_PROMPT=0` + `GIT_EDITOR=true`, so git
  can never sit waiting for input a TUI can't give) and relays stdout+stderr through a
  `lineWriter` that breaks on `\r` as well as `\n` (git's progress output is CR-delimited).
  **This is not a git client**: `GitPull` is `--ff-only`, so a diverged branch aborts having
  changed nothing rather than leaving a conflicted tree behind the TUI, and every failure
  routes the user to a real terminal. **`GitCommit`'s `stageAll` is load-bearing**: `commit -a`
  stages only *tracked* files, so a newly created file is silently left out — the commit form
  makes the user pick (`-a` vs a leading `add -A`), and the confirm (`commitBody`) lists what
  will be committed *and* names the untracked files that won't. `addon.GitChanges` (gitscan.go,
  read-only) parses `status --porcelain` for that screen. The **project-wide** version lives in
  `internal/tui/flows/git/` (a flow, since both the Project tab and Actions open it):
  `git.AllRepos` is the fetch/pull/push-all menu (Actions ▸ Git, and `V` on the Project list),
  with a scope switch cycling clones → submodules → all (clones default — pulling a submodule
  dirties the parent repo's recorded pointer). It confirms with the repo list (`confirmBody`,
  capped like `commitBody`) then runs each repo **sequentially** under its own header — reading
  git's output is the point, so concurrent interleaving is avoided (the concurrent no-confirm
  path is the `f` key / `addon.FetchAll`). `git.Task` is the single-repo streaming stay-task
  shared by that flow and the per-addon submenu. Both raise `appctx.GitRefresh` on success so
  the Project list's local markers settle without re-firing the network update check. Keys: `v`
  (row-level, an addon's own Git page) and `V` (the all-repos page) — deliberately not `g`/`G`,
  which bubbles binds to jump-to-top/bottom on every list.
- `addon.UpdateEntry` / `addon.AddEntry` — rewrite a manifest entry's url/path/version in place (empty url/path leaves that line untouched) / append a new entry (deduped by `source.RepoID`). `addon.SetKind` / `addon.SetLock` / `addon.SetCommit` write single scalar lines the same way (empty value removes the line) — `SetCommit` records/clears a branch package's pinned HEAD sha.
- `source.AvailableVersions` / `source.Branches` / `source.RepoID` — configured-host releases (uploaded `.zip`s + a generated source archive), branch archives, and canonical repo identity, driven by per-host VCS rules from config/sources.yml (github.com/codeberg.org as defaults). `Branches` pins each branch to its HEAD commit (`Asset.Commit` + a `commit_archive_url`) when the host rule supplies `branches.commit_path` + `commit_archive_url`, else falls back to the floating branch-HEAD archive.
- `archive.Archive` / `archive.List` / `archive.Repos` / `archive.Merge` — save a downloaded asset zip (ctx-first, so the archive task's abort cancels the download), read one repo's archived packages back as "(archived)" releases (local-file URLs), enumerate every archived repo (the Archive tab), and fold them into a `source.Listing` (with archive-only fallback when the upstream fetch fails). A commit-pinned branch package is stored under `<branch>@<sha>` (so distinct commits of the same branch don't overwrite), and `parseArchiveTag` recovers the branch + `Asset.Commit` pin when the archive is listed back.
- `archive.RemoveRepo` / `archive.Remove` — delete a repo's whole archive (used by Global → Remove "+ archive"), or one archived package by its local path, pruning emptied folders (the Archive tab).

## Installing the binary

**Dev:** `install_unix.sh` symlinks the built binary into `~/.local/bin/gdaddon`
(target tracks `build/`, so it follows rebuilds). Update `GO_DIR` inside the script if
your repo path differs.

**Release (general users):** the install logic lives in the binary itself —
`gdaddon install` (`cmd/install.go` + `internal/installer/`). It copies the running
binary (`os.Executable()`) to a destination chosen via a small bubbletea menu (reusing
the bubblestack stack), or non-interactively with `--dest system|user|home`. The three
destinations: **system** (`/usr/local/bin` / `%ProgramFiles%\gdaddon`, on PATH, needs
sudo/admin), **user** (`~/.local/bin` / `%LOCALAPPDATA%\Programs\gdaddon`, sets up PATH —
registry on Windows, profile guidance on unix), or **gdaddon home** (`~/.gdaddon/bin`, no
PATH change — the permission-free target a Godot plugin launches via
`~/.gdaddon/bin/gdaddon` (`+.exe` on Windows) after probing PATH for a global `gdaddon`).
The TUI only *selects*; the copy/sudo/PATH work runs after it exits (bubbletea owns the
terminal). Per-OS bits are build-tagged in `internal/installer/path_unix.go` /
`path_windows.go`.

`gdaddon uninstall` (`installer.Uninstall`) removes the binary from all three locations
wherever present — binaries only, leaving PATH entries and other `~/.gdaddon` files alone
(sudo for the system copy; the running binary is skipped).

### Self-update

`gdaddon self-update` (`cmd/selfupdate.go` + `internal/selfupdate/`) checks gdaddon's own
repo (`selfupdate.RepoURL` = github.com/brohd11/gdaddon) for a newer release than the
running binary's injected `version`, reusing `source.AvailableVersions` for the listing and
`addon.SemverGE` for the tag comparison — the same machinery as the per-addon update check.
When a newer release exists it downloads the platform release zip (`<os>-<arch>` token, e.g.
`darwin-arm64`, matching `make package`'s asset names), extracts the binary, and installs it
via `installer.InstallFrom(dest, …)` to `installer.DefaultDest()` — the managed location the
running binary occupies (`installer.CurrentDest`), or `~/.gdaddon/bin` otherwise.
`--check [--json]` only reports (`{current,latest_tag,available}` for the Godot plugin to
parse, like `--list --json`); `--interactive` opens the same dest picker as `install`. The
check also runs automatically on TUI startup (wired as `bubblestack.Config.Init` →
`appctx.SelfUpdateCheckCmd`, a generic app-level startup hook in `bubblestack/core`'s Router)
and writes an "update available" line to the status/log; Actions ▸ Update gdaddon runs the
loading → confirm → task flow in-TUI (`internal/tui/tabs/actions/selfupdate.go`). A
self-update doesn't change the already-running process — relaunch to use the new binary.

`make package` zips each platform build into `dist/` (`zip -j`, binary only). Caveat:
macOS Gatekeeper warns on the unsigned binary first run (right-click → Open, or clear
quarantine with `xattr -dr com.apple.quarantine`).

On startup `runRoot` also writes `~/.gdaddon/.gitignore` (ignoring `bin/`) if absent —
`~/.gdaddon` is meant to be committable, but the OS binary isn't by default
(`config.EnsureGitignore`). An existing `.gitignore` is left alone, so a user can opt to
commit the binary.
