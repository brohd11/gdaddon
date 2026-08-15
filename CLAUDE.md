# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`gdaddon` — a Go CLI (built with [Cobra](https://github.com/spf13/cobra)) for
browsing and installing Godot addons from a YAML manifest (`.zip` or `.git`, with
version pinning via `plugin.cfg`).

The root command with no arguments launches the interactive TUI (browse, pick
versions/branches/assets, install/update). Everything non-interactive is a subcommand —
**one verb per job, no mode flags on root**:

| Command | File | Does |
|---|---|---|
| `install --all [--no-deps]` | `cmd/addoninstall.go` | install every manifest entry + declared deps |
| `install <owner/repo>[@tag]` | `cmd/addoninstall.go` | install one addon + *its* dep closure |
| `list [root] [--json] [--updates]` | `cmd/list.go` | print install status; JSON for tools |
| `update-addons [root]` | `cmd/updateaddons.go` | update installed addons to their latest release |
| `update [--check]` | `cmd/update.go` | update the **gdaddon binary** (goutil's shared command) |
| `repos [flags] -- <cmd>` | `cmd/repos.go` | run a shell command in every nested git repo |

The `update` / `update-addons` split is the one distinction worth knowing: `update` is
the binary (identical in every brohd11 app), `update-addons` is the Godot addons.

This surface replaced an earlier one where the modes were root flags (`--install`,
`--list --json --check-updates`, `--update-packages`) *and* `install` meant an addon
while `self-install`/`uninstall` meant the binary. **Do not reintroduce a root mode
flag.** Two consequences of that cut are load-bearing:

- **`gdaddon` no longer installs its own binary anywhere.** `self-install`/`uninstall`,
  `internal/installer` and `internal/selfupdate` are all gone; `install.sh` (with
  `BIN_DIR`) owns placement, exactly as in gote/gossh/golaunch/repoview. Self-update
  writes next to the running binary via goutil's `BinDir()`.
- **The Godot EditorPlugin's contract changed** and needs a matching release: it used to
  call `gdaddon --list --json [--check-updates]` and parse `self-update --check --json`.
  Those are now `gdaddon list --json --updates` (same 18-key shape) and `gdaddon update
  --check` (three human lines — the shared command has no `--json`).


## Build commands

```bash
# Build for the host platform only (default target)
make

# Run the test suite (gdaddon module only; the sibling repos have their own CI)
make test

# Cross-compile all release targets (darwin/arm64, darwin/amd64, linux/amd64,
# linux/arm64, windows/amd64)
make all

# Clean build artifacts
make clean
```

Build outputs go to `build/<os>-<arch>/gdaddon[.exe]` (e.g. `build/darwin-arm64/gdaddon`).

`bubblestack`, `gitstack`, and `repoview` are separate repos (see Architecture). For local
cross-repo work they sit beside this one under `~/main/go/`, tied by the `go.work` there
(`use ./bubblestack ./gdaddon ./gitstack ./golaunch ./goutil ./repoview ./gossh` — it lives
in the workspace repo, not in this one). Ordinary `make`/`go build` here builds against the
tagged module versions.

## Running

```bash
gdaddon                # TUI; git root auto-detected, manifest found by walking from it
gdaddon /godot/proj    # TUI with an explicit project root (manifest still discovered by the scan)
gdaddon install --all         # non-interactive install of the discovered manifest + declared deps
gdaddon install --all --no-deps  # the manifest's own entries only (what --install used to do)
gdaddon install owner/repo    # install ONE addon (+ the deps it declares) into this project
gdaddon list                  # print the manifest's install status (state/local/pinned), then exit
gdaddon list --json           # same, as a JSON array for machine consumption (e.g. a Godot plugin UI)
gdaddon list --json --updates # JSON + per-addon update state (network)
gdaddon list --updates        # the plain table + an update= column (same network check)
gdaddon update-addons         # non-interactive update of installed addons to their latest release
gdaddon update                # update the gdaddon BINARY to the latest release (see below)
gdaddon update --check        # report without installing
```

Every project-scoped subcommand takes the project root as an optional positional
argument, matching the root command; `install` takes it as `--root` because its
positional slot holds the repo spec (`resolveRootArg` / `resolveRootQuiet` in
`cmd/paths.go` — never the prompting `resolveRoot`, which is the TUI path's).

`list --json` (`printListJSON` in `cmd/list.go`) emits one `listEntryJSON` object per
manifest entry with stable snake_case keys: `name`, `state`
(`missing`/`installed`/`mismatch`/`unversioned`/`branch_changed`/`invalid`), `kind`
(`package`/`clone`/`submodule`), `path`, `full_path`, `local_version`, `pinned_version`,
`tag`, `commit` (a branch package's pinned HEAD sha; `""` otherwise), `live_branch`
(a git checkout's current branch; `""` for non-git entries), `url`,
`lock` (bool; version-pinned, no update alerts),
`is_dependency` (bool; entry auto-added because another plugin declares it as a dep),
`orphan` (bool; an `is_dependency` entry nothing installed still requires),
`update` (`unknown`/`current`/`available`/`locked`), `latest_tag`,
`ahead`/`behind` (a git checkout's divergence from its upstream; `0` otherwise),
`missing_deps`
(array of `{repo_id, tag, url}`). Always valid JSON (`[]` when empty). Everything is
local/instant except `update`/`latest_tag`, which stay `"unknown"`/`""` unless
`--updates` is passed (then each entry calls the network-bound `addon.CheckUpdate`;
a locked entry reports `update: "locked"` with or without the check). `--updates` is not
a `--json` modifier — it works in both output modes, and `printListTable` grows an
`update=` column from the same `resolveUpdateStates` call, so the flag is never silently
ignored. `ahead`/`behind` are local too — they read the remote-tracking refs git already
has — so they're only as current as that checkout's last `git fetch`; a `list` never fetches.
`missing_deps` comes from `addon.MissingDeps` (local; deps absent from the manifest,
excluding any the declaring entry's `suppress_deps` ignores). `is_dependency`/`orphan` are
local too: `orphan` comes from `addon.OrphanDeps` (the union of every non-suppressed dep
declared by a *present* plugin — an `is_dependency` entry outside that union is orphaned).

### `gdaddon install` — both forms

`--all` is the whole manifest (`addon.InstallAllDeps`, or `InstallAll` with `--no-deps`);
a repo spec is the targeted form: record and install **one** addon plus the
dependency closure *it* declares, touching nothing else in the manifest. `checkInstallArgs`
rejects the combinations cobra can't express — naming neither target, naming both, or
pairing `--asset`/`--name`/`--clone` (single-addon options) with `--all`. Note the two
forms differ on a missing manifest: the targeted form creates one (`findOrCreateManifest`),
`--all` errors via `discoverManifest`, since bootstrapping an empty manifest to install
nothing out of it is a confusing no-op.

The targeted form's `cmd/addoninstall.go` is thin wiring; the reusable flow is `addon.InstallOne` /
`addon.InstallDepsFor` in `internal/addon/install_one.go` (no cobra, no bubbletea — a
per-addon TUI action can call them unchanged).

The repo spec is parsed by `addon.ParseRepoSpec`, which is `parseDependency` exported:
the CLI argument and a `plugin.cfg` `deps` item are deliberately the same syntax
(`owner/repo`, `host/owner/repo`, optional `@tag`) parsed by the same code, so they
can't drift. `@tag` matches a release via `TagEqual` (leading `v` tolerated either way);
no tag means `addon.LatestRelease`. Asset choice is `addon.SelectRelease` +
`addon.SelectAsset` (`internal/addon/select.go`) — the latter wraps `source.AutoAsset`
and returns `*addon.AmbiguousAssetError` when a release ships 2+ uploaded assets, which
the CLI prints as a candidate list pointing at `--asset` (the TUI's answer to the same
condition is a picker).

Flags: `--root` (project root; default the git toplevel via the non-prompting
`resolveRootQuiet` in `cmd/paths.go`, *not* `resolveRoot`, which prompts and can
`os.Exit`), `--asset`, `--name`, `--clone`, `--no-deps`. The manifest is found by
`addon.FindManifest` and **created** at the root when absent (`findOrCreateManifest`) —
deliberately unlike `discoverManifest`, which errors on a miss for the read-only paths.

`--clone` installs a live git checkout instead of a release; the branch comes from
`@ref`, and a bare `--clone` takes the remote's default branch. `gitCloneBranch` omits
`--branch` entirely when none is named, and `InstallOne` reads the checked-out branch
back (`CurrentBranch`) and records it as the entry's `tag` — a clone whose tag doesn't
match its live branch reads as `branch_changed` forever otherwise. A clone records no
`version:` (it tracks a branch), matching the TUI's `pinInstall`.

**`InstallDepsFor` vs `InstallAllDeps`** — the distinction is the point.
`InstallAllDeps` is a manifest-wide fixed-point loop (inspect → `InstallAll` →
`importDeps` → repeat) that installs *everything*; right for "set up this project",
wrong for "install this one plugin". `InstallDepsFor` is a breadth-first walk outward
from a single installed addon, deduped by `source.RepoID` (so a diamond resolves once
and a cycle terminates) and depth-bounded by `maxDepRounds`. Per dep, `ensureDep` either
skips it (installed and satisfying, by the same rule as `MissingDeps`/`DepStatuses`:
tagless → presence suffices, uncomparable tags → trusted), adds it via `AddDepEntry`
(carrying `is_dependency: true`), or re-pins a verifiably-older entry via `UpsertEntry`.
It then looks the entry up **by name, not by repo id**, and marks both identities as seen.

**The upstream-rename fallback** is a rule the whole dependency system shares, and it now
lives in exactly one place: `depIndex` in `internal/addon/depmatch.go`, which matches a dep
by `source.RepoID` *and then* by `DeriveName(d.RepoURL)`. Every reader — `MissingDeps`,
`DepStatuses`, `ensureDep`, and `PlanDeps` (which backs the TUI's "Get deps") — looks a dep
up through it, so the rule applies by construction rather than by each site remembering to
write it. The reason it exists: a repo renamed upstream keeps serving release assets under
its **new** name, so the manifest records an id the declared `deps` spec no longer parses
to. Without the fallback such a dep reads as perpetually missing — the TUI nags forever,
`list --json` reports it in `missing_deps`, and "Add all" / `install --all` fail on it with
`already added from <new-id> (as "<name>")`. `brohd11/Godot-TreeSitter-Wrapper` →
`godot-tree-sitter-gd` is a live instance of this in the wild. This used to be three
hand-synced sites plus a fourth (the TUI's dep resolver) that was missing the fallback
entirely, which is exactly the failure above; `TestPlanDepsRenamedUpstream` pins it.

`addon.PlanDeps(a, projectRoot, manifest)` is the shared classification: each declared dep
is `Add` (no entry), `Satisfied`, or `Stale` (entry present but verifiably behind), with
suppressed deps omitted. It is local-only — resolving an addable dep to a release asset is
a network call and stays with the caller (`ResolveDepAsset`).

`addon.ErrNameTaken` is the sentinel `AddEntry` wraps on a key collision with a
*different* repo's entry, so the CLI can suggest `--name` without matching message text.

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
  is_dependency: true                  # optional; auto-added because another plugin declares it — provenance for the "unused dependency" marker
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

A dep added via the Dependencies flow is recorded with `is_dependency: true` (provenance —
written by `addon.SetIsDependency`, the same single-line writer as `lock`/`commit`, and
carried by `AddEntryFull` so it survives set import/export; export to global drops it, an
explicit promotion). `addon.OrphanDeps` (local, cached as `appctx.Ctx.OrphanDeps`, computed
in the same `loadProject` pass as `DepStatuses`) then flags any `is_dependency` entry no
longer required by any *installed* plugin — the **"unused dependency"** row marker. The
graph is read from installed plugins only, so uninstalling a depender flags its dep (a
stateless, self-healing recompute). It's a warning marker, not an `addon.State` — an entry
can be installed *and* orphaned at once, like the "missing deps"/"behind origin" markers.
The per-addon **Keep (not a dependency)** action clears the flag (`SetIsDependency` false),
adopting the entry so it stops flagging; removal is the existing per-addon **Remove**.

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
  root.go            — the root `gdaddon` cobra command (TUI only; every mode is a subcommand now), plus the shared discoverManifest and the PersistentPreRunE `bootstrap` that ensures ~/.gdaddon for EVERY command
  addoninstall.go    — the `install` subcommand, both forms: --all (whole manifest) and <owner/repo>[@tag] (one addon + its declared dep closure); thin wiring over addon.InstallAllDeps / addon.InstallOne
  list.go            — the `list` subcommand: the status table, the listEntryJSON wire shape, and printListJSON
  updateaddons.go    — the `update-addons` subcommand: ResolveUpdatePlans + UpdateAll over the manifest
  update.go          — the `update` subcommand: one line registering goutil's shared NewUpdateCommand. Updates the BINARY, not addons
  repos.go           — the `repos` subcommand: run a shell command in every nested git repo (uses addon.FindGitRepos / addon.HasUncommittedChanges)
  paths.go           — resolveRoot (project-root arg / git-root detection, may prompt on stdin — TUI path only), resolveRootQuiet (never prompts or exits — the subcommand path) and resolveRootArg (the optional positional [project_root] the subcommands share). The manifest is discovered by the TUI context scan (appctx.Ctx.Scan), discoverManifest, or findOrCreateManifest in addoninstall.go
internal/
  addon/             — manifest parsing, install state (Inspect), Install/InstallAll, addon-config version read, manifest Update/AddEntry, plugin.cfg dependency parsing + semver matching (deps.go), ~/.gdaddon global list. The git engine lives in the gitstack module (below); addon re-exports it via aliases in git_reexport.go (addon.GitFetch/GitSync/GitChanges/CurrentBranch/… = gitstack/repo.*) plus the manifest-aware FetchAll([]Status) adapter, so existing addon.* callers are unchanged. gitscan.go keeps only the manifest/scan probes (gitProbe, isGitCheckout, normalizeGitRemote) that classify a plugin folder's `.git`
  source/            — config-driven version resolution from a URL (resolver.go/parse.go): per-host VCS rules from config/sources.yml (releases, branches, source archives; RepoID), github.com/codeberg.org as defaults, git-clone fallback for ruleless hosts
  archive/           — local package archive (~/.gdaddon/archive or config/config.yml archive_dir): store/list package zips (List per repo, Repos for all), remove (RemoveRepo / Remove by path), merge into a listing
  config/            — ~/.gdaddon/config/ split into config.yml (archive_dir, last search source — Load; the theme moved out to the framework-wide ~/.bubblestack/config.yml) and sources.yml (search sources + per-host VCS rules — LoadSources); `Ensure` dumps both defaults on first run, each file the source of truth once present
  restrule/          — generic config-driven REST query engine used by `source` to talk to host APIs
  gitcred/           — git credential/token resolution backing restrule's authenticated HTTP GETs (host APIs, archive downloads); clones don't use it — they run with GIT_TERMINAL_PROMPT=0 against the user's own git credentials
  search/            — addon search (Godot Asset Store + configured sources); backs the Search tab
  store/             — Asset Store URL detection/backend used by search/install
  quarantine/        — Actions ▸ Dequarantine Addons: `Clear` walks <root>/addons removing com.apple.quarantine (x/sys/unix.Lremovexattr) and returns counts. Hidden dirs are pruned — an addon's .git is thousands of mode-0444 objects that can't own the attribute and only answer EACCES. darwin-only; `quarantine_other.go` is the non-macOS stub
  tui/               — bubbletea front-end (see internal/tui/doc.go)
    tui.go           — thin wiring: Run builds Shared chrome + the tab set, hands them to the router
    appctx/          — the domain↔framework seam: gdaddon's Ctx (ManifestPath/ProjectRoot) on Shared.App, the Header renderer, and the Project/Global/Archive refresh targets
    tabs/<domain>/   — one package per top-level tab (project, global, sets, archive, actions, search): its root screen, flow screens, and the builders that wire components to features
    flows/<name>/    — domain-aware flow screens shared by >1 tab (e.g. newplugin, docs)
    sysopen/         — thin domain adapter over `bubblestack/sysopen` (the shared OS-open helpers). `Path`/`Terminal` delegate straight through; `URL` first reduces a file-extension addon url to its repo host (`source.RepoURL`) so the browser lands on the repo, not the asset. The launcher logic (per-OS emulator table, the `cmd.Dir` fix so `t` no longer opens at gdaddon's own cwd, x-terminal-emulator demoted to last, command-in-terminal support) lives in bubblestack now, shared with repoview + go-ssh. NOTE: the old config.yml `terminal` override is not honored right now — it's slated to return via a future bubblestack-owned config (settable config path, supplying terminal override + theme)
The three modules below live in their OWN GitHub repos (github.com/brohd11/{bubblestack,gitstack,repoview}) — gdaddon `require`s bubblestack + gitstack as tagged versions, no `replace`. For local co-development all of them are checked out side by side under ~/main/go/ and tied by the workspace `go.work` there (`use ./bubblestack ./gdaddon ./gitstack ./golaunch ./goutil ./repoview ./gossh` — committed in the workspace repo, not in this one), so cross-module edits are picked up without re-tagging; releases/CI/outside consumers use the tags. None of the three imports a gdaddon package. Their layouts:

gitstack (github.com/brohd11/gitstack) — the reusable git module, extracted from addon so a second tool (repoview, below) can share it:
  repo/              — domain-neutral git engine (stdlib + goutil/stream for output streaming): FindGitRepos, Scan (folder → []Repo), Describe/CurrentBranch, HasUncommittedChanges, GitSyncStatus, GitChanges, GitFetch, FetchAll([]Repo), GitStream + GitStatus/GitPull/GitPush/GitCommit, diff reads (Diff/DiffStat/DiffStats) and tag ops (LocalTags/RemoteTags/NextTag/GitTag/GitPushTag/GitDeleteTag); types Repo{Name,Dir,Branch,Sync,Dirty,Root}, GitSync, GitChange, FetchResult, Reporter
  repoui/            — git-viewing screens over bubblestack (name no domain type): RepoMenu (per-repo status/fetch/pull/push/commit submenu), AllReposMenu(sh, []Scope, RootOption) (batch fetch/pull/push, consumer supplies the scopes; RootOption adds an include-root toggle), DiffScreen, TagsScreen, Task, RefreshMsg/FetchDoneMsg. gdaddon consumes these behind thin adapters (flows/git/git.go builds its clone/submodule/all scopes; tabs/project/git.go maps Status→repo.Repo); appctx.GitRefresh is an alias of repoui.RefreshMsg
repoview (github.com/brohd11/repoview) — a separate binary/repo, the manifest-free sibling of gdaddon: scan a plain directory for git checkouts (repo.Scan) and show each one's status (branch/ahead/behind/dirty) in one list screen, driving fetch/pull/push/commit through the shared gitstack/repoui screens. One repo-list tab (no tab strip) + an "a" Actions menu (the shared components.NewActionsMenu: theme, docs, self-update, refresh) with its manual embedded from doc/embedded/ (the standard bubblestack docs layout, same as gdaddon's); no manifest, fresh scan each run. gdaddon does NOT depend on it — it's a second consumer of bubblestack + gitstack, developed in its own repo (see its main.go + internal/app/).
bubblestack (github.com/brohd11/bubblestack) — the reusable TUI framework:
  core/              — Shared state (consumer context behind App any, recovered via App[T]; optional Chrome = header closure + status line + pluggable Output pane, each toggleable and gateable per-screen via ChromeMasker/FullscreenMask; plus a router-drawn breadcrumb bar under the tab strip, built each frame from the live stack via the optional Crumber interface — CrumbLabel(short bool) — and RenderBreadcrumb), Router over a screen stack, nav commands that return a core.Action (Push/Pop/Replace/ResetToRoot/ShowTab, plus Seq to group several), Screen (Update returns (Screen, core.Action): Action bundles a control Msg the router applies synchronously and an async Cmd; Async wraps a cmd-only Action, the zero Action is a no-op) + optional interfaces (incl. Overlayer — a popup drawn over the screen below it; Composite/PopupBox in overlay.go do the ANSI-aware compositing), router messages (PropagateAll broadcast with opaque payload to every Receiver, streaming TaskEvent with opaque Payload), list/help/style helpers. Mouse support: cell-motion tracking (toggled with `m` — off restores terminal text selection), with the router hit-testing its own chrome on a left click — a breadcrumb segment pops the stack to it, a tab activates+unwinds via ShowTab (spans computed arithmetically from the titles, so spaces hit-test fine), and a consumer-supplied Config.HeaderClick fires on the header box (gdaddon wires it to the root's git page, same as ctrl+v)
  components/        — reusable, context-agnostic pieces configured by closures (Item self-dispatching list row; PickerScreen, DialogScreen (a confirm box, or a modal overlay when its Overlay flag is set — composited by core.PopupBox), LoadingScreen, TaskScreen, FormScreen, DocScreen (a scrollable read-only text page: a viewport under an optional title bar, its body supplied by a `Render(width) string` closure re-run on resize, so the caller owns formatting and DocScreen owns only scrolling); LogPane = default core.Output, with a wrap render mode (`w`, via the optional core.Wrapper capability) that folds long lines the viewport would otherwise clip at the pane edge) — they name no domain type. TaskScreen (streaming work) and LoadingScreen (fetch spinner) each own a context.WithCancel and let esc abort the in-flight work — their work closures (TaskScreen's RunFunc, LoadingScreen's Run) take that ctx as their first arg, so a cancellable closure threads it into its network/process call. Also here: NewActionsMenu (the standard Actions sheet — theme, docs when the app ships pages via DocsItem, self-update, refresh), the in-TUI manual engine (ParseDocPages/DocsIndex/RenderMarkdown over embedded markdown), the shared self-update flow, plus the sysopen (OS terminal/file-manager launchers) and config (framework-wide ~/.bubblestack theme) packages
```

### TUI design goals

The TUI was restructured for scalability around three ideas:

- **Tabs are domains.** Each top-level tab is its own package under `tui/tabs/`
  (`project`, `global`, `sets`, `archive`, `actions`, `search`) owning its root screen and flows.
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

The manual ships inside the binary: the pages live at `doc/embedded/*.md` (the repo's
doc folder, so they're easy to find and edit), embedded by the tiny `gdaddon/doc`
package — `go:embed` can't reach a parent directory, so the embed must live at that
level. `internal/tui/flows/docs/` is only the TUI flow (index + first-run welcome),
rendering them in a `components.DocScreen`. **Adding a page is dropping a numbered
`.md` into `doc/embedded/`** — no code change. The filename orders it (`embed.FS`
reads sorted), its first `# ` heading is the title (breadcrumb + index row), and the
first line under that heading is the index description. The renderer (in
bubblestack/components) is a deliberately partial markdown reader (headings /
bullets / fenced code / inline code / re-flowed paragraphs) styled from the live
theme — no renderer dependency, so pages repaint on a theme switch like everything
else.

It's a `flows/` package because two layers reach it: **Actions ▸ Docs** (`docs.Index()`)
and `tui.Run`, which shows the first-run welcome popup. "First run" is *`~/.gdaddon` did
not exist*, sampled by `isFirstRun` in `cmd/root.go` **before** `config.Ensure` creates it
(`Ensure`'s created-paths return would also fire for someone who merely deleted one config
file) and passed to `tui.Run(projectRoot, version, firstRun)`. The popup rides
`bubblestack.Config.Init` next to `appctx.SelfUpdateCheckCmd` — `docs.WelcomeCmd` returns
a `core.Push` as a message, which the router applies. Enter `Replace`s the popup with the
docs index (so esc from the index lands on the tab root, not back on the popup); esc
dismisses. The subcommands never reach `tui.Run`,
so their output is untouched.

Key packages/functions:
- `addon.Inspect(manifest, root)` — parses the manifest and computes each entry's local state (missing/installed/mismatch/…). url-only entries (no path yet) read as missing. For git checkouts (clone/submodule) it reads the live checked-out branch (`CurrentBranch`, gitstack's `repo.CurrentBranch` re-exported, exposed on `Status.LiveBranch`) and reports `StateBranchChanged` when it differs from the recorded `tag` — branch drift, reconciled by re-recording the tag via the per-addon **Update branch record** action. A present git checkout is never touched by `Install All` (it skips clones/submodules whether unversioned or drifted).
- `addon.Install` / `addon.InstallAll` — fetch (`.zip` download / `.git` clone / **local `.zip` path** for archived packages), derive the install dir from the package's `plugin.cfg`/`version.cfg` (`internal/addon/cfg.go`), and report progress via a callback. `Install` returns the resolved path+version. Both take a leading `ctx context.Context` (as do `UpdateAll`/`InstallAllDeps`) — cancelling it aborts the in-flight download/clone (HTTP request + `git clone` are context-bound), which is how the TUI's task-abort works.
- `addon.GitSyncStatus` / `addon.GitFetch` / `addon.FetchAll` (engine in `gitstack/repo`,
  re-exported as `addon.*`; `FetchAll` is gdaddon's `[]Status` adapter over `repo.FetchAll`) — a git checkout's
  divergence from its upstream (`GitSync{Ahead, Behind, Tracking}`), a ctx-bound `git fetch`,
  and the fan-out that fetches every present clone/submodule in an inspected manifest. The
  split matters: **`GitSyncStatus` is a local read** of the remote-tracking refs git already
  has (a `git rev-list`, same cost class as `HasUncommittedChanges`), so it rides the
  synchronous `appctx.Ctx.refreshGitChecks` pass next to `GitDirty` and populates `Ctx.GitSync`
  on every refresh — but it can only *see* new upstream commits after a fetch. **`GitFetch` is
  the only network call**, so it's bound to an explicit key (Project tab `f` →
  `tabs/project/fetch.go`, an async cmd broadcasting `repoui.FetchDoneMsg`, mirroring `checkUpdatesCmd`)
  and never runs on its own. Both `GitFetch` and the clone paths set `GIT_TERMINAL_PROMPT=0`
  so a repo with uncached credentials fails fast instead of hanging the TUI on an invisible
  auth prompt. The counts surface as the `behind origin N` / `ahead N` row markers alongside
  `uncommitted changes`; behind-ness is a *flag*, not an `addon.State` (a checkout can be
  behind, dirty, and branch-drifted at once).
- `addon.GitStream` / `GitStatus` / `GitPull` / `GitPush` / `GitCommit` (engine in
  `gitstack/repo/ops.go`, re-exported as `addon.*`) — the git operations behind the per-addon
  **Git** submenu, which is now the shared `repoui.RepoMenu` (status, fetch, pull, push, commit —
  a `components.NewStayTask` per op, streaming git's output to the log via `Reporter`, then
  broadcasting `repoui.RefreshMsg` so the row markers settle; `tabs/project/git.go` is a thin
  adapter mapping `Status`→`repo.Repo`). `GitStream` is the
  primitive: it runs git with `repo.GitEnv()` (`GIT_TERMINAL_PROMPT=0` + `GIT_EDITOR=true`, so git
  can never sit waiting for input a TUI can't give) and relays stdout+stderr through a
  `lineWriter` that breaks on `\r` as well as `\n` (git's progress output is CR-delimited).
  **This is not a git client**: `GitPull` is `--ff-only`, so a diverged branch aborts having
  changed nothing rather than leaving a conflicted tree behind the TUI, and every failure
  routes the user to a real terminal. **`GitCommit`'s `stageAll` is load-bearing**: `commit -a`
  stages only *tracked* files, so a newly created file is silently left out — the commit form
  makes the user pick (`-a` vs a leading `add -A`), and the confirm (`commitBody`) lists what
  will be committed *and* names the untracked files that won't. `addon.GitChanges`
  (`gitstack/repo`, read-only) parses `status --porcelain` for that screen. The commit form and
  its confirm/body all live in `repoui` now, keyed on `repo.Repo`. The **project-wide** version
  is the shared `repoui.AllReposMenu`, reached via `internal/tui/flows/git/` (a flow, since both
  the Project tab and Actions open it): `git.AllRepos` builds the three scopes — clones,
  submodules, all (clones default — pulling a submodule dirties the parent repo's recorded
  pointer) — and hands them to `repoui.AllReposMenu`, which owns the scope cycling, the confirm
  (`confirmBody`, capped like `commitBody`) and the **sequential** per-repo batch (reading git's
  output is the point, so concurrent interleaving is avoided; the concurrent no-confirm path is
  the `f` key / `addon.FetchAll`). The single-repo streaming stay-task (`repoui.Task`) is shared
  by the batch and the per-repo submenu; both raise `repoui.RefreshMsg` on success (aliased as
  `appctx.GitRefresh`) so the Project list's local markers settle without re-firing the network
  update check. Keys: `v` (row-level, an addon's own Git page) and `V` (the all-repos page) —
  deliberately not `g`/`G`, which bubbles binds to jump-to-top/bottom on every list.
- `addon.InstallOne` / `addon.InstallDepsFor` (`internal/addon/install_one.go`) — install one addon and pin it, and walk the dependency closure rooted at one installed addon. The targeted counterpart to `InstallAll`/`InstallAllDeps`; front-end agnostic, so the CLI's `gdaddon install` and (later) a per-addon TUI action share them. See "Installing one addon" above for the semantics that differ from the manifest-wide flow.
- `addon.ParseRepoSpec` / `addon.SelectRelease` / `addon.SelectAsset` — parse an `owner/repo[@tag]` spec (the exported `parseDependency`, shared with `deps=` items), pick the release for a tag (or the latest non-prerelease), and pick its install asset (`*AmbiguousAssetError` when a release ships several uploads).
- `addon.UpdateEntry` / `addon.AddEntry` — rewrite a manifest entry's url/path/version/tag in place (empty url/path leaves that line untouched) / append a new entry (deduped by `source.RepoID`). `addon.SetKind` / `addon.SetLock` / `addon.SetCommit` / `addon.SetIsDependency` write single scalar lines the same way (empty/false value removes the line) — `SetCommit` records/clears a branch package's pinned HEAD sha, `SetIsDependency` records/clears a dep's auto-added provenance. `addon.OrphanDeps` reports which `is_dependency` entries nothing installed still needs (the "unused dependency" marker).
- `source.AvailableVersions` / `source.Branches` / `source.RepoID` — configured-host releases (uploaded `.zip`s + a generated source archive), branch archives, and canonical repo identity, driven by per-host VCS rules from config/sources.yml (github.com/codeberg.org as defaults). `Branches` pins each branch to its HEAD commit (`Asset.Commit` + a `commit_archive_url`) when the host rule supplies `branches.commit_path` + `commit_archive_url`, else falls back to the floating branch-HEAD archive.
- `archive.Archive` / `archive.List` / `archive.Repos` / `archive.Merge` — save a downloaded asset zip (ctx-first, so the archive task's abort cancels the download), read one repo's archived packages back as "(archived)" releases (local-file URLs), enumerate every archived repo (the Archive tab), and fold them into a `source.Listing` (with archive-only fallback when the upstream fetch fails). A commit-pinned branch package is stored under `<branch>@<sha>` (so distinct commits of the same branch don't overwrite), and `parseArchiveTag` recovers the branch + `Asset.Commit` pin when the archive is listed back.
- `archive.RemoveRepo` / `archive.Remove` — delete a repo's whole archive (used by Global → Remove "+ archive"), or one archived package by its local path, pruning emptied folders (the Archive tab).

## Installing the binary

**Dev:** `install_unix.sh` symlinks the built binary into `~/.local/bin/gdaddon`
(target tracks `build/`, so it follows rebuilds). Update `GO_DIR` inside the script if
your repo path differs.

**Release (general users):** `install.sh` places the binary and nothing else does —
`gdaddon` has no self-install command, matching gote/gossh/golaunch/repoview. It's the
same script the README tells users to curl, and `BIN_DIR` picks the destination
(default `~/.local/bin`). The `~/.gdaddon/bin` copy the Godot EditorPlugin launches via
`~/.gdaddon/bin/gdaddon` (`+.exe` on Windows, after probing PATH for a global `gdaddon`)
is just `BIN_DIR="$HOME/.gdaddon/bin"`, and `install.sh`'s post-install note says so.

This used to be a `self-install` command backed by `internal/installer` — a Dest enum,
a temp-file-plus-rename copy, and per-OS PATH wiring (Windows registry, unix profile
guidance). All of it is deleted. **Don't bring it back**: placing a binary is a shell
script's job, and having gdaddon do it is what made `install` vs `self-install` a
confusing pair in the first place.

### Self-update

`gdaddon update` is `selfupdate.NewUpdateCommand("brohd11/gdaddon", "gdaddon", version)`
from `github.com/brohd11/goutil/selfupdate` — the identical one-liner every sibling app
registers, so `cmd/update.go` holds no gdaddon-specific logic. It resolves the latest tag
off the `/releases/latest` redirect (no GitHub API, no rate limit) and compares semvers
locally — a `dev` build is never comparable, hence never offered an update. When a newer
release exists it downloads the repo's own `install.sh` and runs it with `BIN_DIR` set to
`goutil.BinDir()` (the running binary's own directory, **symlinks deliberately
unresolved**) and `VERSION` pinned to the checked tag, `--no-modify-path` so PATH is never
touched. install.sh stages in a temp dir and `mv -f`s into place, so overwriting the
running binary is safe. `--check` reports without installing.

The TUI side is `appctx.SelfUpdateHooks`, a one-liner over
`bubblestack/selfupdate.Hooks(appName, repo, version)` — the shared bridge that owns the
goutil `Check`/`Apply`/`BinDir` wiring and the `selfupdate.Info` ↔
`components.SelfUpdateInfo` conversion for every app. It used to be a copy of
`repoview/internal/app/update.go`; both now call the bridge, so there is nothing left to
keep in sync by hand. It feeds both the startup check (wired as
`bubblestack.Config.Init` → `appctx.SelfUpdateCheckCmd`, a generic app-level hook in
`bubblestack/core`'s Router), which writes an "update available" line to the status/log,
and Actions ▸ Update gdaddon, which runs the loading → confirm → task flow in-TUI. Both
ends share `bubblestack/components/selfupdate.go`; only the hooks are gdaddon's. A
self-update doesn't change the already-running process — relaunch to use the new binary.

Note for anyone updating the companion Godot plugin: the old `gdaddon self-update --check
--json` contract is **gone** (the shared command has no `--json` and no `self-update`
alias). `gdaddon update --check` prints three human lines.

`make package` zips each platform build into `dist/` (`zip -j`, binary only). Caveat:
macOS Gatekeeper warns on the unsigned binary first run (right-click → Open, or clear
quarantine with `xattr -dr com.apple.quarantine`).

On startup `runRoot` also writes `~/.gdaddon/.gitignore` (ignoring `bin/`) if absent —
`~/.gdaddon` is meant to be committable, but the OS binary isn't by default
(`config.EnsureGitignore`). An existing `.gitignore` is left alone, so a user can opt to
commit the binary.
