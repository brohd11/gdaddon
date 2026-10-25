package addon

import (
	"fmt"
	"strings"

	"github.com/brohd11/gdaddon/internal/source"

	"gopkg.in/ini.v1"
)

// defaultDepHost is assumed when a dependency item names only owner/repo.
const defaultDepHost = "github.com"

// LatestTag is the reserved ref for the newest non-prerelease, resolved when the entry is
// recorded. A real tag named `latest` cannot be pinned by name.
const LatestTag = "latest"

// IsLatestTag reports whether ref is the reserved LatestTag word, case-insensitively —
// the single definition shared by the spec parser and ResolveVersion.
func IsLatestTag(ref string) bool { return strings.EqualFold(ref, LatestTag) }

// Dependency is one parsed plugin.cfg dependency, in PackageExporter's require grammar
// `[host/]owner/repo[/source|/release][@<ref>[:<glob>]]` (github.com unless the first of
// three or more segments has a dot):
//   - `owner/repo@v1.0.0` is that tag's source (`/source` says the same);
//   - `owner/repo/release@v1.0.0` is its uploaded release asset, `:<glob>` picking one of
//     several (git forbids `:` in tags);
//   - `@latest` pins the newest non-prerelease when recorded; no `@ref` pins nothing;
//   - `owner/repo/clone@main` is a live checkout, Tag being a branch (none means the remote
//     default). Only a typed install may ask for one (see ParseRepoSpec).
//
// RepoURL is the canonical url; RepoID its host/owner/repo form for matching entries.
// Every field is comparable.
type Dependency struct {
	Host         string
	Owner        string
	Repo         string
	Tag          string
	Kind         Kind
	Release      bool   // `/release`: the tag's uploaded asset rather than its source
	AssetPattern string // `/release@tag:<glob>`: which upload, when there are several
	RepoURL      string
	RepoID       string
}

// IsClone reports whether the spec asked for a live git checkout rather than a package,
// mirroring Addon.IsClone. For one of these Tag names a branch, not a release.
func (d Dependency) IsClone() bool { return d.Kind == KindClone }

// WantsLatest reports whether the ref is the reserved `latest` (see ResolveDepAsset).
func (d Dependency) WantsLatest() bool { return IsLatestTag(d.Tag) }

// Dependencies reads what an installed addon's config declares. Missing config or an empty
// key yields nil. `require` aliases `deps` and wins when both are present, even if empty.
func Dependencies(addonDir string) ([]Dependency, error) {
	cfgPath := pluginCfgPath(addonDir)
	if cfgPath == "" {
		return nil, nil
	}
	cfg, err := ini.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", cfgPath, err)
	}
	plugin := cfgSection(cfg)
	key := "deps"
	if plugin.HasKey("require") {
		key = "require"
	}
	raw := plugin.Key(key).String()
	return parseDependencyList(raw), nil
}

// parseDependencyList parses a Godot bracketed, comma-separated, optionally quoted list of
// specs, skipping malformed items. A declared `/clone` is skipped too: a build can't read a
// branch, so PackageExporter skips it and the two tools must agree on what a cfg requires.
func parseDependencyList(raw string) []Dependency {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var deps []Dependency
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(strings.Trim(strings.TrimSpace(item), `"'`))
		if item == "" {
			continue
		}
		if d, ok := parseDependency(item, false); ok {
			deps = append(deps, d)
		}
	}
	return deps
}

// ParseRepoSpec parses a spec typed at the CLI into the Dependency a plugin.cfg item yields,
// so typed and declared specs agree; it alone also accepts `/clone`, since cloning is a
// development choice made at install time.
func ParseRepoSpec(spec string) (Dependency, bool) {
	return parseDependency(strings.TrimSpace(spec), true)
}

// Spec markers: the optional third path segment, after owner/repo.
const (
	sourceMarker  = "source"
	releaseMarker = "release"
	cloneMarker   = "clone"
)

// legacyClonePrefix is the old `clone:owner/repo` spelling. It is read as plain source, as
// PackageExporter reads it, so an old cfg means the same thing to both tools.
const legacyClonePrefix = "clone:"

func parseDependency(item string, allowClone bool) (Dependency, bool) {
	item = strings.TrimPrefix(item, legacyClonePrefix)
	// An `@ref` suffix is optional: with it the dependency names a tag (or, for a clone, a
	// branch); without it the repo is added version-less / on the default branch.
	repoPart, tag, pattern := item, "", ""
	if at := strings.LastIndex(item, "@"); at >= 0 {
		repoPart, tag = item[:at], strings.TrimSpace(item[at+1:])
		if colon := strings.Index(tag, ":"); colon >= 0 {
			tag, pattern = strings.TrimSpace(tag[:colon]), strings.TrimSpace(tag[colon+1:])
			if pattern == "" {
				return Dependency{}, false
			}
		}
	}
	host, owner, repo, marker, ok := parseRepoPath(repoPart)
	if !ok {
		return Dependency{}, false
	}
	d := Dependency{Host: host, Owner: owner, Repo: repo, Tag: tag, AssetPattern: pattern}
	switch marker {
	case "", sourceMarker:
	case releaseMarker:
		d.Release = true
	case cloneMarker:
		// `latest` is a release concept and a clone checks out a branch, so the combination
		// asks for two different things at once and is rejected rather than guessed at.
		if !allowClone || IsLatestTag(tag) {
			return Dependency{}, false
		}
		d.Kind = KindClone
	default:
		return Dependency{}, false
	}
	if pattern != "" && (!d.Release || tag == "") {
		return Dependency{}, false
	}
	d.RepoURL = fmt.Sprintf("https://%s/%s/%s", host, owner, repo)
	id, err := source.RepoID(d.RepoURL)
	if err != nil {
		return Dependency{}, false
	}
	d.RepoID = id
	return d, true
}

// parseRepoPath splits `[host/]owner/repo[/marker]`. The first segment is a host only when
// three or more remain and it has a dot, which keeps `owner/repo/release` apart from
// `host/owner/repo`; a colon is allowed only there (a port).
func parseRepoPath(s string) (host, owner, repo, marker string, ok bool) {
	parts := strings.Split(strings.Trim(strings.TrimSpace(s), "/"), "/")
	host = defaultDepHost
	if len(parts) >= 3 && strings.Contains(parts[0], ".") {
		host, parts = parts[0], parts[1:]
	}
	if len(parts) == 3 {
		marker, parts = parts[2], parts[:2]
	}
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.Contains(parts[0]+parts[1], ":") {
		return "", "", "", "", false
	}
	return host, parts[0], parts[1], marker, true
}

// parseRepoShorthand parses owner/repo or host/owner/repo (no marker) and returns the
// canonical https url; ok is false otherwise.
func parseRepoShorthand(s string) (host, owner, repo, url string, ok bool) {
	host, owner, repo, marker, ok := parseRepoPath(s)
	if !ok || marker != "" {
		return "", "", "", "", false
	}
	return host, owner, repo, fmt.Sprintf("https://%s/%s/%s", host, owner, repo), true
}

// MissingDeps returns, in declaration order, a's declared dependencies that the manifest
// lacks or holds at a verifiably older tag: what "Add all missing" adds. Tagless deps are
// satisfied by any entry, uncomparable tags are trusted, suppressed deps are excluded. It
// is manifest-only and cheap; DepStatuses is the install-aware form. It uses the shared
// depIndex and depSatisfied, but keeps declaration order (published by `list --json`).
func MissingDeps(a Addon, projectRoot string, manifest []Addon) ([]Dependency, error) {
	deps, err := declaredDeps(a, projectRoot)
	if err != nil || len(deps) == 0 {
		return nil, err
	}

	ix := newDepIndex(manifest)
	suppressed := stringSet(a.SuppressDeps)

	var missing []Dependency
	for _, d := range deps {
		if suppressed[d.RepoID] {
			continue
		}
		if i := ix.find(d); i < 0 || !depSatisfied(d, manifest[i]) {
			missing = append(missing, d)
		}
	}
	return missing, nil
}

// OrphanDeps reports is_dependency entries no installed plugin requires any more, keyed by
// Name (only orphans present). The requirement graph comes from installed plugins' configs,
// so uninstalling a depender flags its dependency. Local-only.
func OrphanDeps(statuses []Status) map[string]bool {
	needed := make(map[string]bool)
	for _, s := range statuses {
		if !s.Present() {
			continue
		}
		deps, err := Dependencies(s.FullPath)
		if err != nil || len(deps) == 0 {
			continue
		}
		suppressed := stringSet(s.Addon.SuppressDeps)
		for _, d := range deps {
			if !suppressed[d.RepoID] {
				needed[d.RepoID] = true
			}
		}
	}

	orphan := make(map[string]bool)
	for _, s := range statuses {
		if !s.Addon.Dependency {
			continue
		}
		id, err := source.RepoID(s.Addon.URL)
		if err != nil {
			continue
		}
		if !needed[id] {
			orphan[s.Addon.Name] = true
		}
	}
	return orphan
}

// DepState is a declared dependency's install state relative to the project, as shown
// on the Dependencies screen.
type DepState int

const (
	DepInstalled    DepState = iota // present on disk and satisfying (or its tag is unverifiable → trusted)
	DepMissing                      // no manifest entry (the addable set)
	DepNotInstalled                 // manifest entry exists but nothing on disk yet
	DepOutdated                     // installed but the entry's tag is verifiably older than required
)

// String renders the state as a stable snake_case label (published by `list --json`).
func (s DepState) String() string {
	switch s {
	case DepInstalled:
		return "installed"
	case DepNotInstalled:
		return "not_installed"
	case DepOutdated:
		return "outdated"
	default:
		return "missing"
	}
}

// DepStatus is one declared dependency paired with its resolved install state and
// whether the user has suppressed it — the row model for the Dependencies screen.
type DepStatus struct {
	Dep        Dependency
	State      DepState
	Suppressed bool
	LocalTag   string // the matched manifest entry's tag, for display ("" when none)
}

// NeedsAttention reports an unsuppressed dependency that is not installed and satisfying:
// the one rule behind the TUI's "missing deps" marker and `list --json`'s missing_deps.
func (ds DepStatus) NeedsAttention() bool {
	return !ds.Suppressed && ds.State != DepInstalled
}

// DepStatuses returns the install state of each dependency a declares, matched (via
// depIndex, like MissingDeps) against the inspected statuses so it reflects what is on
// disk. It backs the Dependencies screen and the missing-deps warning. Local-only.
func DepStatuses(a Addon, projectRoot string, statuses []Status) ([]DepStatus, error) {
	deps, err := declaredDeps(a, projectRoot)
	if err != nil || len(deps) == 0 {
		return nil, err
	}

	ix := newDepIndex(addonsOf(statuses))
	suppressed := stringSet(a.SuppressDeps)

	out := make([]DepStatus, 0, len(deps))
	for _, d := range deps {
		ds := DepStatus{Dep: d, Suppressed: suppressed[d.RepoID]}
		i := ix.find(d)
		switch {
		case i < 0:
			ds.State = DepMissing
		case !statuses[i].Present():
			ds.LocalTag = statuses[i].Addon.Tag
			ds.State = DepNotInstalled
		default:
			ds.LocalTag = statuses[i].Addon.Tag
			if depSatisfied(d, statuses[i].Addon) {
				ds.State = DepInstalled // satisfied, tagless, or unverifiable tag → trusted
			} else {
				ds.State = DepOutdated
			}
		}
		out = append(out, ds)
	}
	return out, nil
}

// stringSet builds a lookup set from a slice.
func stringSet(ss []string) map[string]bool {
	set := make(map[string]bool, len(ss))
	for _, s := range ss {
		set[s] = true
	}
	return set
}
