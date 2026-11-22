package addon

import (
	"context"
	"testing"

	"github.com/brohd11/gdaddon/internal/source"
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
	if _, res := ResolveUpdate(context.Background(), a, "1.0.0"); res != ResolveNone {
		t.Errorf("ResolveUpdate on locked addon = %v, want ResolveNone", res)
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

func TestReleaseForURL(t *testing.T) {
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
			if _, got := releaseForURL(c.url, releases); got != c.want {
				t.Errorf("releaseForURL(%q) found = %v, want %v", c.url, got, c.want)
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

func TestUpdatePinKey(t *testing.T) {
	base := Addon{Name: "a", URL: "https://github.com/u/r", Tag: "v1", Version: "1", Path: "addons/a"}
	key := UpdatePinKey(base)
	same := base
	same.Path, same.Display, same.Dependency = "addons/b", "Other", true
	if UpdatePinKey(same) != key {
		t.Fatal("fields CheckUpdate ignores changed the key")
	}
	for name, edit := range map[string]func(*Addon){
		"url":     func(a *Addon) { a.URL += "x" },
		"tag":     func(a *Addon) { a.Tag = "v2" },
		"version": func(a *Addon) { a.Version = "2" },
		"commit":  func(a *Addon) { a.Commit = "abc" },
		"kind":    func(a *Addon) { a.Kind = KindClone },
		"lock":    func(a *Addon) { a.Lock = true },
	} {
		a := base
		edit(&a)
		if UpdatePinKey(a) == key {
			t.Errorf("%s change kept the key", name)
		}
	}
}

func TestCheckUpdatesOfKeysByName(t *testing.T) {
	_, calls := tagInstallHTTP(t)
	got := CheckUpdatesOf(context.Background(), []Addon{
		{Name: "locked", URL: "https://github.com/u/r", Lock: true},
		{Name: "nourl"},
	})
	if len(got) != 2 || got["locked"].State != UpdateLocked || got["nourl"].State != UpdateUnknown {
		t.Fatalf("checks = %+v", got)
	}
	if len(calls) != 0 {
		t.Fatalf("unexpected requests %v", calls)
	}
}
