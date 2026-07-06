package addon

import (
	"fmt"
	"path/filepath"
	"strings"

	"gdaddon/internal/source"

	"gopkg.in/ini.v1"
)

// defaultDepHost is assumed when a dependency item names only owner/repo.
const defaultDepHost = "github.com"

// Dependency is one parsed entry of an addon's plugin.cfg `dependencies` list:
// `owner/repo@tag` or `host/owner/repo@tag` (Tag set), or a tagless `owner/repo`
// (Tag empty — the release is unambiguous / no version pinned, so the repo is added
// version-less). RepoURL is the canonical repo url used to list versions and resolve
// an asset; RepoID is its source.RepoID form (host/owner/repo, lowercased) for
// matching against installed manifest entries.
type Dependency struct {
	Host    string
	Owner   string
	Repo    string
	Tag     string
	RepoURL string
	RepoID  string
}

// Dependencies reads the dependencies an installed addon declares in its
// plugin.cfg/version.cfg under addonDir. A missing config or absent/empty
// `deps` key yields nil with no error.
func Dependencies(addonDir string) ([]Dependency, error) {
	cfgPath := pluginCfgPath(addonDir)
	if cfgPath == "" {
		return nil, nil
	}
	cfg, err := ini.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", cfgPath, err)
	}
	raw := cfg.Section("plugin").Key("deps").String()
	return parseDependencyList(raw), nil
}

// parseDependencyList parses a Godot-style bracketed, comma-separated,
// optionally-quoted list of `owner/repo@tag` items. Malformed items (missing @tag
// or owner/repo) are skipped rather than failing the whole parse.
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
		if d, ok := parseDependency(item); ok {
			deps = append(deps, d)
		}
	}
	return deps
}

func parseDependency(item string) (Dependency, bool) {
	// An `@tag` suffix is optional: with it the dependency is version-pinned; without
	// it the repo is added version-less (the unambiguous case).
	repoPart, tag := item, ""
	if at := strings.LastIndex(item, "@"); at >= 0 {
		repoPart, tag = item[:at], strings.TrimSpace(item[at+1:])
	}
	host, owner, repo, repoURL, ok := parseRepoShorthand(repoPart)
	if !ok {
		return Dependency{}, false
	}
	id, err := source.RepoID(repoURL)
	if err != nil {
		return Dependency{}, false
	}
	return Dependency{Host: host, Owner: owner, Repo: repo, Tag: tag, RepoURL: repoURL, RepoID: id}, true
}

// parseRepoShorthand splits owner/repo or host/owner/repo shorthand, defaulting the
// host to defaultDepHost (github.com) for the 2-part form, and returns the canonical
// https url. ok is false for any other shape or an empty owner/repo.
func parseRepoShorthand(s string) (host, owner, repo, url string, ok bool) {
	switch parts := strings.Split(strings.Trim(s, "/"), "/"); len(parts) {
	case 2:
		host, owner, repo = defaultDepHost, parts[0], parts[1]
	case 3:
		host, owner, repo = parts[0], parts[1], parts[2]
	default:
		return "", "", "", "", false
	}
	if owner == "" || repo == "" {
		return "", "", "", "", false
	}
	return host, owner, repo, fmt.Sprintf("https://%s/%s/%s", host, owner, repo), true
}

// MissingDeps returns the dependencies addon a declares (in its installed
// plugin.cfg under projectRoot) that the manifest does not yet contain: a dep whose
// repo has no manifest entry, or a tagged dep whose existing entry's tag is
// verifiably older — the set "Add all missing" would add. A tagless dep is satisfied
// by any present entry, and a present entry with a non-comparable tag (a date stamp,
// or a branch-HEAD install with no tag) is left alone — "can't verify", not flagged —
// so deliberate HEAD-tracking isn't nagged. Deps the user has suppressed (a.SuppressDeps)
// are excluded. A not-installed addon (no path / no plugin.cfg) declares nothing.
// It is local-only (no network), so it's cheap enough to recompute on every refresh.
//
// Note this is manifest-presence only (not on-disk state); DepStatuses is the
// install-aware form used by the Dependencies screen and the missing-deps warning.
func MissingDeps(a Addon, projectRoot string, manifest []Addon) ([]Dependency, error) {
	if a.Path == "" {
		return nil, nil
	}
	deps, err := Dependencies(filepath.Join(projectRoot, a.Path))
	if err != nil || len(deps) == 0 {
		return nil, err
	}

	byRepo := IndexByRepo(manifest)
	suppressed := stringSet(a.SuppressDeps)

	var missing []Dependency
	for _, d := range deps {
		if suppressed[d.RepoID] {
			continue
		}
		e, present := byRepo[d.RepoID]
		switch {
		case !present:
			missing = append(missing, d)
		case d.Tag == "":
			// tagless: presence satisfies it.
		default:
			if sat, verified := d.SatisfiedByTag(e.Tag); verified && !sat {
				missing = append(missing, d)
			}
		}
	}
	return missing, nil
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

// DepStatus is one declared dependency paired with its resolved install state and
// whether the user has suppressed it — the row model for the Dependencies screen.
type DepStatus struct {
	Dep        Dependency
	State      DepState
	Suppressed bool
	LocalTag   string // the matched manifest entry's tag, for display ("" when none)
}

// DepStatuses returns the install state of every dependency addon a declares, matched
// against the freshly inspected project statuses (so it knows what is actually on disk,
// unlike MissingDeps which is manifest-presence only). It backs the Dependencies screen
// and — via the "needs attention" subset (unsuppressed && not DepInstalled) — the
// missing-deps warning. Local-only. A not-installed addon declares nothing.
func DepStatuses(a Addon, projectRoot string, statuses []Status) ([]DepStatus, error) {
	if a.Path == "" {
		return nil, nil
	}
	deps, err := Dependencies(filepath.Join(projectRoot, a.Path))
	if err != nil || len(deps) == 0 {
		return nil, err
	}

	byRepo := statusesByRepo(statuses)
	suppressed := stringSet(a.SuppressDeps)

	out := make([]DepStatus, 0, len(deps))
	for _, d := range deps {
		ds := DepStatus{Dep: d, Suppressed: suppressed[d.RepoID]}
		st, present := byRepo[d.RepoID]
		switch {
		case !present:
			ds.State = DepMissing
		case !st.Present():
			ds.LocalTag = st.Addon.Tag
			ds.State = DepNotInstalled
		default:
			ds.LocalTag = st.Addon.Tag
			if d.Tag == "" {
				ds.State = DepInstalled // tagless: presence satisfies it
			} else if sat, verified := d.SatisfiedByTag(st.Addon.Tag); verified && !sat {
				ds.State = DepOutdated
			} else {
				ds.State = DepInstalled // satisfied, or unverifiable tag → trusted
			}
		}
		out = append(out, ds)
	}
	return out, nil
}

// statusesByRepo indexes inspected statuses by canonical repo identity (source.RepoID
// of the entry url); entries with an unparseable url are skipped, later duplicates win.
func statusesByRepo(statuses []Status) map[string]Status {
	byRepo := make(map[string]Status, len(statuses))
	for _, s := range statuses {
		id, err := source.RepoID(s.Addon.URL)
		if err != nil {
			continue
		}
		byRepo[id] = s
	}
	return byRepo
}

// stringSet builds a lookup set from a slice.
func stringSet(ss []string) map[string]bool {
	set := make(map[string]bool, len(ss))
	for _, s := range ss {
		set[s] = true
	}
	return set
}
