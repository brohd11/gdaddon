package addon

import (
	"errors"
	"strings"
	"testing"

	"gdaddon/internal/source"
)

func TestParseRepoSpec(t *testing.T) {
	cases := []struct {
		name string
		spec string
		want Dependency
		ok   bool
	}{
		{
			"owner/repo defaults to github",
			"brohd11/Godot-Script-Tabs",
			Dependency{Host: "github.com", Owner: "brohd11", Repo: "Godot-Script-Tabs", RepoURL: "https://github.com/brohd11/Godot-Script-Tabs", RepoID: "github.com/brohd11/godot-script-tabs"},
			true,
		},
		{
			"owner/repo@tag",
			"brohd11/Godot-Script-Tabs@v0.1.4",
			Dependency{Host: "github.com", Owner: "brohd11", Repo: "Godot-Script-Tabs", Tag: "v0.1.4", RepoURL: "https://github.com/brohd11/Godot-Script-Tabs", RepoID: "github.com/brohd11/godot-script-tabs"},
			true,
		},
		{
			"explicit host",
			"codeberg.org/u/Foo",
			Dependency{Host: "codeberg.org", Owner: "u", Repo: "Foo", RepoURL: "https://codeberg.org/u/Foo", RepoID: "codeberg.org/u/foo"},
			true,
		},
		{
			"explicit host with tag",
			"codeberg.org/u/Foo@1.2.3",
			Dependency{Host: "codeberg.org", Owner: "u", Repo: "Foo", Tag: "1.2.3", RepoURL: "https://codeberg.org/u/Foo", RepoID: "codeberg.org/u/foo"},
			true,
		},
		{"surrounding space is tolerated", "  u/r  ", Dependency{Host: "github.com", Owner: "u", Repo: "r", RepoURL: "https://github.com/u/r", RepoID: "github.com/u/r"}, true},
		{"single segment", "justaname", Dependency{}, false},
		{"too many segments", "a/b/c/d", Dependency{}, false},
		{"empty owner", "/repo", Dependency{}, false},
		{"empty", "", Dependency{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseRepoSpec(tc.spec)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Errorf("\n got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

func TestSelectRelease(t *testing.T) {
	releases := []source.Release{
		{Tag: "v2.0.0-rc1", Prerelease: true},
		{Tag: "v1.2.0"},
		{Tag: "1.1.0"},
	}

	t.Run("empty tag takes the latest non-prerelease", func(t *testing.T) {
		rel, err := SelectRelease(releases, "")
		if err != nil {
			t.Fatal(err)
		}
		if rel.Tag != "v1.2.0" {
			t.Errorf("Tag = %q, want v1.2.0", rel.Tag)
		}
	})

	t.Run("exact tag", func(t *testing.T) {
		rel, err := SelectRelease(releases, "v2.0.0-rc1")
		if err != nil {
			t.Fatal(err)
		}
		if rel.Tag != "v2.0.0-rc1" {
			t.Errorf("Tag = %q, want v2.0.0-rc1", rel.Tag)
		}
	})

	// A leading "v" is tolerated in either direction, so a user need not know whether
	// the author tags "1.2.0" or "v1.2.0".
	t.Run("v-prefix tolerated both ways", func(t *testing.T) {
		for spec, want := range map[string]string{"1.2.0": "v1.2.0", "v1.1.0": "1.1.0"} {
			rel, err := SelectRelease(releases, spec)
			if err != nil {
				t.Fatalf("%s: %v", spec, err)
			}
			if rel.Tag != want {
				t.Errorf("%s → %q, want %q", spec, rel.Tag, want)
			}
		}
	})

	t.Run("unknown tag names the available ones", func(t *testing.T) {
		_, err := SelectRelease(releases, "v9.9.9")
		if err == nil {
			t.Fatal("want an error")
		}
		if !strings.Contains(err.Error(), "v1.2.0") {
			t.Errorf("error should list the available tags, got: %v", err)
		}
	})

	t.Run("no releases", func(t *testing.T) {
		if _, err := SelectRelease(nil, ""); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestSelectAsset(t *testing.T) {
	uploaded := source.Asset{Name: "addon-1.0.0.zip", URL: "https://h/u/r/releases/download/v1/addon-1.0.0.zip"}
	linux := source.Asset{Name: "addon-linux.zip", URL: "https://h/u/r/releases/download/v1/addon-linux.zip"}
	generated := source.Asset{Name: "Source code.zip", URL: "https://h/u/r/archive/v1.zip", Generated: true}

	t.Run("one upload wins over the generated archive", func(t *testing.T) {
		got, err := SelectAsset(source.Release{Tag: "v1", Assets: []source.Asset{uploaded, generated}}, "")
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != uploaded.Name {
			t.Errorf("Name = %q, want %q", got.Name, uploaded.Name)
		}
	})

	t.Run("no upload falls back to the generated archive", func(t *testing.T) {
		got, err := SelectAsset(source.Release{Tag: "v1", Assets: []source.Asset{generated}}, "")
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != generated.Name {
			t.Errorf("Name = %q, want %q", got.Name, generated.Name)
		}
	})

	t.Run("two uploads are ambiguous", func(t *testing.T) {
		_, err := SelectAsset(source.Release{Tag: "v1", Assets: []source.Asset{uploaded, linux, generated}}, "")
		var ambiguous *AmbiguousAssetError
		if !errors.As(err, &ambiguous) {
			t.Fatalf("want *AmbiguousAssetError, got %v", err)
		}
		if len(ambiguous.Assets) != 3 {
			t.Errorf("Assets = %v, want all three listed", ambiguous.Assets)
		}
		if !strings.Contains(err.Error(), "--asset") {
			t.Errorf("error should point at --asset, got: %v", err)
		}
	})

	t.Run("hint resolves the ambiguity", func(t *testing.T) {
		got, err := SelectAsset(source.Release{Tag: "v1", Assets: []source.Asset{uploaded, linux, generated}}, "LINUX")
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != linux.Name {
			t.Errorf("Name = %q, want %q", got.Name, linux.Name)
		}
	})

	t.Run("hint matching nothing lists the candidates", func(t *testing.T) {
		_, err := SelectAsset(source.Release{Tag: "v1", Assets: []source.Asset{uploaded, generated}}, "windows")
		if err == nil {
			t.Fatal("want an error")
		}
		if !strings.Contains(err.Error(), uploaded.Name) {
			t.Errorf("error should list the assets, got: %v", err)
		}
	})

	t.Run("hint matching several is still ambiguous", func(t *testing.T) {
		_, err := SelectAsset(source.Release{Tag: "v1", Assets: []source.Asset{uploaded, linux, generated}}, "addon")
		var ambiguous *AmbiguousAssetError
		if !errors.As(err, &ambiguous) {
			t.Fatalf("want *AmbiguousAssetError, got %v", err)
		}
		// Only the matches are worth showing, not every asset in the release.
		if len(ambiguous.Assets) != 2 {
			t.Errorf("Assets = %v, want only the two matches", ambiguous.Assets)
		}
	})
}
