package source

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"gdaddon/internal/config"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// stubHTTP swaps the default client's transport to return body for any request
// and restores it when the test ends.
func stubHTTP(t *testing.T, body string) {
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

// githubRule / codebergRule mirror the shipped defaults, fetched by host so the
// tests assert the per-host archive-URL patterns the config drives.
func githubRule(t *testing.T) *config.VCSRule   { return ruleByHost(t, "github.com") }
func codebergRule(t *testing.T) *config.VCSRule { return ruleByHost(t, "codeberg.org") }

func ruleByHost(t *testing.T, host string) *config.VCSRule {
	t.Helper()
	for _, s := range config.DefaultSources() {
		if s.VCS != nil && s.VCS.Host == host {
			return s.VCS
		}
	}
	t.Fatalf("no default vcs rule for %s", host)
	return nil
}

func TestParseRepoURL(t *testing.T) {
	cases := []struct {
		url, host, owner, repo, branch string
	}{
		{"https://github.com/TokisanGames/Terrain3D/releases/download/v1.0.1/Terrain3D_v1.0.1.zip", "github.com", "TokisanGames", "Terrain3D", ""},
		{"https://github.com/brohd11/godot-plugin-devtools/archive/refs/heads/main-dev.zip", "github.com", "brohd11", "godot-plugin-devtools", "main-dev"},
		{"https://github.com/brohd11/YAML.gd.git", "github.com", "brohd11", "YAML.gd", ""},
		{"https://codeberg.org/bramwell/cogito", "codeberg.org", "bramwell", "cogito", ""},
	}
	for _, c := range cases {
		ref, err := parseRepoURL(c.url)
		if err != nil {
			t.Fatalf("%s: %v", c.url, err)
		}
		if ref.Host != c.host || ref.Owner != c.owner || ref.Repo != c.repo || ref.Branch != c.branch {
			t.Errorf("%s => %+v, want %s %s/%s branch=%q", c.url, ref, c.host, c.owner, c.repo, c.branch)
		}
	}
}

func TestResolveReleasesGitHub(t *testing.T) {
	stubHTTP(t, `[
		{"tag_name":"v1.0.0","prerelease":false,"assets":[
			{"name":"addon.zip","browser_download_url":"https://x/addon.zip"},
			{"name":"notes.txt","browser_download_url":"https://x/notes.txt"}
		]},
		{"tag_name":"v0.9.0","prerelease":true,"assets":[]}
	]`)

	rels, err := resolveReleases(context.Background(), githubRule(t), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 {
		t.Fatalf("want 2 releases, got %d", len(rels))
	}

	// v1.0.0: uploaded .zip kept, .txt dropped, source archive appended last.
	r0 := rels[0]
	if len(r0.Assets) != 2 || r0.Assets[0].Name != "addon.zip" {
		t.Fatalf("v1.0.0 assets = %+v", r0.Assets)
	}
	if got, want := r0.Assets[1].URL, "https://github.com/owner/repo/archive/refs/tags/v1.0.0.zip"; got != want {
		t.Errorf("source archive url = %q, want %q", got, want)
	}

	// v0.9.0: prerelease flag read; source archive is the only option.
	if !rels[1].Prerelease {
		t.Error("v0.9.0 should be prerelease")
	}
	if len(rels[1].Assets) != 1 {
		t.Errorf("v0.9.0 want 1 asset (source only), got %d", len(rels[1].Assets))
	}
}

func TestResolveReleasesCodeberg(t *testing.T) {
	stubHTTP(t, `[{"tag_name":"v2.0","prerelease":false,"assets":[
		{"name":"cogito.zip","browser_download_url":"https://cb/cogito.zip"}
	]}]`)

	rels, err := resolveReleases(context.Background(), codebergRule(t), "bramwell", "cogito")
	if err != nil {
		t.Fatal(err)
	}
	// Codeberg/Gitea archive URL has no /refs/tags/ segment.
	last := rels[0].Assets[len(rels[0].Assets)-1]
	if got, want := last.URL, "https://codeberg.org/bramwell/cogito/archive/v2.0.zip"; got != want {
		t.Errorf("codeberg source archive = %q, want %q", got, want)
	}
}

func TestResolveBranches(t *testing.T) {
	stubHTTP(t, `[{"name":"main"},{"name":"feature/x"}]`)

	bs, err := resolveBranches(context.Background(), githubRule(t), "owner", "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(bs) != 2 {
		t.Fatalf("want 2 branches, got %d", len(bs))
	}
	if bs[0].URL != "https://github.com/owner/repo/archive/refs/heads/main.zip" {
		t.Errorf("branch[0] = %+v", bs[0])
	}
	if bs[1].URL != "https://github.com/owner/repo/archive/refs/heads/feature/x.zip" {
		t.Errorf("branch[1] url = %q", bs[1].URL)
	}
}

func TestCloneFallbackForUnknownHost(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no user config → built-in defaults only
	l, err := AvailableVersions(context.Background(), "https://git.example.com/alice/widget.git")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Releases) != 1 || len(l.Releases[0].Assets) != 1 {
		t.Fatalf("fallback listing = %+v", l)
	}
	if got, want := l.Releases[0].Assets[0].URL, "https://git.example.com/alice/widget.git"; got != want {
		t.Errorf("fallback clone url = %q, want %q", got, want)
	}
}

func TestRepoID(t *testing.T) {
	cases := map[string]string{
		"https://github.com/brohd11/godot-plugin-devtools.git":                                 "github.com/brohd11/godot-plugin-devtools",
		"https://github.com/brohd11/godot-plugin-devtools":                                     "github.com/brohd11/godot-plugin-devtools",
		"https://github.com/brohd11/godot-plugin-devtools/releases/download/0.2.1/p-0.2.1.zip": "github.com/brohd11/godot-plugin-devtools",
		"https://github.com/brohd11/godot-plugin-devtools/archive/refs/heads/main.zip":         "github.com/brohd11/godot-plugin-devtools",
		"https://github.com/BroHD11/Godot-Plugin-Devtools.git":                                 "github.com/brohd11/godot-plugin-devtools",
		"https://codeberg.org/bramwell/cogito":                                                 "codeberg.org/bramwell/cogito",
		"https://gitlab.com/u/repo.git":                                                        "gitlab.com/u/repo",
	}
	for u, want := range cases {
		got, err := RepoID(u)
		if err != nil {
			t.Errorf("RepoID(%q) error: %v", u, err)
			continue
		}
		if got != want {
			t.Errorf("RepoID(%q) = %q, want %q", u, got, want)
		}
	}
	// A URL without owner/repo is unparseable.
	if _, err := RepoID("https://github.com/onlyowner"); err == nil {
		t.Error("expected error for url missing owner/repo")
	}
}

func TestRepoURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/brohd11/godot-plugin-devtools.git":                                 "https://github.com/brohd11/godot-plugin-devtools",
		"https://github.com/brohd11/godot-plugin-devtools":                                     "https://github.com/brohd11/godot-plugin-devtools",
		"https://github.com/brohd11/godot-plugin-devtools/releases/download/0.2.1/p-0.2.1.zip": "https://github.com/brohd11/godot-plugin-devtools",
		"https://github.com/brohd11/godot-plugin-devtools/archive/refs/heads/main.zip":         "https://github.com/brohd11/godot-plugin-devtools",
		"https://codeberg.org/bramwell/cogito":                                                 "https://codeberg.org/bramwell/cogito",
	}
	for u, want := range cases {
		got, err := RepoURL(u)
		if err != nil {
			t.Errorf("RepoURL(%q) error: %v", u, err)
			continue
		}
		if got != want {
			t.Errorf("RepoURL(%q) = %q, want %q", u, got, want)
		}
	}
	if _, err := RepoURL("https://github.com/onlyowner"); err == nil {
		t.Error("expected error for url missing owner/repo")
	}
}

// TestLiveAvailableVersions hits the real GitHub + Codeberg APIs. Run with GDUTIL_LIVE=1.
func TestLiveAvailableVersions(t *testing.T) {
	if os.Getenv("GDUTIL_LIVE") == "" {
		t.Skip("set GDUTIL_LIVE=1 to run live API test")
	}
	urls := []string{
		"https://github.com/TokisanGames/Terrain3D/releases/download/v1.0.1/Terrain3D_v1.0.1.zip",
		"https://codeberg.org/Phazorknight/Cogito",
	}
	for _, u := range urls {
		l, err := AvailableVersions(context.Background(), u)
		if err != nil {
			t.Fatalf("%s: %v", u, err)
		}
		t.Logf("%s/%s: %d releases", l.Owner, l.Repo, len(l.Releases))
		for _, r := range l.Releases[:min(2, len(l.Releases))] {
			t.Logf("    %s  pre=%v  assets=%d (last=%s)", r.Tag, r.Prerelease, len(r.Assets), r.Assets[len(r.Assets)-1].URL)
		}
	}
}
