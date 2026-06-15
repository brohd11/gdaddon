package source

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// stubGitHub swaps the default client's transport to return body for any request
// and restores it when the test ends.
func stubGitHub(t *testing.T, body string) {
	t.Helper()
	orig := http.DefaultClient.Transport
	t.Cleanup(func() { http.DefaultClient.Transport = orig })
	http.DefaultClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})
}

func TestParseGitHub(t *testing.T) {
	cases := []struct {
		url, owner, repo, branch string
	}{
		{"https://github.com/TokisanGames/Terrain3D/releases/download/v1.0.1/Terrain3D_v1.0.1.zip", "TokisanGames", "Terrain3D", ""},
		{"https://github.com/brohd11/godot-plugin-devtools/archive/refs/heads/main-dev.zip", "brohd11", "godot-plugin-devtools", "main-dev"},
		{"https://github.com/brohd11/YAML.gd.git", "brohd11", "YAML.gd", ""},
	}
	for _, c := range cases {
		ref, err := parseGitHub(c.url)
		if err != nil {
			t.Fatalf("%s: %v", c.url, err)
		}
		if ref.Owner != c.owner || ref.Repo != c.repo || ref.Branch != c.branch {
			t.Errorf("%s => %+v, want %s/%s branch=%q", c.url, ref, c.owner, c.repo, c.branch)
		}
	}
}

func TestFetchReleasesAppendsSourceArchive(t *testing.T) {
	stubGitHub(t, `[
		{"tag_name":"v1.0.0","prerelease":false,"assets":[
			{"name":"addon.zip","browser_download_url":"https://x/addon.zip"},
			{"name":"notes.txt","browser_download_url":"https://x/notes.txt"}
		]},
		{"tag_name":"v0.9.0","prerelease":true,"assets":[]}
	]`)

	rels, err := fetchReleases(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 {
		t.Fatalf("want 2 releases, got %d", len(rels))
	}

	// v1.0.0: uploaded .zip kept, .txt dropped, source archive appended last.
	r0 := rels[0]
	if len(r0.Assets) != 2 {
		t.Fatalf("v1.0.0 want 2 assets, got %d: %+v", len(r0.Assets), r0.Assets)
	}
	if r0.Assets[0].Name != "addon.zip" {
		t.Errorf("first asset = %q, want addon.zip", r0.Assets[0].Name)
	}
	last := r0.Assets[len(r0.Assets)-1]
	wantURL := "https://github.com/owner/repo/archive/refs/tags/v1.0.0.zip"
	if last.URL != wantURL {
		t.Errorf("source archive url = %q, want %q", last.URL, wantURL)
	}

	// v0.9.0: no uploaded assets, so the source archive is the only option.
	if len(rels[1].Assets) != 1 {
		t.Errorf("v0.9.0 want 1 asset (source only), got %d", len(rels[1].Assets))
	}
}

func TestFetchBranches(t *testing.T) {
	stubGitHub(t, `[{"name":"main"},{"name":"feature/x"}]`)

	bs, err := fetchBranches(context.Background(), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 {
		t.Fatalf("want 2 branches, got %d", len(bs))
	}
	if bs[0].Name != "main" || bs[0].URL != "https://github.com/owner/repo/archive/refs/heads/main.zip" {
		t.Errorf("branch[0] = %+v", bs[0])
	}
	if bs[1].URL != "https://github.com/owner/repo/archive/refs/heads/feature/x.zip" {
		t.Errorf("branch[1] url = %q", bs[1].URL)
	}
}

// TestLiveAvailableVersions hits the real GitHub API. Run with GDUTIL_LIVE=1.
func TestLiveAvailableVersions(t *testing.T) {
	if os.Getenv("GDUTIL_LIVE") == "" {
		t.Skip("set GDUTIL_LIVE=1 to run live API test")
	}
	urls := []string{
		"https://github.com/TokisanGames/Terrain3D/releases/download/v1.0.1/Terrain3D_v1.0.1.zip",
		"https://github.com/brohd11/godot-plugin-devtools/archive/refs/heads/main-dev.zip",
	}
	for _, u := range urls {
		l, err := AvailableVersions(context.Background(), u)
		if err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		t.Logf("%s/%s: %d releases, branch=%v", l.Owner, l.Repo, len(l.Releases), l.Branch != nil)
		for _, r := range l.Releases {
			for _, a := range r.Assets {
				t.Logf("    %s  pre=%v  %s", r.Tag, r.Prerelease, a.Name)
			}
		}
	}
}

func TestRepoID(t *testing.T) {
	want := "github.com/brohd11/godot-plugin-devtools"
	urls := []string{
		"https://github.com/brohd11/godot-plugin-devtools.git",
		"https://github.com/brohd11/godot-plugin-devtools",
		"https://github.com/brohd11/godot-plugin-devtools/releases/download/0.2.1/plugin-devtools-0.2.1.zip",
		"https://github.com/brohd11/godot-plugin-devtools/archive/refs/heads/main.zip",
		"https://github.com/brohd11/godot-plugin-devtools/archive/refs/tags/v0.2.1.zip",
		"https://github.com/BroHD11/Godot-Plugin-Devtools.git", // case-insensitive
	}
	for _, u := range urls {
		got, err := RepoID(u)
		if err != nil {
			t.Errorf("RepoID(%q) error: %v", u, err)
			continue
		}
		if got != want {
			t.Errorf("RepoID(%q) = %q, want %q", u, got, want)
		}
	}
	if _, err := RepoID("https://gitlab.com/u/repo.git"); err == nil {
		t.Errorf("expected error for non-github url")
	}
}
