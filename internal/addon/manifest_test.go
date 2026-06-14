package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleManifest = `Terrain3D:
    url: https://github.com/TokisanGames/Terrain3D/releases/download/v1.0.1/Terrain3D_v1.0.1.zip
    path: addons/terrain_3d
    version: "1.0.1"

PluginDevTools:
    url: https://github.com/brohd11/godot-plugin-devtools/archive/refs/heads/main-dev.zip
    path: addons/plugin_devtools
    version: "0.1.0"
`

func TestUpdateEntryPreservesFormatting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	newURL := "https://github.com/TokisanGames/Terrain3D/releases/download/v1.0.2-stable/Terrain3D_v1.0.2-stable.zip"
	if err := UpdateEntry(path, "Terrain3D", newURL, "1.0.2"); err != nil {
		t.Fatal(err)
	}

	out, _ := os.ReadFile(path)
	got := string(out)
	t.Logf("\n%s", got)

	if !strings.Contains(got, newURL) {
		t.Errorf("url not updated")
	}
	if !strings.Contains(got, `version: "1.0.2"`) {
		t.Errorf("version not updated or lost quoting; got:\n%s", got)
	}
	// Untouched entry must survive verbatim, key order preserved.
	if !strings.Contains(got, `version: "0.1.0"`) {
		t.Errorf("other entry mutated")
	}
	if strings.Index(got, "Terrain3D:") > strings.Index(got, "PluginDevTools:") {
		t.Errorf("key order changed")
	}
	// 4-space indentation preserved.
	if !strings.Contains(got, "\n    path: addons/terrain_3d") {
		t.Errorf("indentation changed; got:\n%s", got)
	}
	// Blank line between entries preserved.
	if !strings.Contains(got, "\n\nPluginDevTools:") {
		t.Errorf("blank line between entries lost; got:\n%s", got)
	}
}

func TestUpdateEntryEmptyURLPreservesURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	origURL := "url: https://github.com/TokisanGames/Terrain3D/releases/download/v1.0.1/Terrain3D_v1.0.1.zip"
	if err := UpdateEntry(path, "Terrain3D", "", "1.0.2"); err != nil {
		t.Fatal(err)
	}

	got := string(mustRead(t, path))
	if !strings.Contains(got, origURL) {
		t.Errorf("url should be untouched when url arg is empty; got:\n%s", got)
	}
	if !strings.Contains(got, `version: "1.0.2"`) {
		t.Errorf("version not updated; got:\n%s", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
