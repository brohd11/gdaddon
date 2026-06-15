package addon

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// buildZip returns an in-memory zip whose entries are the given path→content map.
func buildZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInstallZipEndToEnd(t *testing.T) {
	// A GitHub-style archive: single wrapper folder, addon under addons/.
	data := buildZip(t, map[string]string{
		"MyRepo-1.0.0/README.md":                   "hi",
		"MyRepo-1.0.0/addons/my_addon/plugin.cfg":  "[plugin]\nname=\"My Addon\"\nversion=\"1.0.0\"\n",
		"MyRepo-1.0.0/addons/my_addon/my_addon.gd": "extends Node",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(data)
	}))
	defer srv.Close()

	project := t.TempDir()
	a := Addon{Name: "Whatever", URL: srv.URL + "/archive.zip"}

	res, err := Install(a, project, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "addons/my_addon" {
		t.Errorf("Path = %q, want addons/my_addon", res.Path)
	}
	if res.Version != "1.0.0" {
		t.Errorf("Version = %q, want 1.0.0", res.Version)
	}
	// Files landed at the resolved path; the wrapper folder is gone.
	if _, err := os.Stat(filepath.Join(project, "addons/my_addon/plugin.cfg")); err != nil {
		t.Errorf("plugin.cfg not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(project, "MyRepo-1.0.0")); err == nil {
		t.Errorf("wrapper folder should not be installed")
	}
}

func TestInstallLocalZip(t *testing.T) {
	// A local archive zip (as produced by the archive feature) installs without
	// any network access.
	data := buildZip(t, map[string]string{
		"MyRepo-1.0.0/addons/my_addon/plugin.cfg": "[plugin]\nversion=\"1.0.0\"\n",
	})
	zipPath := filepath.Join(t.TempDir(), "pkg.zip")
	if err := os.WriteFile(zipPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	project := t.TempDir()
	res, err := Install(Addon{Name: "Whatever", URL: zipPath}, project, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "addons/my_addon" || res.Version != "1.0.0" {
		t.Errorf("got Path=%q Version=%q", res.Path, res.Version)
	}
	if _, err := os.Stat(filepath.Join(project, "addons/my_addon/plugin.cfg")); err != nil {
		t.Errorf("plugin.cfg not installed from local zip: %v", err)
	}
	// The source archive must be left intact (cleanup is a no-op for local zips).
	if _, err := os.Stat(zipPath); err != nil {
		t.Errorf("local archive zip should not be deleted: %v", err)
	}
}
