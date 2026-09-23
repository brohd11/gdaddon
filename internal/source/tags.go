package source

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/brohd11/gdaddon/internal/restrule"
)

// SupportsTags reports whether this host has a tag listing and source archive rule.
func SupportsTags(rawURL string) bool {
	ref, err := parseRepoURL(rawURL)
	if err != nil {
		return false
	}
	rule, ok := ruleForHost(ref.Host)
	return ok && rule.Tags.URL != "" && rule.SourceArchive.URL != ""
}

// Tags lists git tags as generated source packages, kept apart from AvailableVersions so
// unpublished tags never become update candidates.
func Tags(ctx context.Context, rawURL string) ([]Release, error) {
	ref, err := parseRepoURL(rawURL)
	if err != nil {
		return nil, err
	}
	rule, ok := ruleForHost(ref.Host)
	if !ok || rule.Tags.URL == "" || rule.SourceArchive.URL == "" {
		return nil, fmt.Errorf("tag source packages are not configured for %s", ref.Host)
	}
	endpoint := restrule.Render(rule.Tags.URL, vars(ref.Owner, ref.Repo, "", ""))
	var tags []Release
	seenPages, seenTags := map[string]bool{}, map[string]bool{}
	for endpoint != "" {
		if seenPages[endpoint] {
			return nil, fmt.Errorf("repeated tag page: %s", endpoint)
		}
		seenPages[endpoint] = true
		var root any
		next, err := restrule.GetJSONPage(ctx, endpoint, &root)
		if err != nil {
			return nil, err
		}
		arr, _ := restrule.GetPath(root, rule.Tags.ResultsPath)
		for _, el := range asSlice(arr) {
			tag := restrule.GetPathString(el, rule.Tags.NamePath)
			if tag == "" || seenTags[tag] {
				continue
			}
			seenTags[tag] = true
			tags = append(tags, Release{Tag: tag, Assets: []Asset{{
				Name:      rule.SourceArchive.Name,
				URL:       restrule.Render(rule.SourceArchive.URL, vars(ref.Owner, ref.Repo, url.PathEscape(tag), "")),
				Generated: true,
			}}})
		}
		endpoint = next
	}
	return tags, nil
}

// ResolveTag prefers a published release (searching past the first page), falling back to
// the tag's source package only when none matches. An API failure is an error, never a
// silent switch to source.
func ResolveTag(ctx context.Context, rawURL, tag string) (Release, error) {
	if tag == "" {
		return Release{}, fmt.Errorf("a tag is required")
	}
	ref, err := parseRepoURL(rawURL)
	if err != nil {
		return Release{}, err
	}
	rule, ok := ruleForHost(ref.Host)
	if !ok {
		return Release{}, fmt.Errorf("version lookup is not configured for %s", ref.Host)
	}
	endpoint := restrule.Render(rule.Releases.URL, vars(ref.Owner, ref.Repo, "", ""))
	seen := map[string]bool{}
	var alias *Release
	for endpoint != "" {
		if seen[endpoint] {
			return Release{}, fmt.Errorf("repeated release page: %s", endpoint)
		}
		seen[endpoint] = true
		releases, next, err := resolveReleasesPage(ctx, rule, ref.Owner, ref.Repo, endpoint)
		if err != nil {
			return Release{}, err
		}
		for _, rel := range releases {
			if rel.Tag == tag {
				return rel, nil
			}
			if alias == nil && TagEqual(rel.Tag, tag) {
				copy := rel
				alias = &copy
			}
		}
		endpoint = next
	}
	if alias != nil {
		return *alias, nil
	}
	tags, err := Tags(ctx, rawURL)
	if err != nil {
		return Release{}, err
	}
	for _, rel := range tags {
		if rel.Tag == tag {
			return rel, nil
		}
		if alias == nil && TagEqual(rel.Tag, tag) {
			copy := rel
			alias = &copy
		}
	}
	if alias != nil {
		return *alias, nil
	}
	return Release{}, fmt.Errorf("no release or Git tag named %s", tag)
}

// TagEqual tolerates a leading v on either version identifier.
func TagEqual(a, b string) bool {
	return a == b || strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}
