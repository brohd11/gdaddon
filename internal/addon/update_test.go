package addon

import (
	"context"
	"testing"

	"gdaddon/internal/source"
)

func TestLockedSkipsUpdateCheck(t *testing.T) {
	// A locked addon short-circuits before any network fetch: CheckUpdate reads as
	// UpdateUnknown (no marker) and ResolveUpdate plans nothing. The url would
	// otherwise be resolved over the network, so reaching it would hang/fail the test.
	a := Addon{
		Name:    "Locked",
		URL:     "https://github.com/owner/repo/releases/download/v1.0.0/repo.zip",
		Path:    "addons/repo",
		Version: "1.0.0",
		Tag:     "v1.0.0",
		Lock:    true,
	}

	if info := CheckUpdate(context.Background(), a); info.State != UpdateLocked {
		t.Errorf("CheckUpdate on locked addon = %v, want UpdateLocked", info.State)
	}
	if _, ok := ResolveUpdate(context.Background(), a, "1.0.0"); ok {
		t.Errorf("ResolveUpdate on locked addon returned a plan, want ok=false")
	}
	if got := UpdateLocked.String(); got != "locked" {
		t.Errorf("UpdateLocked.String() = %q, want \"locked\"", got)
	}
}

func TestLatestRelease(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if _, ok := LatestRelease(nil); ok {
			t.Errorf("expected ok=false for no releases")
		}
	})

	t.Run("skips prereleases", func(t *testing.T) {
		releases := []source.Release{
			{Tag: "v2.0.0-rc1", Prerelease: true},
			{Tag: "v1.0.0"},
		}
		got, ok := LatestRelease(releases)
		if !ok || got.Tag != "v1.0.0" {
			t.Errorf("got %q ok=%v, want v1.0.0", got.Tag, ok)
		}
	})

	t.Run("falls back to newest when all prerelease", func(t *testing.T) {
		releases := []source.Release{
			{Tag: "v2.0.0-rc2", Prerelease: true},
			{Tag: "v2.0.0-rc1", Prerelease: true},
		}
		got, ok := LatestRelease(releases)
		if !ok || got.Tag != "v2.0.0-rc2" {
			t.Errorf("got %q ok=%v, want v2.0.0-rc2", got.Tag, ok)
		}
	})
}

func TestResolveUpdateAsset(t *testing.T) {
	// Two releases; each has an uploaded asset plus a (last-appended) source archive.
	old := source.Release{Tag: "v1.0.0", Assets: []source.Asset{
		{Name: "addon.zip", URL: "https://h/dl/v1.0.0/addon.zip"},
		{Name: "Source code (zip)", URL: "https://h/archive/v1.0.0.zip"},
	}}
	latest := source.Release{Tag: "v2.0.0", Assets: []source.Asset{
		{Name: "addon.zip", URL: "https://h/dl/v2.0.0/addon.zip"},
		{Name: "Source code (zip)", URL: "https://h/archive/v2.0.0.zip"},
	}}
	releases := []source.Release{latest, old}

	t.Run("matches the uploaded asset by name", func(t *testing.T) {
		got, ok := resolveUpdateAsset("https://h/dl/v1.0.0/addon.zip", releases, latest)
		if !ok || got.URL != "https://h/dl/v2.0.0/addon.zip" {
			t.Errorf("got %q ok=%v, want the v2.0.0 addon.zip", got.URL, ok)
		}
	})

	t.Run("matches the source archive by name", func(t *testing.T) {
		got, ok := resolveUpdateAsset("https://h/archive/v1.0.0.zip", releases, latest)
		if !ok || got.URL != "https://h/archive/v2.0.0.zip" {
			t.Errorf("got %q ok=%v, want the v2.0.0 source archive", got.URL, ok)
		}
	})

	t.Run("falls back to the last asset when url is unknown", func(t *testing.T) {
		got, ok := resolveUpdateAsset("https://h/dl/v0.9.0/legacy.zip", releases, latest)
		if !ok || got.URL != "https://h/archive/v2.0.0.zip" {
			t.Errorf("got %q ok=%v, want the source-archive fallback", got.URL, ok)
		}
	})

	t.Run("no assets", func(t *testing.T) {
		if _, ok := resolveUpdateAsset("x", nil, source.Release{}); ok {
			t.Errorf("expected ok=false when the latest release has no assets")
		}
	})
}

func TestURLInReleases(t *testing.T) {
	releases := []source.Release{
		{Tag: "v2.0.0", Assets: []source.Asset{{Name: "addon.zip", URL: "https://h/dl/v2.0.0/addon.zip"}}},
		{Tag: "v1.0.0", Assets: []source.Asset{{Name: "addon.zip", URL: "https://h/dl/v1.0.0/addon.zip"}}},
	}
	cases := []struct {
		name string
		url  string
		want bool
	}{
		{"latest", "https://h/dl/v2.0.0/addon.zip", true},
		{"older", "https://h/dl/v1.0.0/addon.zip", true},
		{"bare/clone url", "https://github.com/owner/repo.git", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := urlInReleases(c.url, releases); got != c.want {
				t.Errorf("urlInReleases(%q) = %v, want %v", c.url, got, c.want)
			}
		})
	}
}

func TestCurrentByVersion(t *testing.T) {
	cases := []struct {
		name                string
		addon               Addon
		latestTag           string
		wantCurrent, wantOK bool
	}{
		{"tag preferred over version", Addon{Tag: "v2.0.0", Version: "1.0.0"}, "v2.0.0", true, true},
		{"version used when tag empty", Addon{Version: "1.2.3"}, "v1.2.3", true, true},
		{"equal is current", Addon{Version: "1.0.0"}, "v1.0.0", true, true},
		{"older is not current", Addon{Version: "1.0.0"}, "v2.0.0", false, true},
		{"newer is current", Addon{Version: "2.1.0"}, "v2.0.0", true, true},
		{"date stamp uncomparable", Addon{Version: "2024-01-02"}, "v2.0.0", false, false},
		{"empty installed uncomparable", Addon{}, "v2.0.0", false, false},
		{"uncomparable latest tag", Addon{Version: "1.0.0"}, "main", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			current, ok := currentByVersion(c.addon, c.latestTag)
			if current != c.wantCurrent || ok != c.wantOK {
				t.Errorf("currentByVersion = (%v, %v), want (%v, %v)", current, ok, c.wantCurrent, c.wantOK)
			}
		})
	}
}
