package addon

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// scanMaxDepth caps how deep ScanInstalled descends from the project root looking
// for plugin folders (root is depth 0, addons/<name> is depth 2).
const scanMaxDepth = 4

// Installed is a plugin folder found on disk by ScanInstalled: its project-root-
// relative path, a display name (the config's name key, else the folder basename),
// and the version read from its plugin.cfg/version.cfg. SuggestedURL is set by
// UntrackedInstalls when a pathless manifest entry looks like this folder.
type Installed struct {
	Path         string
	Name         string
	Version      string
	SuggestedURL string
}

// ScanInstalled walks the project root (up to scanMaxDepth, skipping dotfolders like
// .godot/.git) and returns each top-level plugin folder — a directory holding a
// plugin.cfg/version.cfg. It stops descending into a plugin folder once found, so a
// nested sub-addon is reported as part of its parent, not on its own.
func ScanInstalled(root string) ([]Installed, error) {
	var out []Installed
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path == root {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(base, ".") {
			return filepath.SkipDir
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return filepath.SkipDir
		}
		if strings.Count(rel, string(filepath.Separator))+1 > scanMaxDepth {
			return filepath.SkipDir
		}
		if !hasPluginCfg(path) {
			return nil
		}
		name := readPluginCfgKey(path, "name")
		if name == "" {
			name = base
		}
		out = append(out, Installed{
			Path:    filepath.ToSlash(rel),
			Name:    name,
			Version: getLocalPluginVersion(path),
		})
		return filepath.SkipDir // found the top-level plugin here; don't dive in
	})
	return out, err
}

// UntrackedInstalls returns the installed plugin folders under root that no manifest
// entry already tracks by path. For each, when a manifest entry exists with an empty
// path whose name matches the folder basename (the cogito case — tracked by url but
// never pinned), SuggestedURL is prefilled with that entry's url so capturing it
// backfills the path rather than adding a duplicate.
func UntrackedInstalls(manifestPath, root string) ([]Installed, error) {
	installed, err := ScanInstalled(root)
	if err != nil {
		return nil, err
	}

	entries, _ := Parse(manifestPath) // missing/empty manifest ⇒ nothing tracked
	tracked := make(map[string]bool, len(entries))
	pathlessURL := make(map[string]string)
	for _, e := range entries {
		if e.Path != "" {
			tracked[normPath(e.Path)] = true
		} else {
			pathlessURL[strings.ToLower(e.Name)] = e.URL
		}
	}

	var out []Installed
	for _, in := range installed {
		if tracked[normPath(in.Path)] {
			continue
		}
		if url, ok := pathlessURL[strings.ToLower(filepath.Base(in.Path))]; ok {
			in.SuggestedURL = url
		}
		out = append(out, in)
	}
	return out, nil
}

// normPath canonicalizes a manifest/relative path for comparison.
func normPath(p string) string {
	return filepath.ToSlash(filepath.Clean(p))
}
