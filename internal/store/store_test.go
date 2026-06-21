package store

import "testing"

func TestIsStoreURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://store.godotengine.org/chickensoft/godot_serialization", true},
		{"https://www.store.godotengine.org/pub/slug", true},
		{"https://github.com/owner/repo.git", false},
		{"https://codeberg.org/owner/repo", false},
		{"https://godotengine.org/asset-library/api/asset", false},
		{"not a url at all ::::", false},
		{"", false},
	}
	for _, c := range cases {
		if got := IsStoreURL(c.url); got != c.want {
			t.Errorf("IsStoreURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestAssetID(t *testing.T) {
	id, err := AssetID("https://store.godotengine.org/chickensoft/godot_serialization")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "chickensoft/godot_serialization" {
		t.Errorf("AssetID = %q, want chickensoft/godot_serialization", id)
	}

	if _, err := AssetID("https://store.godotengine.org/onlyone"); err == nil {
		t.Error("expected error for a url without publisher/slug")
	}
}

func TestPickStable(t *testing.T) {
	// Prefers the first stable release over an earlier unstable one.
	rel, ok := PickStable([]Release{
		{Version: "2.0.0-beta", Stable: false, DownloadURL: "u-beta"},
		{Version: "1.5.0", Stable: true, DownloadURL: "u-stable"},
	})
	if !ok || rel.Version != "1.5.0" || rel.DownloadURL != "u-stable" {
		t.Errorf("PickStable stable = %+v, ok=%v; want 1.5.0/u-stable", rel, ok)
	}

	// No stable flag: falls back to the first release.
	rel, ok = PickStable([]Release{
		{Version: "0.9.0", DownloadURL: "u-first"},
		{Version: "0.8.0", DownloadURL: "u-second"},
	})
	if !ok || rel.Version != "0.9.0" {
		t.Errorf("PickStable fallback = %+v, ok=%v; want 0.9.0", rel, ok)
	}

	if _, ok := PickStable(nil); ok {
		t.Error("expected ok=false for empty release list")
	}
}

func TestAssetName(t *testing.T) {
	if got := assetName("godot_serialization", "1.2.3"); got != "godot_serialization-1.2.3.zip" {
		t.Errorf("assetName = %q", got)
	}
	if got := assetName("slug", ""); got != "slug.zip" {
		t.Errorf("assetName (no version) = %q", got)
	}
}
