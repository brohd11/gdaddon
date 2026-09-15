package source

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type tagPage struct {
	body   string
	next   string
	status int
}

func tagHTTP(t *testing.T, pages map[string]tagPage) *[]string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv("GITHUB_TOKEN", "test-token")
	var calls []string
	orig := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = orig })
	http.DefaultClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls = append(calls, r.URL.RequestURI())
		p, ok := pages[r.URL.RequestURI()]
		if !ok {
			t.Errorf("unexpected request: %s", r.URL)
			return nil, fmt.Errorf("unexpected request")
		}
		if p.status == 0 {
			p.status = http.StatusOK
		}
		h := make(http.Header)
		if p.next != "" {
			h.Set("Link", "<"+p.next+">; rel=\"next\"")
		}
		return &http.Response{StatusCode: p.status, Status: http.StatusText(p.status), Header: h,
			Body: io.NopCloser(strings.NewReader(p.body)), Request: r}, nil
	})
	return &calls
}

func TestResolveTag(t *testing.T) {
	const releases = "/repos/u/r/releases?per_page=30"
	const tags = "/repos/u/r/tags?per_page=100"
	for _, tc := range []struct {
		name, requested, wantTag, wantURL string
		pages                             map[string]tagPage
		ambiguous, wantErr                bool
	}{
		{name: "source only", requested: "v1.0.0", wantTag: "v1.0.0", wantURL: "https://github.com/u/r/archive/refs/tags/v1.0.0.zip",
			pages: map[string]tagPage{releases: {body: `[]`}, tags: {body: `[{"name":"v1.0.0"}]`}}},
		{name: "leading v", requested: "1.0.0", wantTag: "v1.0.0", wantURL: "https://github.com/u/r/archive/refs/tags/v1.0.0.zip",
			pages: map[string]tagPage{releases: {body: `[]`}, tags: {body: `[{"name":"v1.0.0"}]`}}},
		{name: "slash tag", requested: "lib/v1.0.0", wantTag: "lib/v1.0.0", wantURL: "https://github.com/u/r/archive/refs/tags/lib%2Fv1.0.0.zip",
			pages: map[string]tagPage{releases: {body: `[]`}, tags: {body: `[{"name":"lib/v1.0.0"}]`}}},
		{name: "exact tag beyond alias page", requested: "1.0.0", wantTag: "1.0.0", wantURL: "https://github.com/u/r/archive/refs/tags/1.0.0.zip",
			pages: map[string]tagPage{releases: {body: `[]`}, tags: {body: `[{"name":"v1.0.0"}]`, next: "/tags-page-2"}, "/tags-page-2": {body: `[{"name":"1.0.0"}]`}}},
		{name: "release build wins", requested: "v1.0.0", wantTag: "v1.0.0", wantURL: "https://download/build.zip",
			pages: map[string]tagPage{releases: {body: `[{"tag_name":"v1.0.0","assets":[{"name":"build.zip","browser_download_url":"https://download/build.zip"}]}]`}}},
		{name: "release alias wins over unpublished exact tag", requested: "1.0.0", wantTag: "v1.0.0", wantURL: "https://download/build.zip",
			pages: map[string]tagPage{releases: {body: `[{"tag_name":"v1.0.0","assets":[{"name":"build.zip","browser_download_url":"https://download/build.zip"}]}]`}}},
		{name: "exact release beyond alias page", requested: "1.0.0", wantTag: "1.0.0", wantURL: "https://download/exact.zip",
			pages: map[string]tagPage{releases: {body: `[{"tag_name":"v1.0.0"}]`, next: "/releases-page-2"}, "/releases-page-2": {body: `[{"tag_name":"1.0.0","assets":[{"name":"exact.zip","browser_download_url":"https://download/exact.zip"}]}]`}}},
		{name: "ambiguous builds", requested: "v1.0.0", wantTag: "v1.0.0", ambiguous: true,
			pages: map[string]tagPage{releases: {body: `[{"tag_name":"v1.0.0","assets":[{"name":"a.zip"},{"name":"b.zip"}]}]`}}},
		{name: "missing", requested: "main", wantErr: true,
			pages: map[string]tagPage{releases: {body: `[]`}, tags: {body: `[{"name":"v1.0.0"}]`}}},
		{name: "release failure does not fall back", requested: "v1.0.0", wantErr: true,
			pages: map[string]tagPage{releases: {status: 503}}},
		{name: "tag failure", requested: "v1.0.0", wantErr: true,
			pages: map[string]tagPage{releases: {body: `[]`}, tags: {status: 503}}},
		{name: "repeated page", requested: "v1.0.0", wantErr: true,
			pages: map[string]tagPage{releases: {body: `[]`}, tags: {body: `[]`, next: tags}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tagHTTP(t, tc.pages)
			rel, err := ResolveTag(context.Background(), "https://github.com/u/r", tc.requested)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected lookup error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if rel.Tag != tc.wantTag {
				t.Fatalf("tag = %q, want %q", rel.Tag, tc.wantTag)
			}
			asset, ok := AutoAsset(rel)
			if tc.ambiguous {
				if ok {
					t.Fatalf("ambiguous release selected %+v", asset)
				}
				return
			}
			if !ok || asset.URL != tc.wantURL {
				t.Fatalf("asset = %+v, ok=%v, want %s", asset, ok, tc.wantURL)
			}
		})
	}
}

func TestTagsCodeberg(t *testing.T) {
	tagHTTP(t, map[string]tagPage{
		"/api/v1/repos/u/r/tags?limit=100":        {body: `[{"name":"v1.0.0"}]`, next: "?limit=100&page=2"},
		"/api/v1/repos/u/r/tags?limit=100&page=2": {body: `[{"name":"v0.9.0"}]`},
	})
	tags, err := Tags(context.Background(), "https://codeberg.org/u/r")
	if err != nil || len(tags) != 2 {
		t.Fatalf("tags = %+v, err = %v", tags, err)
	}
	if a := tags[1].Assets[0]; a.URL != "https://codeberg.org/u/r/archive/v0.9.0.zip" || !a.Generated {
		t.Fatalf("source asset = %+v", a)
	}
}

func TestAvailableVersionsDoesNotFetchTags(t *testing.T) {
	calls := tagHTTP(t, map[string]tagPage{
		"/repos/u/r/releases?per_page=30": {body: `[{"tag_name":"v1.0.0"}]`, next: "/older-releases"},
	})
	listing, err := AvailableVersions(context.Background(), "https://github.com/u/r")
	if err != nil || len(listing.Releases) != 1 || listing.Releases[0].Tag != "v1.0.0" || len(*calls) != 1 {
		t.Fatalf("listing = %+v, err = %v, calls = %v", listing, err, *calls)
	}
}
