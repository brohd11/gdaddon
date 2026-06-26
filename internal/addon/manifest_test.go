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
	if err := UpdateEntry(path, "Terrain3D", newURL, "", "1.0.2", ""); err != nil {
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

func TestEditEntrySetAndClear(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set tag (insert new line), keep url/path, and clear version (blank ⇒ remove).
	if err := EditEntry(path, "Terrain3D",
		"https://github.com/TokisanGames/Terrain3D/releases/download/v1.0.1/Terrain3D_v1.0.1.zip",
		"addons/terrain_3d", "", "v1.0.1"); err != nil {
		t.Fatal(err)
	}

	out, _ := os.ReadFile(path)
	got := string(out)
	t.Logf("\n%s", got)

	if strings.Contains(got, `version: "1.0.1"`) {
		t.Errorf("cleared version line should be removed; got:\n%s", got)
	}
	if !strings.Contains(got, `tag: "v1.0.1"`) {
		t.Errorf("tag not inserted; got:\n%s", got)
	}
	if !strings.Contains(got, "\n    path: addons/terrain_3d") {
		t.Errorf("path should be untouched; got:\n%s", got)
	}
	// Other entry must survive verbatim.
	if !strings.Contains(got, `version: "0.1.0"`) || !strings.Contains(got, "\n\nPluginDevTools:") {
		t.Errorf("other entry mutated; got:\n%s", got)
	}
}

func TestSetCloneFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// Insert clone: true.
	if err := SetCloneFlag(path, "Terrain3D", true); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "\n    clone: true") {
		t.Fatalf("clone line not inserted with indentation; got:\n%s", got)
	}
	// Untouched entry survives.
	if !strings.Contains(string(got), `version: "0.1.0"`) {
		t.Errorf("other entry mutated; got:\n%s", got)
	}

	// Parse reads it back as a bool.
	addons, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range addons {
		if a.Name == "Terrain3D" && !a.Clone {
			t.Errorf("Clone not parsed as true")
		}
		if a.Name == "PluginDevTools" && a.Clone {
			t.Errorf("Clone leaked onto another entry")
		}
	}

	// Idempotent: setting true again does not duplicate the line.
	if err := SetCloneFlag(path, "Terrain3D", true); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if strings.Count(string(got), "clone: true") != 1 {
		t.Errorf("clone line duplicated; got:\n%s", got)
	}

	// Clearing removes the line.
	if err := SetCloneFlag(path, "Terrain3D", false); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(path)
	if strings.Contains(string(got), "clone:") {
		t.Errorf("clone line not removed; got:\n%s", got)
	}
}

func TestUpdateEntryEmptyURLPreservesURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	origURL := "url: https://github.com/TokisanGames/Terrain3D/releases/download/v1.0.1/Terrain3D_v1.0.1.zip"
	if err := UpdateEntry(path, "Terrain3D", "", "", "1.0.2", ""); err != nil {
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

func TestUpdateEntryInsertsPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	// A url-only entry (no path/version yet), as written by AddEntry.
	const urlOnly = "Libby:\n    url: https://github.com/u/Libby.git\n"
	if err := os.WriteFile(path, []byte(urlOnly), 0o644); err != nil {
		t.Fatal(err)
	}

	// Empty url leaves the url untouched; path + version are inserted.
	if err := UpdateEntry(path, "Libby", "", "addons/libby", "1.0.0", ""); err != nil {
		t.Fatal(err)
	}

	got := string(mustRead(t, path))
	t.Logf("\n%s", got)
	if !strings.Contains(got, "url: https://github.com/u/Libby.git") {
		t.Errorf("url should be untouched; got:\n%s", got)
	}
	if !strings.Contains(got, "\n    path: addons/libby") {
		t.Errorf("path not inserted; got:\n%s", got)
	}
	if !strings.Contains(got, `version: "1.0.0"`) {
		t.Errorf("version not inserted; got:\n%s", got)
	}
}

func TestUpdateEntryTag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pin a tag onto an entry that has none; version arg empty must leave the
	// existing version untouched (no `version: ""` written).
	if err := UpdateEntry(path, "Terrain3D", "", "", "", "v1.0.1"); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(t, path))
	t.Logf("\n%s", got)
	if !strings.Contains(got, `tag: "v1.0.1"`) {
		t.Errorf("tag not written; got:\n%s", got)
	}
	if !strings.Contains(got, `version: "1.0.1"`) {
		t.Errorf("existing version should be untouched; got:\n%s", got)
	}
	if strings.Contains(got, `version: ""`) {
		t.Errorf("empty version arg should not write an empty version line; got:\n%s", got)
	}

	// Empty tag arg on a later call leaves the tag untouched.
	if err := UpdateEntry(path, "Terrain3D", "", "", "1.0.2", ""); err != nil {
		t.Fatal(err)
	}
	got = string(mustRead(t, path))
	if !strings.Contains(got, `tag: "v1.0.1"`) {
		t.Errorf("tag should be untouched when tag arg empty; got:\n%s", got)
	}
	if !strings.Contains(got, `version: "1.0.2"`) {
		t.Errorf("version not updated; got:\n%s", got)
	}
}

func TestAddEntryWithTagNoVersion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	// A tagged dependency add: url + tag, no version line yet.
	if err := AddEntryFull(path, Addon{Name: "Dep", URL: "https://github.com/u/Dep/releases/download/v1.2.0/dep.zip", Tag: "v1.2.0"}); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(t, path))
	t.Logf("\n%s", got)
	if !strings.Contains(got, "Dep:") || !strings.Contains(got, `tag: "v1.2.0"`) {
		t.Errorf("expected a tagged Dep entry; got:\n%s", got)
	}
	if strings.Contains(got, "version:") {
		t.Errorf("no version line should be written for a tag-only add; got:\n%s", got)
	}
}

func TestRemoveEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	// Remove the first entry; the second must survive verbatim.
	if err := RemoveEntry(path, "Terrain3D"); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(t, path))
	t.Logf("\n%s", got)
	if strings.Contains(got, "Terrain3D:") {
		t.Errorf("entry not removed; got:\n%s", got)
	}
	if !strings.Contains(got, "PluginDevTools:") || !strings.Contains(got, `version: "0.1.0"`) {
		t.Errorf("other entry mutated; got:\n%s", got)
	}

	// Removing the last remaining entry empties the manifest.
	if err := RemoveEntry(path, "PluginDevTools"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(mustRead(t, path))); got != "" {
		t.Errorf("manifest should be empty after removing all entries; got:\n%s", got)
	}
}

func TestRemoveEntryNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RemoveEntry(path, "Nope"); err == nil {
		t.Errorf("expected an error removing a missing entry")
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
