package addon

import (
	"os"
	"path/filepath"
	"strings"
)

// placement is one source folder in the staging tree and the project-root-relative
// path it should be installed to.
type placement struct {
	src     string
	destRel string
}

// pluginDirs returns every directory under root that holds an addon config file
// (see hasPluginCfg), pruned so a match nested inside another match is dropped (a
// sub-addon is managed by its parent addon, not installed on its own).
func pluginDirs(root string) []string {
	var dirs []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if hasPluginCfg(path) {
			dirs = append(dirs, path)
		}
		return nil
	})

	var pruned []string
	for _, d := range dirs {
		nested := false
		for _, other := range dirs {
			if other != d && isUnder(d, other) {
				nested = true
				break
			}
		}
		if !nested {
			pruned = append(pruned, d)
		}
	}
	return pruned
}

// isUnder reports whether path is a descendant of base.
func isUnder(path, base string) bool {
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..")
}

// resolveInstall decides where the staged content should land, relative to the
// project root. The manifest path is authoritative only for a *submodule-style*
// package — one whose plugin.cfg sits at the staging root, so the whole tree is the
// addon ("path is king"). Otherwise the package is a container of one or more plugin
// folders and destinations are *derived*: a pinned path still applies to a single
// folder (so a relocation is honored), but it can no longer collapse a multi-folder
// bundle into one folder (which previously dumped the entire tree into the pinned
// path). Precedence stays definedPath > a dir= override (destFor) > addons/<name>.
//
// Derivation order:
//  1. submodule-style (plugin.cfg at the staging root) — the whole tree is the addon.
//  2. an addons/ folder anywhere in the tree — the canonical Godot layout: mirror its
//     immediate child folders into the project's addons/. This handles packages with
//     loose files beside the plugin folder(s) and packages with no plugin.cfg at all
//     (icon packs, asset libraries), which the config search alone would mis-derive.
//  3. otherwise locate plugin folders by their plugin.cfg/version.cfg and derive.
func resolveInstall(stagingRoot, name, definedPath, pkgName string) []placement {
	// rootName is the install dir basename when the package is installed whole (no
	// config, or its config is at the staging root): the author's package folder
	// name when known, else the manifest name.
	rootName := name
	if pkgName != "" {
		rootName = pkgName
	}

	// Submodule-style: the package root itself is the addon. Install the whole tree
	// to the pinned path, else addons/<name> (honoring a dir= override). Kept first:
	// a root config unambiguously means the repo *is* the addon, so any addons/
	// subfolder it bundles must ride along inside it rather than be extracted alone.
	if hasPluginCfg(stagingRoot) {
		return []placement{{src: stagingRoot, destRel: pathOr(definedPath, destFor(stagingRoot, DefaultPath(rootName)))}}
	}

	// An addons/ folder in the tree is the canonical layout: its immediate child
	// folders are the plugins. Derive from those, ignoring sibling junk (docs/,
	// .github/, README) and the GitHub wrapper name.
	if addonsDir := findAddonsDir(stagingRoot); addonsDir != "" {
		if ps := placementsForDirs(addonsDir, childDirs(addonsDir), definedPath); len(ps) > 0 {
			return ps
		}
	}

	dirs := pluginDirs(stagingRoot)
	if len(dirs) == 0 {
		// No config anywhere: install the whole tree (pinned path, else addons/<name>).
		return []placement{{src: stagingRoot, destRel: pathOr(definedPath, DefaultPath(rootName))}}
	}
	// No addons/ folder: the package root *is* the addons folder, so a plugin folder's
	// path relative to it is its path under addons/ (addon_lib/my_addon keeps its
	// addon_lib namespace).
	return placementsForDirs(stagingRoot, dirs, definedPath)
}

// placementsForDirs maps a set of plugin folders to their install destinations with
// the derive precedence definedPath > a dir= override (destFor) > addons/<folder's path
// under base>. A single folder honors a pinned/relocated definedPath; a bundle of
// folders each derives its own destination (the pinned path can't collapse them —
// installStaged overwrites only the entry's own folder and leaves bundled siblings, see
// primaryPlacement). Returns nil for an empty set.
func placementsForDirs(base string, dirs []string, definedPath string) []placement {
	switch len(dirs) {
	case 0:
		return nil
	case 1:
		return []placement{{src: dirs[0], destRel: pathOr(definedPath, destFor(dirs[0], defaultPathUnder(base, dirs[0])))}}
	default:
		out := make([]placement, 0, len(dirs))
		for _, d := range dirs {
			out = append(out, placement{src: d, destRel: destFor(d, defaultPathUnder(base, d))})
		}
		return out
	}
}

// defaultPathUnder is a staged plugin folder's derived install path: addons/ + the
// folder's path relative to base, so directory levels between the two survive
// (base=<root>, dir=<root>/addon_lib/my_addon → addons/addon_lib/my_addon). Callers
// pick base to express where the addons/ anchor is: the package's own addons/ folder
// when it ships one, else the package root. Falls back to the leaf name when dir is
// not under base.
func defaultPathUnder(base, dir string) string {
	rel, err := filepath.Rel(base, dir)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return DefaultPath(filepath.Base(dir))
	}
	return DefaultPath(filepath.ToSlash(rel))
}

// findAddonsDir returns the shallowest directory named "addons" anywhere under root,
// or "" when there is none. Shallowest wins so a submodule that bundles its own
// addons/ (e.g. addons/foo/addons/bar) resolves to the top-level addons/, not the
// nested one — and a found addons/ is not descended into for that reason.
func findAddonsDir(root string) string {
	best, bestDepth := "", -1
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if filepath.Base(path) != "addons" {
			return nil
		}
		depth := pathDepth(root, path)
		if best == "" || depth < bestDepth {
			best, bestDepth = path, depth
		}
		return filepath.SkipDir
	})
	return best
}

// pathDepth is the number of path segments from base to path (0 when equal).
func pathDepth(base, path string) int {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(os.PathSeparator)) + 1
}

// childDirs returns the immediate subdirectories of dir (absolute paths), or nil.
func childDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(dir, e.Name()))
		}
	}
	return dirs
}

// pathOr returns p when set, else the fallback — used to express the "explicit
// manifest path wins, otherwise derive" precedence in resolveInstall.
func pathOr(p, fallback string) string {
	if p != "" {
		return p
	}
	return fallback
}

// destFor returns a staged config dir's install destination: the addon's own
// `dir=` override declared in its config (see installDir) when present, else the
// derived fallback. An explicit manifest path still wins upstream in resolveInstall.
func destFor(dir, fallback string) string {
	if d := installDir(dir); d != "" {
		return d
	}
	return fallback
}
