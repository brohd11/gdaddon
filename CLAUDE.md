# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`gdutil` — a Go CLI (built with [Cobra](https://github.com/spf13/cobra)) that collects Godot development utilities as subcommands.

Current subcommands:
- `addon_install` — installs Godot addons from a YAML manifest (`.zip` or `.git`, with version pinning via `plugin.cfg`)

A companion Godot EditorScript (`namespace_builder.gd`) also lives here — it runs inside the Godot editor to auto-generate GDScript namespace preload files and is a candidate for a future `namespace_build` subcommand.

## Build commands

```bash
# Build for current platform only
go build -o build/mac-arm64/gdutil .

# Cross-compile all targets (mac-arm64, mac-x86_64, linux, windows)
make

# Clean build artifacts
make clean
```

Build outputs go to `build/<platform>/gdutil[.exe]`.

## Running

```bash
gdutil addon_install                                        # manifest found via filesystem walk + git root auto-detected
gdutil addon_install path/to/addon_manifest.yml             # explicit manifest
gdutil addon_install path/to/addon_manifest.yml /godot/proj # explicit manifest + project root
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
main.go          — calls cmd.Execute()
cmd/
  root.go        — defines the root `gdutil` cobra command and Execute()
  addon_install.go — addon_install subcommand + all install logic
```

Adding a new subcommand: create `cmd/<name>.go`, define a `*cobra.Command`, and register it via `rootCmd.AddCommand(...)` in an `init()` function.

Key functions in `cmd/addon_install.go`:
- `runAddonInstall()` — cobra RunE handler; resolves manifest path and project root, then calls `installAddons`.
- `installAddons()` — parses YAML, skips already-installed versions, dispatches to zip or git installer.
- `downloadAndExtractZip()` — downloads to temp file, extracts, finds the target subfolder by name.
- `cloneGitRepo()` — shallow-clones (`--depth 1`); if the repo has an `addons/` folder, extracts the target subdirectory; otherwise installs the full repo. Strips `.git` from the destination.
- `getLocalPluginVersion()` — reads `plugin.cfg` (INI via `gopkg.in/ini.v1`) to compare against the YAML-pinned version.

## Installing the binary

`install_unix.sh` symlinks the built binary into `~/.local/bin/godot_addon_installer`. Update `GO_DIR` inside the script if your repo path differs from `~/dotfiles/.misc/scripts/go/godot_addon_installer`.
