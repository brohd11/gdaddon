package addon

import (
	"net/url"
	"strings"

	"github.com/brohd11/gdaddon/internal/source"
	"github.com/brohd11/gdaddon/internal/store"
)

// EntryKey is a new entry's manifest key: its canonical identity "<host>/<owner>/<repo>"
// (or store.godotengine.org/<publisher>/<slug>), so re-recording a repo finds the same
// entry whatever the addon calls itself. Unparseable urls fall back to DeriveName; existing
// keys are never rewritten.
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

// DeriveName returns a url's last path segment without .git/.zip ("Foo" from
// github.com/u/Foo.git), or "plugin".
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

// CloneURL is the https .git url a clone install of repoID (host/owner/repo) records.
func CloneURL(repoID string) string { return "https://" + repoID + ".git" }

// TagVersion is the version a release tag names: the tag without a leading "v".
func TagVersion(tag string) string { return strings.TrimPrefix(tag, "v") }

// NormalizeRepoURL appends ".git" to a bare repo url so it can be cloned and still parsed;
// .zip and .git urls pass through.
func NormalizeRepoURL(rawURL string) string {
	trimmed := strings.TrimRight(rawURL, "/")
	if strings.HasSuffix(trimmed, ".git") || strings.HasSuffix(trimmed, ".zip") {
		return trimmed
	}
	return trimmed + ".git"
}
