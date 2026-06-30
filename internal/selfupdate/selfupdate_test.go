package selfupdate

import (
	"runtime"
	"testing"

	"gdaddon/internal/addon"
	"gdaddon/internal/source"
)

// platformAssetName is the asset name the running test platform should select.
func platformAssetName() string {
	return "gdaddon-v1.2.3-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
}

func TestPlatformAsset(t *testing.T) {
	want := platformAssetName()
	rel := source.Release{
		Tag: "v1.2.3",
		Assets: []source.Asset{
			{Name: "gdaddon-v1.2.3-darwin-arm64.zip", URL: "https://x/darwin-arm64"},
			{Name: "gdaddon-v1.2.3-linux-amd64.zip", URL: "https://x/linux-amd64"},
			{Name: "gdaddon-v1.2.3-windows-amd64.zip", URL: "https://x/windows-amd64"},
			{Name: "Source code (zip)", URL: "https://x/src", Generated: true},
		},
	}
	got, ok := platformAsset(rel)
	if !ok {
		t.Fatalf("platformAsset: no asset selected for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if got.Name != want {
		t.Fatalf("platformAsset: got %q, want %q", got.Name, want)
	}
}

func TestPlatformAssetNoMatch(t *testing.T) {
	rel := source.Release{Tag: "v1.2.3", Assets: []source.Asset{
		{Name: "gdaddon-v1.2.3-plan9-mips.zip", URL: "https://x/other"},
		{Name: "Source code (zip)", URL: "https://x/src", Generated: true},
	}}
	if _, ok := platformAsset(rel); ok {
		t.Fatal("platformAsset: matched an asset for a foreign platform")
	}
}

func TestLatestRelease(t *testing.T) {
	tests := []struct {
		name     string
		releases []source.Release
		wantTag  string
		wantOK   bool
	}{
		{"empty", nil, "", false},
		{"first stable", []source.Release{{Tag: "v2.0.0"}, {Tag: "v1.0.0"}}, "v2.0.0", true},
		{"skips prerelease", []source.Release{{Tag: "v3.0.0", Prerelease: true}, {Tag: "v2.0.0"}}, "v2.0.0", true},
		{"all prerelease falls back to newest", []source.Release{{Tag: "v3.0.0", Prerelease: true}, {Tag: "v2.0.0", Prerelease: true}}, "v3.0.0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := latestRelease(tt.releases)
			if ok != tt.wantOK || got.Tag != tt.wantTag {
				t.Fatalf("latestRelease(%v) = (%q, %v), want (%q, %v)", tt.releases, got.Tag, ok, tt.wantTag, tt.wantOK)
			}
		})
	}
}

// evaluate mirrors Check's version/asset decision (without the network fetch) so the
// "dev build / already current / available" branches can be asserted directly.
func evaluate(current string, latest source.Release) Info {
	info := Info{Current: current, LatestTag: latest.Tag}
	ge, comparable := addon.SemverGE(current, latest.Tag)
	if !comparable || ge {
		return info
	}
	if asset, ok := platformAsset(latest); ok {
		info.Available = true
		info.AssetURL = asset.URL
		info.AssetName = asset.Name
	}
	return info
}

func TestEvaluate(t *testing.T) {
	latest := source.Release{Tag: "v1.5.0", Assets: []source.Asset{
		{Name: platformAssetName(), URL: "https://x/bin"},
	}}
	tests := []struct {
		name      string
		current   string
		available bool
	}{
		{"older is available", "v1.4.0", true},
		{"equal is current", "v1.5.0", false},
		{"newer is current", "v1.6.0", false},
		{"dev build uncomparable", "dev", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := evaluate(tt.current, latest).Available; got != tt.available {
				t.Fatalf("evaluate(%q).Available = %v, want %v", tt.current, got, tt.available)
			}
		})
	}
}
