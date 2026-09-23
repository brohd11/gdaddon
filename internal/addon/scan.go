package addon

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/brohd11/goutil/strutil"
)

// scanMaxDepth caps how deep ScanInstalled descends from the project root looking
// for plugin folders (root is depth 0, addons/<name> is depth 2).
const scanMaxDepth = 4

// Installed is a plugin folder ScanInstalled found: its project-relative path, a display
// name (config name, else folder name) and version. SuggestedURL prefills tracking: the
// git origin, else a `source=` key, else a matching path-less manifest entry (from
// UntrackedInstalls). Kind and Branch are set for a folder that is its own checkout.
type Installed struct {
	Path         string
	Name         string
	Version      string
	SuggestedURL string
	Kind         Kind
	Branch       string
}

// ScanInstalled walks root (to scanMaxDepth, skipping dot-folders) and returns each
// top-level plugin folder, not descending into one once found.
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
		if strutil.Depth(root, path) > scanMaxDepth {
			return filepath.SkipDir
		}
		if !hasPluginCfg(path) {
			return nil
		}
		kind, remote, branch := gitProbe(path)
		name := getLocalPluginName(path)
		if name == "" {
			name = base
		}
		sug, entryKind := SourceURL(path), KindPackage
		switch {
		case kind == gitRepo && remote != "":
			sug, entryKind = remote, KindClone // a real checkout's origin wins over source=
		case kind == gitSubmodule && remote != "":
			sug, entryKind = remote, KindSubmodule // parent-managed; registered for utility only
		default:
			branch = "" // only real checkouts carry a tracked branch
		}
		out = append(out, Installed{
			Path:         filepath.ToSlash(rel),
			Name:         name,
			Version:      getLocalPluginVersion(path),
			SuggestedURL: sug,
			Kind:         entryKind,
			Branch:       branch,
		})
		return filepath.SkipDir // found the top-level plugin here; don't dive in
	})
	return out, err
}

// UntrackedInstalls returns installed folders no entry tracks by path. For those without a
// source url, a path-less entry whose name matches the folder prefills SuggestedURL, so
// tracking fills in its path instead of duplicating it.
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
		// An author-declared `source=` (set by ScanInstalled) wins; fall back to a
		// matching pathless manifest entry's url only when none was declared.
		if in.SuggestedURL == "" {
			if url, ok := pathlessURL[strings.ToLower(filepath.Base(in.Path))]; ok {
				in.SuggestedURL = url
			}
		}
		out = append(out, in)
	}
	return out, nil
}

// normPath canonicalizes a manifest/relative path for comparison.
func normPath(p string) string {
	return filepath.ToSlash(filepath.Clean(p))
}
