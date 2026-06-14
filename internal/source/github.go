// Package source resolves the available versions of an addon from its remote.
// Today it understands GitHub (release assets and branch/tag archives); the
// Listing/Release/Asset shapes are deliberately host-agnostic so other sources
// can be added behind AvailableVersions later.
package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Asset is one downloadable file (always a .zip for our purposes).
type Asset struct {
	Name string
	URL  string
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
	Owner  string
	Repo   string
	Branch *Release // branch-HEAD archive, if the URL pointed at refs/heads/<branch>
	Releases []Release
}

// repoRef is the parsed github.com coordinates of a manifest URL.
type repoRef struct {
	Owner  string
	Repo   string
	Branch string // non-empty if the URL was a refs/heads archive
}

// AvailableVersions parses a github.com URL and fetches its versions.
func AvailableVersions(ctx context.Context, rawURL string) (*Listing, error) {
	ref, err := parseGitHub(rawURL)
	if err != nil {
		return nil, err
	}

	releases, err := fetchReleases(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return nil, err
	}

	listing := &Listing{Owner: ref.Owner, Repo: ref.Repo, Releases: releases}

	if ref.Branch != "" {
		listing.Branch = &Release{
			Tag: ref.Branch,
			Assets: []Asset{{
				Name: ref.Branch + ".zip",
				URL:  fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", ref.Owner, ref.Repo, ref.Branch),
			}},
		}
	}

	return listing, nil
}

// Branches lists the repo's branches as branch-HEAD archive assets, newest
// commits tracked live. Fetched lazily (only when the user opens HEAD) to avoid
// spending an extra API call on every version listing.
func Branches(ctx context.Context, rawURL string) ([]Asset, error) {
	ref, err := parseGitHub(rawURL)
	if err != nil {
		return nil, err
	}
	return fetchBranches(ctx, ref.Owner, ref.Repo)
}

func parseGitHub(rawURL string) (repoRef, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return repoRef{}, fmt.Errorf("could not parse URL: %w", err)
	}
	host := strings.TrimPrefix(u.Host, "www.")
	if host != "github.com" {
		return repoRef{}, fmt.Errorf("unsupported host %q (only github.com is supported)", u.Host)
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 {
		return repoRef{}, fmt.Errorf("could not find owner/repo in %q", rawURL)
	}

	ref := repoRef{Owner: parts[0], Repo: strings.TrimSuffix(parts[1], ".git")}

	// .../archive/refs/heads/<branch>.zip → branch-tracking archive.
	if len(parts) >= 6 && parts[2] == "archive" && parts[3] == "refs" && parts[4] == "heads" {
		ref.Branch = strings.TrimSuffix(strings.Join(parts[5:], "/"), ".zip")
	}

	return ref, nil
}

// ghRelease mirrors the subset of the GitHub releases API we use.
type ghRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// ghGetJSON performs a GitHub API GET and decodes the JSON body into out,
// applying the shared headers, timeout, optional token, and rate-limit handling.
func ghGetJSON(ctx context.Context, endpoint string, out any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "gdutil")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("github API rate-limited (set GITHUB_TOKEN to raise the limit)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github API returned %s", resp.Status)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func fetchReleases(ctx context.Context, owner, repo string) ([]Release, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases?per_page=30", owner, repo)

	var raw []ghRelease
	if err := ghGetJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}

	releases := make([]Release, 0, len(raw))
	for _, r := range raw {
		rel := Release{Tag: r.TagName, Prerelease: r.Prerelease}
		for _, a := range r.Assets {
			// The installer only handles .zip; hide .tgz / platform binaries etc.
			if !strings.HasSuffix(strings.ToLower(a.Name), ".zip") {
				continue
			}
			rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL})
		}
		// Every release also offers GitHub's generated source archive, appended
		// last. For releases with no uploaded .zip it's the only option.
		rel.Assets = append(rel.Assets, Asset{
			Name: "Source code.zip",
			URL:  fmt.Sprintf("https://github.com/%s/%s/archive/refs/tags/%s.zip", owner, repo, r.TagName),
		})
		releases = append(releases, rel)
	}
	return releases, nil
}

func fetchBranches(ctx context.Context, owner, repo string) ([]Asset, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/branches?per_page=100", owner, repo)

	var raw []struct {
		Name string `json:"name"`
	}
	if err := ghGetJSON(ctx, endpoint, &raw); err != nil {
		return nil, err
	}

	branches := make([]Asset, 0, len(raw))
	for _, b := range raw {
		branches = append(branches, Asset{
			Name: b.Name,
			URL:  fmt.Sprintf("https://github.com/%s/%s/archive/refs/heads/%s.zip", owner, repo, b.Name),
		})
	}
	return branches, nil
}
