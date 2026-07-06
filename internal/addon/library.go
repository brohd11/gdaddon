package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gdaddon/internal/config"
	"gdaddon/internal/source"
)

// GlobalListPath is the user's cross-project plugin library: a manifest-shaped
// YAML file (usually url-only entries) under ~/.gdaddon. New Plugin → Global
// writes here; Import Plugin reads from it. The folder is git-committable and is
// the future home for archived/downloaded assets.
func GlobalListPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "plugins.yml"), nil
}

// FindByRepo returns the entry whose URL is the same repo as url — matched by
// source.RepoID, so .git vs release-zip forms collapse. ok is false when url is
// unparseable or nothing matches. The target id is resolved once, then compared
// against each entry's RepoID.
func FindByRepo(entries []Addon, url string) (Addon, bool) {
	id, err := source.RepoID(url)
	if err != nil {
		return Addon{}, false
	}
	for _, e := range entries {
		if eid, err := source.RepoID(e.URL); err == nil && eid == id {
			return e, true
		}
	}
	return Addon{}, false
}

// IndexByRepo maps each parseable entry's source.RepoID to the entry, for repeated
// by-repo lookups (later duplicates win). Unparseable urls are skipped.
func IndexByRepo(entries []Addon) map[string]Addon {
	m := make(map[string]Addon, len(entries))
	for _, e := range entries {
		if id, err := source.RepoID(e.URL); err == nil {
			m[id] = e
		}
	}
	return m
}

// InGlobal reports whether this addon's repo is already present in a pre-loaded
// global addon list (matched by source.RepoID so .git vs release-zip collapse).
func (s Status) InGlobal(globals []Addon) bool {
	_, ok := FindByRepo(globals, s.Addon.URL)
	return ok
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
	entries, err := Parse(globalPath)
	if err != nil { // includes file-not-exist → empty list
		return false
	}
	_, ok := FindByRepo(entries, url)
	return ok
}

// UpsertEntry updates the existing entry for a.URL's repo (matched by source.RepoID)
// in place — overwriting its url/version/tag — or appends a new one when absent. Used
// where re-selecting a plugin should re-pin it rather than error on a duplicate
// (a set's "Add Version", tracking an installed plugin). Reuses UpdateEntry /
// AddEntryFull. An empty tag leaves an existing tag line untouched (a branch pin
// records no tag); a.Kind is applied additively (set for a non-package kind, never
// cleared here).
func UpsertEntry(manifestPath string, a Addon) error {
	existingName := ""
	if entries, err := Parse(manifestPath); err == nil {
		if e, ok := FindByRepo(entries, a.URL); ok {
			existingName = e.Name
		}
	}
	if existingName == "" {
		return AddEntryFull(manifestPath, a)
	}
	// UpdateEntry leaves path/tag untouched when "" and writes version.
	if err := UpdateEntry(manifestPath, existingName, a.URL, a.Path, a.Version, a.Tag); err != nil {
		return err
	}
	if a.Kind != KindPackage {
		if err := SetKind(manifestPath, existingName, a.Kind); err != nil {
			return err
		}
	}
	if a.Lock {
		return SetLock(manifestPath, existingName, true)
	}
	return nil
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
	if len(existing) > 0 {
		entries, perr := Parse(manifestPath)
		if perr != nil {
			return perr
		}
		if e, ok := FindByRepo(entries, url); ok {
			id, _ := source.RepoID(url)
			return fmt.Errorf("already added from %s (as %q)", id, e.Name)
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
