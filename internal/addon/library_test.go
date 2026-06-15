package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeriveName(t *testing.T) {
	cases := map[string]string{
		"https://github.com/u/Foo.git":           "Foo",
		"https://github.com/u/Foo":               "Foo",
		"https://github.com/u/Foo/":              "Foo",
		"https://github.com/u/Foo/archive/x.zip": "x",
		"https://example.com/bar/baz-plugin.zip": "baz-plugin",
		"":                                       "plugin",
	}
	for in, want := range cases {
		if got := DeriveName(in); got != want {
			t.Errorf("DeriveName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeRepoURL(t *testing.T) {
	cases := map[string]string{
		"https://github.com/u/Foo":     "https://github.com/u/Foo.git",
		"https://github.com/u/Foo/":    "https://github.com/u/Foo.git",
		"https://github.com/u/Foo.git": "https://github.com/u/Foo.git",
		"https://example.com/a/b.zip":  "https://example.com/a/b.zip",
	}
	for in, want := range cases {
		if got := NormalizeRepoURL(in); got != want {
			t.Errorf("NormalizeRepoURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddEntryProjectAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AddEntry(path, "NewAddon", "https://github.com/u/NewAddon.git", "addons/NewAddon"); err != nil {
		t.Fatal(err)
	}

	got := string(mustRead(t, path))
	t.Logf("\n%s", got)

	if !strings.Contains(got, "\n\nNewAddon:\n    url: https://github.com/u/NewAddon.git\n    path: addons/NewAddon\n") {
		t.Errorf("new block not appended as expected; got:\n%s", got)
	}
	// Existing entries survive verbatim.
	if !strings.Contains(got, `version: "1.0.1"`) || !strings.Contains(got, "Terrain3D:") {
		t.Errorf("existing entries mutated; got:\n%s", got)
	}
}

func TestAddEntryGlobalURLOnly(t *testing.T) {
	dir := t.TempDir()
	// Nested path that doesn't exist yet, to exercise MkdirAll.
	path := filepath.Join(dir, ".gdaddon", "plugins.yml")

	if err := AddEntry(path, "Foo", "https://github.com/u/Foo.git", ""); err != nil {
		t.Fatal(err)
	}

	got := string(mustRead(t, path))
	if got != "Foo:\n    url: https://github.com/u/Foo.git\n" {
		t.Errorf("unexpected url-only content; got:\n%q", got)
	}
	if strings.Contains(got, "path:") {
		t.Errorf("url-only entry should not contain a path line; got:\n%s", got)
	}
}

func TestAddEntrySameRepoRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	// Existing entry installed from a release .zip of repo brohd11/Foo.
	os.WriteFile(path, []byte("Foo:\n    url: https://github.com/brohd11/Foo/releases/download/1.0.0/foo-1.0.0.zip\n"), 0o644)

	// Adding the same repo under a different key via its .git url must be rejected.
	err := AddEntry(path, "foo-git", "https://github.com/brohd11/Foo.git", "")
	if err == nil {
		t.Fatal("expected same-repo rejection, got nil")
	}
	if !strings.Contains(err.Error(), "already added from") {
		t.Errorf("unexpected error: %v", err)
	}
	if strings.Contains(string(mustRead(t, path)), "foo-git") {
		t.Errorf("entry should not have been written")
	}
}

func TestAddEntryDuplicateName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	if err := os.WriteFile(path, []byte(sampleManifest), 0o644); err != nil {
		t.Fatal(err)
	}

	err := AddEntry(path, "Terrain3D", "https://github.com/u/Terrain3D.git", "addons/Terrain3D")
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
	// File must be unchanged on the duplicate error.
	if string(mustRead(t, path)) != sampleManifest {
		t.Errorf("file should be unchanged on duplicate error")
	}
}
