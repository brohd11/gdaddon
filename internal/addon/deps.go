package addon

import (
	"fmt"
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

// ParseRepoSpec parses an `owner/repo[@tag]` (or `host/owner/repo[@tag]`) spec into
// the same Dependency shape a plugin.cfg `deps` item yields — the host defaults to
// github.com and RepoURL/RepoID come out canonical. Exported for the CLI's
// `gdaddon install <owner/repo>` argument, which deliberately shares this parser so a
// hand-typed spec and a declared dependency can never diverge.
func ParseRepoSpec(spec string) (Dependency, bool) {
	return parseDependency(strings.TrimSpace(spec))
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
//
// It is the "needs recording" half of PlanDeps' classification — no entry at all, or an
// entry verifiably behind — but it walks the deps itself rather than concatenating that
// function's buckets, because the result is returned in *declaration* order and
// `list --json` publishes it as an ordered array. Matching and the tag rule are still the
// shared ones (depIndex, depSatisfied — see depmatch.go), which is what keeps this in step
// with the other readers; only the grouping differs.
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
		if i := ix.find(d); i < 0 || !depSatisfied(d, manifest[i].Tag) {
			missing = append(missing, d)
		}
	}
	return missing, nil
}

// OrphanDeps reports which is_dependency-flagged manifest entries are no longer required
// by any installed plugin — the "unused dependency" markers. It builds the union of every
// non-suppressed dependency RepoID declared by a present plugin's plugin.cfg, then flags
// each entry with Dependency==true whose own RepoID is absent from that union. Keyed by
// addon Name; only orphans appear (a missing key reads as not-orphaned).
//
// The graph is read from installed plugins only (an uninstalled plugin's plugin.cfg isn't
// on disk), so removing/uninstalling a depender flags its dep here — intended, and a
// stateless self-healing recompute. Local-only, so it rides every refresh like DepStatuses.
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
// Matching is depIndex's (see depmatch.go) — the same lookup MissingDeps uses, so the
// screen and the "Add all missing" set can never disagree about what is present — over the
// statuses' own entries, so a hit indexes straight back into statuses for the on-disk half.
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
			if depSatisfied(d, statuses[i].Addon.Tag) {
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
