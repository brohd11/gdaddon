## gdaddon - TUI addon manager for Godot

![gdaddon interface](doc/gdaddon.jpg)

### Features
 - Project manifest - one button to install all logged plugins to pinned version
 - Update Check - checks for new releases of all your plugins
 - Declare dependencies - reads from plugin.cfg, see below
 - Search - query Github, Asset Store, Asset Lib to add directly to your project
 - Global manifest - quickly add your favorite addons to your project
 - Addon sets - save a collection of addons that can be added together
 - Archive - save a copy of any package locally, can be used for install

## Quick Start

### Manifest
Each addon has an entry in the manifest. This editable via the TUI, or by hand.
The path field is for installing repos that are in the submodule format, where it is difficult to infer the plugin directory name.

```
MyAddon:
    tag: "v1.0.0-stable"
	version: "1.0.0"
	url: https://github.com/user/repo/archive/refs/tags/v1.0.0-stable.zip
	path: addons/terrain_3d
```

### Dependency Management
If your addon relies on third party content, you can define those in the plugin.cfg
```
[plugin]
name="My Plugin"
---
deps=["user/repo/@v1.0.0"] # point to the release tag
```
When the addon is installed, the plugin.cfg will be read and checked for dependencies. If they are found and the dependency is not present, the addon will be flagged and you can run the get dependencies command to add them to your project.

If you don't have a tagged version, just the repo will be added to your project where you can add the proper version manually.

There is also an "Install All + Deps" command that will  install, check for dependencies, loop, until there are none left that can be installed.

### Install

There are binaries under releases, you can also build with Go: `make` will build for all platforms.

on macOS, downloaded binaries may have quarantine status that needs to be cleared before you can execute: `xattr -dr com.apple.quarantine path/to/gdaddon`

Alternatively, build and this is not a problem.

The `install_unix.sh` script will symlink the binary into "$HOME/.local/bin/gdaddon", so you can just run `gdaddon` in any project to start it.


More docs can be found [here](doc/docs.md)

