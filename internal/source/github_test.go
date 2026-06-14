package source

import (
	"context"
	"os"
	"testing"
)

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
