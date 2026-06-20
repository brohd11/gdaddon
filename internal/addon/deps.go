package addon

import (
	"fmt"
	"path/filepath"
	"strconv"
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
	repoPart = strings.Trim(strings.TrimSpace(repoPart), "/")
	if repoPart == "" {
		return Dependency{}, false
	}

	var host, owner, repo string
	switch parts := strings.Split(repoPart, "/"); len(parts) {
	case 2:
		host, owner, repo = defaultDepHost, parts[0], parts[1]
	case 3:
		host, owner, repo = parts[0], parts[1], parts[2]
	default:
		return Dependency{}, false
	}
	if owner == "" || repo == "" {
		return Dependency{}, false
	}

	repoURL := fmt.Sprintf("https://%s/%s/%s", host, owner, repo)
	id, err := source.RepoID(repoURL)
	if err != nil {
		return Dependency{}, false
	}
	return Dependency{Host: host, Owner: owner, Repo: repo, Tag: tag, RepoURL: repoURL, RepoID: id}, true
}

// MissingDeps returns the dependencies addon a declares (in its installed
// plugin.cfg under projectRoot) that the manifest does not satisfy: a dep whose
// repo has no manifest entry, or a tagged dep whose existing entry's tag is
// verifiably older. A tagless dep is satisfied by any present copy, and a present
// entry with a non-comparable tag (a date stamp, or a branch-HEAD install with no
// tag) is left alone — "can't verify", not flagged — so deliberate HEAD-tracking
// isn't nagged. A not-installed addon (no path / no plugin.cfg) declares nothing.
// It is local-only (no network), so it's cheap enough to recompute on every refresh.
func MissingDeps(a Addon, projectRoot string, manifest []Addon) ([]Dependency, error) {
	if a.Path == "" {
		return nil, nil
	}
	deps, err := Dependencies(filepath.Join(projectRoot, a.Path))
	if err != nil || len(deps) == 0 {
		return nil, err
	}

	byRepo := make(map[string]Addon, len(manifest))
	for _, e := range manifest {
		if id, err := source.RepoID(e.URL); err == nil {
			byRepo[id] = e
		}
	}

	var missing []Dependency
	for _, d := range deps {
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

// SatisfiedByTag reports whether an installed entry on installedTag meets this
// dependency's required tag. verified is false when either tag isn't a comparable
// dotted-numeric version (a date stamp, a branch-HEAD entry with no tag, …), so the
// caller can surface it as "can't verify" rather than a definite miss.
func (d Dependency) SatisfiedByTag(installedTag string) (satisfied, verified bool) {
	ge, ok := semverGE(installedTag, d.Tag)
	if !ok {
		return false, false
	}
	return ge, true
}

// semverGE reports whether version a is >= version b, treating both as dotted
// numeric versions. A leading "v" and any pre-release/build suffix (after "-"/"+")
// are ignored. ok is false when either side has no comparable numeric components.
func semverGE(a, b string) (ge, ok bool) {
	na, oka := numericParts(a)
	nb, okb := numericParts(b)
	if !oka || !okb {
		return false, false
	}
	for i := 0; i < len(na) || i < len(nb); i++ {
		var x, y int
		if i < len(na) {
			x = na[i]
		}
		if i < len(nb) {
			y = nb[i]
		}
		if x != y {
			return x > y, true
		}
	}
	return true, true
}

func numericParts(v string) ([]int, bool) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V")
	// Strip a semver pre-release/build suffix, but only when the part before it
	// looks like a dotted version — so a date stamp like "2024-01-02" stays
	// non-numeric (uncomparable) rather than truncating to its first field.
	if i := strings.IndexAny(v, "-+"); i >= 0 && strings.Contains(v[:i], ".") {
		v = v[:i]
	}
	if v == "" {
		return nil, false
	}
	parts := strings.Split(v, ".")
	nums := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return nil, false
		}
		nums = append(nums, n)
	}
	return nums, true
}
