package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxManifestDepth limits how deep FindManifest descends from the start dir, and
// bounds where a freshly-created manifest may live so it stays discoverable from the
// project root.
const MaxManifestDepth = 5

// manifestNames are the filenames FindManifest looks for.
var manifestNames = map[string]bool{"addon_manifest.yml": true, "addon_manifest.yaml": true}

// FindManifest walks the tree rooted at start (up to MaxManifestDepth dirs deep,
// including hidden dirs but skipping ".godot") for an addon manifest, returning the
// first match in a shallow-first traversal. A miss is ("", nil) — only an actual walk
// failure is an error — so the TUI can launch without a manifest and bootstrap one.
func FindManifest(start string) (string, error) {
	var found string
	err := filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			if path != start && d.Name() == ".godot" {
				return filepath.SkipDir
			}
			if manifestDepth(start, path) > MaxManifestDepth {
				return filepath.SkipDir
			}
			return nil
		}
		if manifestNames[d.Name()] {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("error searching for manifest: %w", err)
	}
	return found, nil
}

// WithinManifestDepth reports whether dir is the project root or a descendant of it no
// deeper than MaxManifestDepth — i.e. a place where a created manifest would still be
// found by FindManifest from the root. It guards the Create-manifest form against
// writing an unreachable manifest.
func WithinManifestDepth(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return manifestDepth(root, dir) <= MaxManifestDepth
}

// manifestDepth returns how many directory levels path is below base.
func manifestDepth(base, path string) int {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." {
		return 0
	}
	return len(strings.Split(rel, string(filepath.Separator)))
}
