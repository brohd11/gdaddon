package addon

import (
	"testing"

	"gdaddon/internal/source"
)

func TestLatestRelease(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if _, ok := latestRelease(nil); ok {
			t.Errorf("expected ok=false for no releases")
		}
	})

	t.Run("skips prereleases", func(t *testing.T) {
		releases := []source.Release{
			{Tag: "v2.0.0-rc1", Prerelease: true},
			{Tag: "v1.0.0"},
		}
		got, ok := latestRelease(releases)
		if !ok || got.Tag != "v1.0.0" {
			t.Errorf("got %q ok=%v, want v1.0.0", got.Tag, ok)
		}
	})

	t.Run("falls back to newest when all prerelease", func(t *testing.T) {
		releases := []source.Release{
			{Tag: "v2.0.0-rc2", Prerelease: true},
			{Tag: "v2.0.0-rc1", Prerelease: true},
		}
		got, ok := latestRelease(releases)
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
