package source

import "testing"

func TestAutoAsset(t *testing.T) {
	upload := Asset{Name: "addon.zip", URL: "https://h/dl/addon.zip"}
	upload2 := Asset{Name: "other.zip", URL: "https://h/dl/other.zip"}
	gen := Asset{Name: "Source code (zip)", URL: "https://h/archive.zip", Generated: true}

	t.Run("single uploaded asset wins over the source archive", func(t *testing.T) {
		got, ok := AutoAsset(Release{Assets: []Asset{upload, gen}})
		if !ok || got.URL != upload.URL {
			t.Errorf("got %q ok=%v, want the uploaded asset", got.URL, ok)
		}
	})
	t.Run("no uploaded asset falls back to the generated source archive", func(t *testing.T) {
		got, ok := AutoAsset(Release{Assets: []Asset{gen}})
		if !ok || got.URL != gen.URL {
			t.Errorf("got %q ok=%v, want the source archive", got.URL, ok)
		}
	})
	t.Run("multiple uploaded assets are ambiguous", func(t *testing.T) {
		if _, ok := AutoAsset(Release{Assets: []Asset{upload, upload2, gen}}); ok {
			t.Errorf("expected ok=false for 2+ uploaded assets")
		}
	})
	t.Run("empty release", func(t *testing.T) {
		if _, ok := AutoAsset(Release{}); ok {
			t.Errorf("expected ok=false for a release with no assets")
		}
	})
}
