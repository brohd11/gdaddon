# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`gdaddon` — a Go CLI (built with [Cobra](https://github.com/spf13/cobra)) for
browsing and installing Godot addons from a YAML manifest (`.zip` or `.git`, with
version pinning via `plugin.cfg`).

It's a single command, not a suite of subcommands:
- no args → launches the interactive TUI (browse, pick versions/branches/assets, install/update)
- `--install` → runs the non-interactive install (inspect manifest + install/update all), then exits

A companion Godot EditorScript (`namespace_builder.gd`) also lives here — it runs
inside the Godot editor to auto-generate GDScript namespace preload files.

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
gdaddon                                         # TUI; manifest found via filesystem walk + git root auto-detected
gdaddon path/to/addon_manifest.yml              # TUI with explicit manifest
gdaddon path/to/addon_manifest.yml /godot/proj  # TUI with explicit manifest + project root
gdaddon --install                               # non-interactive install of everything in the manifest
```

## Addon manifest format

```yaml
my_addon:
  url: https://example.com/addon.zip   # or https://github.com/user/repo.git
  path: addons/my_addon                # relative to Godot project root
  version: "1.2.3"                     # optional; skips install if already matches plugin.cfg
```

## Architecture

```
main.go              — calls cmd.Execute()
cmd/
  root.go            — the single `gdaddon` cobra command: TUI by default, --install for non-interactive
  paths.go           — resolvePaths (manifest filesystem-walk + git-root detection)
internal/
  addon/             — manifest parsing, install state (Inspect), Install/InstallAll, addon-config version read, manifest Update/AddEntry, ~/.gdaddon global list
  source/            — resolves remote versions from a URL (github.go: releases, branches, source archives; RepoID)
  archive/           — local package archive (~/.gdaddon/archive or config.yml archive_dir): store/list package zips, merge into a listing
  tui/               — bubbletea front-end (see internal/tui/doc.go)
    tui.go           — thin wiring: Run builds Shared chrome + the tab set, hands them to the router
    core/            — the framework: Shared state, Router over a screen stack, nav commands, Screen + optional interfaces, router messages, list/help/style helpers
    components/      — reusable, context-agnostic screens configured by closures (PickerScreen, ConfirmScreen, LoadingScreen, TaskScreen) — they name no domain type
    tabs/<domain>/   — one package per top-level tab (project, global, actions): its root screen, flow screens, and the builders that wire components to features
```

### TUI design goals

The TUI was restructured for scalability around two ideas:

- **Tabs are domains.** Each top-level tab is its own package under `tui/tabs/`
  (`project`, `global`, `actions`) owning its root screen and flows. Adding a
  feature area means adding a tab package, not editing a monolith.
- **Domains share `components` to simplify logic.** Reusable screens
  (picker/confirm/loading/streaming-task) live in `tui/components`, are
  context-agnostic, and are configured by closures the tab supplies — so a tab
  composes flows from shared pieces instead of reimplementing list/confirm/task
  plumbing.

Dependency direction is strictly `core ← components ← tabs/* ← tui`: `core` names
no concrete screen (reaches them via optional interfaces like `Relister`/
`RootHandler`), `components` name no domain type (closures only), and tabs never
import each other. That acyclic layering is what lets the screens live in
separate packages. See `internal/tui/doc.go` for the full contract and how to
add a tab.

Key packages/functions:
- `addon.Inspect(manifest, root)` — parses the manifest and computes each entry's local state (missing/installed/mismatch/…). url-only entries (no path yet) read as missing.
- `addon.Install` / `addon.InstallAll` — fetch (`.zip` download / `.git` clone / **local `.zip` path** for archived packages), derive the install dir from the package's `plugin.cfg`/`version.cfg` (`internal/addon/cfg.go`), and report progress via a callback. `Install` returns the resolved path+version.
- `addon.UpdateEntry` / `addon.AddEntry` — rewrite a manifest entry's url/path/version in place (empty url/path leaves that line untouched) / append a new entry (deduped by `source.RepoID`).
- `source.AvailableVersions` / `source.Branches` / `source.RepoID` — GitHub releases (uploaded `.zip`s + a generated source archive), branch-HEAD archives, and canonical repo identity.
- `archive.Archive` / `archive.List` / `archive.Merge` — save a downloaded asset zip, read archived packages back as "- archived" releases (local-file URLs), and fold them into a `source.Listing` (with archive-only fallback when the upstream fetch fails).

## Installing the binary

`install_unix.sh` symlinks the built binary into `~/.local/bin/gdaddon`. Update
`GO_DIR` inside the script if your repo path differs.
