// Package source lists an addon's available versions from its remote, driven by vcs rules
// in sources.yml keyed by host (github.com and codeberg.org by default). A host without a
// rule degrades to a single git-clone option.
package source

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/brohd11/gdaddon/internal/config"
	"github.com/brohd11/gdaddon/internal/restrule"
)

// Asset is one downloadable file (a .zip, or a .git clone fallback). Generated marks the
// host's auto-generated source archive, so callers can prefer uploaded builds.
type Asset struct {
	Name      string
	URL       string
	Generated bool
	Commit    string // resolved HEAD sha for a branch asset pinned via CommitArchiveURL ("" = floating/unpinned)
}

// Release is a selectable version: a tag plus its downloadable assets.
type Release struct {
	Tag        string
	Prerelease bool
	Assets     []Asset
}

// Listing is everything selectable for a url: releases (semantic descending, then other
// tags) and, for a branch url, a branch-HEAD option.
type Listing struct {
	Owner    string
	Repo     string
	Branch   *Release // branch-HEAD archive, if the URL pointed at refs/heads/<branch>
	Releases []Release
}

// ruleForHost returns the vcs rule for host from the configured providers (or defaults);
// false when none claims it.
func ruleForHost(host string) (*config.VCSRule, bool) {
	for _, s := range config.Sources() {
		if s.VCS != nil && strings.EqualFold(s.VCS.Host, host) {
			return s.VCS, true
		}
	}
	return nil, false
}

// AvailableVersions lists a repo url's versions via its host rule, or a single git-clone
// fallback.
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

// Branches lists branches as branch-HEAD archive assets (nil without a rule), fetched only
// when requested.
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

// RepoID is a repo url's canonical identity, "<host>/<owner>/<repo>" lowercased, whatever
// form the url took. Used to match entries and name archive folders.
func RepoID(rawURL string) (string, error) {
	ref, err := parseRepoURL(rawURL)
	if err != nil {
		return "", err
	}
	return strings.ToLower(ref.Host + "/" + ref.Owner + "/" + ref.Repo), nil
}

// RepoURL reduces any git-host url to "https://<host>/<owner>/<repo>", for recording in the
// global list.
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
	endpoint := restrule.Render(rule.Releases.URL, vars(owner, repo, "", ""))
	var releases []Release
	seenPages, seenTags := map[string]bool{}, map[string]bool{}
	for endpoint != "" {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if seenPages[endpoint] {
			return nil, fmt.Errorf("repeated release page: %s", endpoint)
		}
		seenPages[endpoint] = true
		page, next, err := resolveReleasesPage(ctx, rule, owner, repo, endpoint)
		if err != nil {
			return nil, err
		}
		for _, rel := range page {
			if !seenTags[rel.Tag] {
				seenTags[rel.Tag] = true
				releases = append(releases, rel)
			}
		}
		endpoint = next
	}
	SortReleases(releases)
	return releases, nil
}

func resolveReleasesPage(ctx context.Context, rule *config.VCSRule, owner, repo, endpoint string) ([]Release, string, error) {
	r := rule.Releases

	var root any
	next, err := restrule.GetJSONPage(ctx, endpoint, &root)
	if err != nil {
		return nil, "", err
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
		rel.Prerelease = rel.IsPrerelease()

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
				URL:       restrule.Render(rule.SourceArchive.URL, vars(owner, repo, url.PathEscape(tag), "")),
				Generated: true,
			})
		}
		releases = append(releases, rel)
	}
	return releases, next, nil
}

func resolveBranches(ctx context.Context, rule *config.VCSRule, owner, repo string) ([]Asset, error) {
	b := rule.Branches
	endpoint := restrule.Render(b.URL, vars(owner, repo, "", ""))

	var root any
	if err := restrule.GetJSON(ctx, endpoint, &root); err != nil {
		return nil, err
	}
	arr, _ := restrule.GetPath(root, b.ResultsPath)
	raw, _ := arr.([]any)

	branches := make([]Asset, 0, len(raw))
	for _, el := range raw {
		name := restrule.GetPathString(el, b.NamePath)
		asset := Asset{
			Name: name,
			URL:  restrule.Render(b.ArchiveURL, vars(owner, repo, "", name)),
		}
		// Pin to the branch's HEAD commit when possible, for a reproducible install; otherwise use
		// the floating branch archive.
		if sha := restrule.GetPathString(el, b.CommitPath); sha != "" && rule.CommitArchiveURL != "" {
			v := vars(owner, repo, "", name)
			v["commit"] = sha
			asset.URL = restrule.Render(rule.CommitArchiveURL, v)
			asset.Commit = sha
		}
		branches = append(branches, asset)
	}
	return branches, nil
}

// AutoAsset picks an asset without a user: exactly one uploaded asset is installed (e.g. a
// precompiled GDExtension), none means the generated source archive, and two or more is
// ambiguous (ok=false: skip, or show a picker).
func AutoAsset(rel Release) (Asset, bool) {
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
