package addon

import (
	"context"
	"strings"
	"testing"
)

// publishExt serves u/<repo> with one release, v1.0.0, carrying the given uploads, plus
// the matching git tag.
func publishExt(bodies map[string][]byte, repo string, uploads ...string) {
	var assets []string
	for _, name := range uploads {
		assets = append(assets, `{"name":"`+name+`","browser_download_url":"https://github.com/u/`+repo+`/releases/download/v1.0.0/`+name+`"}`)
	}
	bodies["/repos/u/"+repo+"/releases"] = []byte(`[{"tag_name":"v1.0.0","assets":[` + strings.Join(assets, ",") + `]}]`)
	bodies["/repos/u/"+repo+"/tags"] = []byte(`[{"name":"v1.0.0"}]`)
}

// TestRequireGrammarResolution pins PackageExporter's meaning of a spec: plain is the tag's
// source even when a release uploads a build, and /release is the upload with no fallback
// to source, a glob choosing among several.
func TestRequireGrammarResolution(t *testing.T) {
	bodies, calls := tagInstallHTTP(t)
	publishExt(bodies, "ext", "ext-linux.zip", "ext-windows.zip")
	publishExt(bodies, "plain")
	bodies["/repos/u/tagonly/releases"] = []byte(`[]`)
	bodies["/repos/u/tagonly/tags"] = []byte(`[{"name":"v1.0.0"}]`)

	resolve := func(spec string) (string, bool) {
		t.Helper()
		d, ok := ParseRepoSpec(spec)
		if !ok {
			t.Fatalf("%s did not parse", spec)
		}
		_, asset, ok := ResolveDepAsset(context.Background(), d)
		return asset.URL, ok
	}

	if url, ok := resolve("u/ext@v1.0.0"); !ok || url != "https://github.com/u/ext/archive/refs/tags/v1.0.0.zip" {
		t.Errorf("plain spec = %q ok=%v, want the tag's source", url, ok)
	}
	if calls["/repos/u/ext/releases"] != 0 {
		t.Error("a source spec consulted releases")
	}
	if url, ok := resolve("u/ext/release@v1.0.0:*-linux.zip"); !ok || !strings.HasSuffix(url, "/ext-linux.zip") {
		t.Errorf("glob = %q ok=%v, want the linux upload", url, ok)
	}
	if _, ok := resolve("u/ext/release@v1.0.0"); ok {
		t.Error("two uploads and no glob should not pick one")
	}
	if _, ok := resolve("u/ext/release@v1.0.0:*-mac.zip"); ok {
		t.Error("a glob matching nothing should not pick one")
	}
	if _, ok := resolve("u/plain/release@v1.0.0"); ok {
		t.Error("/release with no upload fell back to source")
	}

	d, _ := ParseRepoSpec("u/tagonly/release@v1.0.0")
	if _, err := ResolveVersion(context.Background(), d); err == nil || !strings.Contains(err.Error(), "drop /release") {
		t.Errorf("unreleased tag under /release: err = %v", err)
	}
}

// TestUpdateKeepsSourceOnSource: a newer release's upload is a different package, so a
// source install updates to the newer tag's source archive.
func TestUpdateKeepsSourceOnSource(t *testing.T) {
	bodies, _ := tagInstallHTTP(t)
	bodies["/repos/u/r/releases"] = []byte(`[{"tag_name":"v2.0.0","assets":[{"name":"r.zip","browser_download_url":"https://github.com/u/r/releases/download/v2.0.0/r.zip"}]},{"tag_name":"v1.0.0"}]`)

	src := Addon{URL: "https://github.com/u/r/archive/refs/tags/v1.0.0.zip", Tag: "v1.0.0"}
	plan, res := ResolveUpdate(context.Background(), src, src.Tag)
	if res != ResolvePlan || plan.Asset.URL != "https://github.com/u/r/archive/refs/tags/v2.0.0.zip" {
		t.Errorf("source update = %+v res=%v, want v2.0.0's source", plan, res)
	}

	rel := Addon{URL: "https://github.com/u/r/releases/download/v1.0.0/r.zip", Tag: "v1.0.0"}
	plan, res = ResolveUpdate(context.Background(), rel, rel.Tag)
	if res != ResolvePlan || plan.Asset.URL != "https://github.com/u/r/releases/download/v2.0.0/r.zip" {
		t.Errorf("release update = %+v res=%v, want v2.0.0's upload", plan, res)
	}
}
