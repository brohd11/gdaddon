package addon

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gdaddon/internal/source"
)

// GlobalListPath is the user's cross-project plugin library: a manifest-shaped
// YAML file (usually url-only entries) under ~/.gdaddon. New Plugin → Global
// writes here; Import Plugin reads from it. The folder is git-committable and is
// the future home for archived/downloaded assets.
func GlobalListPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gdaddon", "plugins.yml"), nil
}

// InGlobal reports whether this addon's repo is already present in a pre-loaded
// global addon list (matched by source.RepoID so .git vs release-zip collapse).
func (s Status) InGlobal(globals []Addon) bool {
	id, err := source.RepoID(s.Addon.URL)
	if err != nil {
		return false
	}
	for _, g := range globals {
		if gid, err := source.RepoID(g.URL); err == nil && gid == id {
			return true
		}
	}
	return false
}

// Archived reports whether this addon's repo has any locally archived packages,
// given the pre-loaded list of archived repo IDs from archive.Repos().
func (s Status) Archived(archivedIDs []string) bool {
	id, err := source.RepoID(s.Addon.URL)
	if err != nil {
		return false
	}
	for _, aid := range archivedIDs {
		if aid == id {
			return true
		}
	}
	return false
}

// InGlobalList reports whether the global plugin list already has an entry for
// the same repo as url (matched by source.RepoID, so .git vs release-zip forms
// collapse). A missing/unparseable list or url reads as "not present".
func InGlobalList(url string) bool {
	globalPath, err := GlobalListPath()
	if err != nil {
		return false
	}
	id, err := source.RepoID(url)
	if err != nil {
		return false
	}
	entries, err := Parse(globalPath)
	if err != nil { // includes file-not-exist → empty list
		return false
	}
	for _, e := range entries {
		if eid, err := source.RepoID(e.URL); err == nil && eid == id {
			return true
		}
	}
	return false
}

// UpsertEntry updates the existing entry for url's repo (matched by source.RepoID)
// in place — overwriting its url/version — or appends a new one when absent. Used
// where re-selecting a plugin should re-pin it rather than error on a duplicate
// (a set's "Add Version"). Reuses UpdateEntry / AddEntryWithVersion.
func UpsertEntry(manifestPath, name, url, path, version string) error {
	existingName := ""
	if id, err := source.RepoID(url); err == nil {
		if entries, err := Parse(manifestPath); err == nil {
			for _, e := range entries {
				if eid, err := source.RepoID(e.URL); err == nil && eid == id {
					existingName = e.Name
					break
				}
			}
		}
	}
	if existingName != "" {
		// UpdateEntry leaves path untouched when "" and always writes version.
		return UpdateEntry(manifestPath, existingName, url, path, version)
	}
	return AddEntryWithVersion(manifestPath, name, url, path, version)
}

// DeriveName extracts a plugin name from a repo URL: the last path segment with
// any .git/.zip suffix stripped (e.g. github.com/u/Foo.git → "Foo"). Falls back
// to "plugin" if nothing usable is found.
func DeriveName(rawURL string) string {
	name := rawURL
	if u, err := url.Parse(rawURL); err == nil && u.Path != "" {
		name = u.Path
	}
	name = strings.Trim(name, "/")
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSuffix(name, ".git")
	name = strings.TrimSuffix(name, ".zip")
	if name == "" {
		return "plugin"
	}
	return name
}

// DefaultPath is the conventional install location for an addon of the given
// name, relative to the Godot project root.
func DefaultPath(name string) string {
	return "addons/" + name
}

// NormalizeRepoURL makes a typed repo URL installable: a bare github repo URL
// (no .git/.zip suffix) gets ".git" appended so Install-all can clone it and the
// version picker can still parse it. Explicit .zip/.git URLs pass through.
func NormalizeRepoURL(rawURL string) string {
	trimmed := strings.TrimRight(rawURL, "/")
	if strings.HasSuffix(trimmed, ".git") || strings.HasSuffix(trimmed, ".zip") {
		return trimmed
	}
	return trimmed + ".git"
}

// CreateManifest creates an empty manifest file at path (and its parent dirs),
// establishing a project's addon_manifest.yml before any entries exist. It refuses to
// overwrite an existing file. Parse/Inspect read the empty file as an empty addon
// list, and AddEntry appends to it later.
func CreateManifest(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", filepath.Base(path))
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte{}, 0o644)
}

// AddEntry appends a new top-level entry to a manifest-shaped YAML file, creating
// the file (and its parent dir) if absent. The block uses the flat 4-space shape:
//
//	<name>:
//	    url: <url>
//	    path: <path>
//
// An empty path omits the path line (used for url-only global entries). No
// version line is written. If name already exists as a column-0 key, it returns
// an error rather than duplicating it.
func AddEntry(manifestPath, name, url, path string) error {
	if name == "" || url == "" {
		return fmt.Errorf("plugin name and url are required")
	}

	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}

	existing, err := os.ReadFile(manifestPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Reject duplicates by repo identity first (names are just labels — the same
	// repo can appear as a .git or a release .zip), then by literal key.
	newID, idErr := source.RepoID(url)
	if len(existing) > 0 {
		entries, perr := Parse(manifestPath)
		if perr != nil {
			return perr
		}
		for _, e := range entries {
			if idErr == nil {
				if id, err := source.RepoID(e.URL); err == nil && id == newID {
					return fmt.Errorf("already added from %s (as %q)", id, e.Name)
				}
			}
		}
	}
	for _, ln := range strings.Split(string(existing), "\n") {
		if isEntryKey(ln, name) {
			return fmt.Errorf("%q is already in %s", name, filepath.Base(manifestPath))
		}
	}

	var b strings.Builder
	// Separate the new block from prior content with a single blank line,
	// normalizing any trailing newlines the file already had.
	if trimmed := strings.TrimRight(string(existing), "\n"); trimmed != "" {
		b.WriteString(trimmed)
		b.WriteString("\n\n")
	}
	b.WriteString(name)
	b.WriteString(":\n")
	b.WriteString("    url: ")
	b.WriteString(url)
	b.WriteString("\n")
	if path != "" {
		b.WriteString("    path: ")
		b.WriteString(path)
		b.WriteString("\n")
	}

	return os.WriteFile(manifestPath, []byte(b.String()), 0o644)
}
