package addon

import (
	"os"
	"path/filepath"

	"github.com/brohd11/goutil/strutil"
)

// placement is one source folder in the staging tree and the project-root-relative
// path it should be installed to.
type placement struct {
	src     string
	destRel string
}

// pluginDirs returns every directory under root holding an addon config, dropping matches
// nested in another (a sub-addon belongs to its parent).
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

// isUnder reports whether path is a strict descendant of base.
func isUnder(path, base string) bool {
	rel, ok := strutil.RelUnder(base, path)
	return ok && rel != "."
}

// resolveInstall decides where staged content lands, relative to the project root. The
// manifest path is authoritative only when the config is at the staging root (the whole
// tree is the addon); otherwise destinations are derived, and a pinned path applies to a
// single folder but never collapses a bundle. Precedence: definedPath, a dir= override
// (destFor), addons/<name>.
//
// Derivation order:
//  1. config at the staging root: the whole tree is the addon;
//  2. an addons/ folder anywhere: mirror its children into addons/ (descending into
//     namespace levels, see addonsTargets), which also handles config-less packages;
//  3. otherwise find plugin folders by their configs.
func resolveInstall(stagingRoot, name, definedPath, pkgName string) []placement {
	// rootName is the folder name for a whole-tree install: the author's package folder when
	// known, else the manifest name.
	rootName := name
	if pkgName != "" {
		rootName = pkgName
	}

	// The root itself is the addon: install the whole tree (to the pinned path, else
	// addons/<name>), including any addons/ it bundles.
	if hasPluginCfg(stagingRoot) {
		return []placement{{src: stagingRoot, destRel: pathOr(definedPath, destFor(stagingRoot, DefaultPath(rootName)))}}
	}

	// Derive from the addons/ folder's children, ignoring siblings like docs/ and the wrapper
	// name.
	if addonsDir := findAddonsDir(stagingRoot); addonsDir != "" {
		if ps := placementsForDirs(addonsDir, addonsTargets(addonsDir), definedPath); len(ps) > 0 {
			return ps
		}
	}

	dirs := pluginDirs(stagingRoot)
	if len(dirs) == 0 {
		// No config anywhere: install the whole tree (pinned path, else addons/<name>).
		return []placement{{src: stagingRoot, destRel: pathOr(definedPath, DefaultPath(rootName))}}
	}
	// No addons/ folder: the package root is the addons folder, so namespaces like addon_lib/
	// are kept.
	return placementsForDirs(stagingRoot, dirs, definedPath)
}

// placementsForDirs maps plugin folders to destinations (definedPath, dir= override,
// addons/<path under base>). A single folder honors definedPath; a bundle derives each
// folder's own (see primaryPlacement). nil for none.
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

// defaultPathUnder is addons/ plus dir's path relative to base, keeping intermediate
// levels (addon_lib/my_addon). base is the package's addons/ folder or its root; falls
// back to the leaf name.
func defaultPathUnder(base, dir string) string {
	rel, ok := strutil.RelUnder(base, dir)
	if !ok || rel == "." {
		return DefaultPath(filepath.Base(dir))
	}
	return DefaultPath(filepath.ToSlash(rel))
}

// findAddonsDir returns the shallowest "addons" directory under root (not descending into
// it), so a bundled nested addons/ is ignored; "" if none.
func findAddonsDir(root string) string {
	best, bestDepth := "", -1
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || !info.IsDir() {
			return nil
		}
		if filepath.Base(path) != "addons" {
			return nil
		}
		depth := strutil.Depth(root, path)
		if best == "" || depth < bestDepth {
			best, bestDepth = path, depth
		}
		return filepath.SkipDir
	})
	return best
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

// addonsTargets returns the install targets under an addons/ folder: each child, or the
// plugin folders inside a config-less namespace child (addons/addon_lib/tree_sitter_gd).
// A child with no config anywhere (an asset pack) is kept whole, so this is not simply
// pluginDirs.
func addonsTargets(addonsDir string) []string {
	var out []string
	for _, child := range childDirs(addonsDir) {
		if hasPluginCfg(child) {
			out = append(out, child)
			continue
		}
		if nested := pluginDirs(child); len(nested) > 0 {
			out = append(out, nested...)
			continue
		}
		out = append(out, child)
	}
	return out
}

// pathOr returns p when set, else the fallback — used to express the "explicit
// manifest path wins, otherwise derive" precedence in resolveInstall.
func pathOr(p, fallback string) string {
	if p != "" {
		return p
	}
	return fallback
}

// destFor returns a config dir's declared dir= override, else fallback.
func destFor(dir, fallback string) string {
	if d := installDir(dir); d != "" {
		return d
	}
	return fallback
}
