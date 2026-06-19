// Package source resolves the available versions of an addon from its remote.
// It is config-driven: each provider entry in ~/.gdaddon/config.yml may carry a
// vcs rule (config.VCSRule) keyed by host, describing the host's release/branch
// API and archive-URL patterns. github.com and codeberg.org ship as defaults;
// any host can be added in YAML. A host with no rule degrades to a single
// git-clone option so install still works. The Listing/Release/Asset shapes are
// host-agnostic.
package source

import (
	"context"
	"fmt"
	"strings"

	"gdaddon/internal/config"
	"gdaddon/internal/restrule"
)

// Asset is one downloadable file (a .zip archive, or a .git clone URL fallback).
// Generated marks the host's auto-generated source archive (appended to every
// release in resolveReleases) as opposed to an asset the author uploaded — so
// callers can prefer the uploaded build without relying on asset ordering.
type Asset struct {
	Name      string
	URL       string
	Generated bool
}

// Release is a selectable version: a tag plus its downloadable assets.
type Release struct {
	Tag        string
	Prerelease bool
	Assets     []Asset
}

// Listing is everything selectable for a manifest URL: the repo's releases
// (newest first) and, when the URL tracked a branch, a branch-HEAD option.
type Listing struct {
	Owner    string
	Repo     string
	Branch   *Release // branch-HEAD archive, if the URL pointed at refs/heads/<branch>
	Releases []Release
}

// ruleForHost returns the vcs rule whose Host matches, scanning the config
// providers (falling back to the built-in defaults when the file is missing or
// empty, mirroring search.Sources). The second result is false when no provider
// claims the host.
func ruleForHost(host string) (*config.VCSRule, bool) {
	sources := config.DefaultSources()
	if cfg, err := config.Load(); err == nil && len(cfg.Sources) > 0 {
		sources = cfg.Sources
	}
	for _, s := range sources {
		if s.VCS != nil && strings.EqualFold(s.VCS.Host, host) {
			return s.VCS, true
		}
	}
	return nil, false
}

// AvailableVersions parses a repo URL and fetches its versions via the matching
// host rule. A host with no rule yields a single git-clone fallback so install
// still works.
func AvailableVersions(ctx context.Context, rawURL string) (*Listing, error) {
	ref, err := parseRepoURL(rawURL)
	if err != nil {
		return nil, err
	}

	rule, ok := ruleForHost(ref.Host)
	if !ok {
		return cloneFallback(ref), nil
	}

	releases, err := resolveReleases(ctx, rule, ref.Owner, ref.Repo)
	if err != nil {
		return nil, err
	}
	listing := &Listing{Owner: ref.Owner, Repo: ref.Repo, Releases: releases}

	if ref.Branch != "" && rule.BranchArchiveURL != "" {
		url := restrule.Render(rule.BranchArchiveURL, vars(ref.Owner, ref.Repo, "", ref.Branch))
		listing.Branch = &Release{Tag: ref.Branch, Assets: []Asset{{Name: ref.Branch + ".zip", URL: url}}}
	}
	return listing, nil
}

// Branches lists the repo's branches as branch-HEAD archive assets. A host with
// no branch rule (or no rule at all) returns nil. Fetched lazily (only when the
// user opens HEAD) to avoid an extra API call on every version listing.
func Branches(ctx context.Context, rawURL string) ([]Asset, error) {
	ref, err := parseRepoURL(rawURL)
	if err != nil {
		return nil, err
	}
	rule, ok := ruleForHost(ref.Host)
	if !ok || rule.Branches.URL == "" {
		return nil, nil
	}
	return resolveBranches(ctx, rule, ref.Owner, ref.Repo)
}

// RepoID is the canonical identity of a repo URL — "<host>/<owner>/<repo>",
// lowercased — independent of which form the URL took (.git, a release-download
// asset, or an archive/refs URL). Used to detect that two manifest entries point
// at the same repository and to name archive folders. Host-agnostic.
func RepoID(rawURL string) (string, error) {
	ref, err := parseRepoURL(rawURL)
	if err != nil {
		return "", err
	}
	return strings.ToLower(ref.Host + "/" + ref.Owner + "/" + ref.Repo), nil
}

// RepoURL strips any standard git-host URL (a .git clone URL, a release-download
// asset, an archive/refs URL, …) down to its canonical repo-level form
// "https://<host>/<owner>/<repo>". Used to record a clean, version-agnostic url in
// the global list instead of the project entry's pinned release/archive url.
func RepoURL(rawURL string) (string, error) {
	ref, err := parseRepoURL(rawURL)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://%s/%s/%s", ref.Host, ref.Owner, ref.Repo), nil
}

// vars builds the placeholder set for a vcs rule's URL templates.
func vars(owner, repo, tag, branch string) map[string]string {
	return map[string]string{"owner": owner, "repo": repo, "tag": tag, "branch": branch}
}

// cloneFallback is the listing for a host with no vcs rule: a single option that
// git-clones the repo's default branch (the installer's fetchGit handles .git).
func cloneFallback(ref repoRef) *Listing {
	gitURL := fmt.Sprintf("https://%s/%s/%s.git", ref.Host, ref.Owner, ref.Repo)
	return &Listing{
		Owner: ref.Owner,
		Repo:  ref.Repo,
		Releases: []Release{{
			Tag:    "default branch",
			Assets: []Asset{{Name: "git clone", URL: gitURL}},
		}},
	}
}

func resolveReleases(ctx context.Context, rule *config.VCSRule, owner, repo string) ([]Release, error) {
	r := rule.Releases
	endpoint := restrule.Render(r.URL, vars(owner, repo, "", ""))

	var root any
	if err := restrule.GetJSON(ctx, endpoint, rule.Auth, &root); err != nil {
		return nil, err
	}
	arr, _ := restrule.GetPath(root, r.ResultsPath)
	raw, _ := arr.([]any)

	suffix := r.AssetSuffix
	if suffix == "" {
		suffix = ".zip"
	}

	releases := make([]Release, 0, len(raw))
	for _, el := range raw {
		tag := restrule.GetPathString(el, r.TagPath)
		rel := Release{Tag: tag, Prerelease: restrule.GetPathBool(el, r.PrereleasePath)}

		if assets, ok := restrule.GetPath(el, r.AssetsPath); ok {
			for _, a := range asSlice(assets) {
				name := restrule.GetPathString(a, r.AssetNamePath)
				// The installer only handles .zip; hide .tgz / platform binaries etc.
				if !strings.HasSuffix(strings.ToLower(name), suffix) {
					continue
				}
				rel.Assets = append(rel.Assets, Asset{Name: name, URL: restrule.GetPathString(a, r.AssetURLPath)})
			}
		}
		// Every release also offers the host's generated source archive, appended
		// last. For releases with no uploaded .zip it's the only option.
		if rule.SourceArchive.URL != "" {
			rel.Assets = append(rel.Assets, Asset{
				Name:      rule.SourceArchive.Name,
				URL:       restrule.Render(rule.SourceArchive.URL, vars(owner, repo, tag, "")),
				Generated: true,
			})
		}
		releases = append(releases, rel)
	}
	return releases, nil
}

func resolveBranches(ctx context.Context, rule *config.VCSRule, owner, repo string) ([]Asset, error) {
	b := rule.Branches
	endpoint := restrule.Render(b.URL, vars(owner, repo, "", ""))

	var root any
	if err := restrule.GetJSON(ctx, endpoint, rule.Auth, &root); err != nil {
		return nil, err
	}
	arr, _ := restrule.GetPath(root, b.ResultsPath)
	raw, _ := arr.([]any)

	branches := make([]Asset, 0, len(raw))
	for _, el := range raw {
		name := restrule.GetPathString(el, b.NamePath)
		branches = append(branches, Asset{
			Name: name,
			URL:  restrule.Render(b.ArchiveURL, vars(owner, repo, "", name)),
		})
	}
	return branches, nil
}

// DependencyAsset picks the asset to install for a tagged dependency, where no
// user is present to choose. Pinning a tagged dependency asserts the release is
// unambiguous: exactly one uploaded asset → install it (e.g. a GDExtension addon's
// precompiled build, where the generated source archive is useless); no uploaded
// asset → the generated source archive (a pure-GDScript addon); two or more uploaded
// assets → ambiguous (ok=false), so the caller reports and skips it.
func DependencyAsset(rel Release) (Asset, bool) {
	var uploaded []Asset
	var generated *Asset
	for i := range rel.Assets {
		if rel.Assets[i].Generated {
			generated = &rel.Assets[i]
		} else {
			uploaded = append(uploaded, rel.Assets[i])
		}
	}
	switch {
	case len(uploaded) == 1:
		return uploaded[0], true
	case len(uploaded) == 0 && generated != nil:
		return *generated, true
	default:
		return Asset{}, false
	}
}

// asSlice coerces a decoded JSON value to a slice (nil when it isn't one).
func asSlice(v any) []any {
	s, _ := v.([]any)
	return s
}
