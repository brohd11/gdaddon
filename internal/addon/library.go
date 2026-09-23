package addon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/brohd11/gdaddon/internal/config"
	"github.com/brohd11/gdaddon/internal/source"
)

// GlobalListPath is the user's cross-project plugin library under ~/.gdaddon, a
// manifest-shaped file of mostly url-only entries.
func GlobalListPath() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "plugins.yml"), nil
}

// FindByRepo returns the entry for url's repo, matched by source.RepoID (so .git and zip
// urls match); false when unparseable or absent.
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

// IndexByName maps each entry's key to the entry (later duplicates win). Dependency name
// matching is depIndex, not this.
func IndexByName(entries []Addon) map[string]Addon {
	m := make(map[string]Addon, len(entries))
	for _, e := range entries {
		m[e.Name] = e
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

// setGlobalDisplayName sets display on the global entry for url's repo when it has no
// name, matched by repo identity since the two sides store different url forms. Silent
// and best-effort: most users have no global list.
func setGlobalDisplayName(url, display string) {
	globalPath, err := GlobalListPath()
	if err != nil {
		return
	}
	entries, err := Parse(globalPath)
	if err != nil { // includes file-not-exist → nothing to backfill
		return
	}
	e, ok := FindByRepo(entries, url)
	if !ok || e.Display != "" {
		return
	}
	_ = SetDisplayName(globalPath, e.Name, display)
}

// UpsertEntry updates the entry for a.URL's repo in place (url, version, tag) or appends
// one, so re-selecting a plugin re-pins it. An empty tag leaves the existing tag; a
// non-package kind is set but never cleared here.
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

// CreateManifest creates an empty manifest (and parent dirs), refusing to overwrite.
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

// ErrNameTaken marks a key already used by a different repo's entry, so front-ends can
// offer a rename.
var ErrNameTaken = errors.New("that name is taken by another entry")

// AddEntry appends a top-level entry, creating the file if needed:
//
//	<name>:
//	    url: <url>
//	    path: <path>
//
// An empty path is omitted. An existing key is an error.
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
			return fmt.Errorf("%q is already in %s (%w)", name, filepath.Base(manifestPath), ErrNameTaken)
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
