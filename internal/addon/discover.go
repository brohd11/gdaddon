package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brohd11/goutil/strutil"
)

// MaxManifestDepth bounds FindManifest's search and where a new manifest may be created, so
// it stays discoverable from the root.
const MaxManifestDepth = 5

// manifestNames are the filenames FindManifest looks for.
var manifestNames = map[string]bool{"addon_manifest.yml": true, "addon_manifest.yaml": true}

// FindManifest searches start (to MaxManifestDepth, hidden dirs included, ".godot" skipped)
// shallow-first for a manifest. A miss is ("", nil), so the TUI can start without one.
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
			if strutil.Depth(start, path) > MaxManifestDepth {
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

// WithinManifestDepth reports whether dir is root or within MaxManifestDepth below it,
// where FindManifest would still find a manifest created there.
func WithinManifestDepth(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return strutil.Depth(root, dir) <= MaxManifestDepth
}
