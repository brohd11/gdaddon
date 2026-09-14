package addon

import (
	"net/url"
	"strings"

	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/store"
)

// EntryKey is the manifest key a newly recorded entry takes: the addon's canonical
// identity, "<host>/<owner>/<repo>" — github.com/brohd11/gdaddon, or
// store.godotengine.org/<publisher>/<slug> for an Asset Store package. The key is what
// every lookup and write addresses, so making it the identity means re-recording the
// same repo finds the same entry no matter what the addon calls itself, and renaming a
// plugin never strands its entry.
//
// A url with no parseable identity falls back to DeriveName, which is exactly what
// every caller recorded before this existed. Nothing migrates: a hand-written or legacy
// key is a perfectly good key and is never rewritten.
func EntryKey(rawURL string) string {
	if store.IsStoreURL(rawURL) {
		if id, err := store.AssetID(rawURL); err == nil {
			return store.Host + "/" + id
		}
	}
	if id, err := source.RepoID(rawURL); err == nil {
		return id
	}
	return DeriveName(rawURL)
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
