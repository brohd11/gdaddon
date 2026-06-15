package archive

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gdaddon/internal/source"
)

func TestRepoDir(t *testing.T) {
	if got := repoDir("github.com/owner/repo"); got != "github.com_owner_repo" {
		t.Errorf("repoDir = %q", got)
	}
}

func TestDirDefaultAndConfig(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		got, err := Dir()
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(home, ".gdaddon", "archive") {
			t.Errorf("default dir = %q", got)
		}
	})

	t.Run("config override with ~ expansion", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		if err := os.MkdirAll(filepath.Join(home, ".gdaddon"), 0o755); err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(home, ".gdaddon", "config.yml"), []byte("archive_dir: ~/pkgs\n"), 0o644)
		got, err := Dir()
		if err != nil {
			t.Fatal(err)
		}
		if got != filepath.Join(home, "pkgs") {
			t.Errorf("override dir = %q, want %q", got, filepath.Join(home, "pkgs"))
		}
	})
}

func TestStoreAndList(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	const repoID = "github.com/owner/repo"

	if _, err := Store(repoID, "v1.0.0", "pkg.zip", strings.NewReader("zipdata")); err != nil {
		t.Fatal(err)
	}
	if _, err := Store(repoID, "v1.1.0", "pkg.zip", strings.NewReader("zipdata2")); err != nil {
		t.Fatal(err)
	}

	releases, err := List(repoID)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 2 {
		t.Fatalf("expected 2 archived releases, got %d", len(releases))
	}
	// Newest tag first.
	if releases[0].Tag != "v1.1.0" {
		t.Errorf("expected newest first, got %q", releases[0].Tag)
	}
	a := releases[0].Assets[0]
	if !strings.HasSuffix(a.Name, " - archived") {
		t.Errorf("asset name missing suffix: %q", a.Name)
	}
	if !strings.HasPrefix(a.URL, home) || !strings.HasSuffix(a.URL, "pkg.zip") {
		t.Errorf("asset url should be a local path: %q", a.URL)
	}

	// index.yml is written.
	root, _ := Dir()
	if _, err := os.Stat(filepath.Join(root, "index.yml")); err != nil {
		t.Errorf("index.yml not written: %v", err)
	}

	// Unknown repo -> nil.
	if got, err := List("github.com/none/none"); err != nil || got != nil {
		t.Errorf("unknown repo should be (nil,nil), got (%v,%v)", got, err)
	}
}

func TestMerge(t *testing.T) {
	listing := &source.Listing{Releases: []source.Release{
		{Tag: "v1.0.0", Assets: []source.Asset{{Name: "a.zip", URL: "http://x/a.zip"}}},
	}}
	archived := []source.Release{
		{Tag: "v1.0.0", Assets: []source.Asset{{Name: "a.zip - archived", URL: "/local/a.zip"}}},
		{Tag: "v0.9.0", Assets: []source.Asset{{Name: "old.zip - archived", URL: "/local/old.zip"}}},
	}

	got := Merge(listing, archived)
	if len(got.Releases) != 2 {
		t.Fatalf("expected 2 releases, got %d", len(got.Releases))
	}
	// v1.0.0 gains the archived asset.
	if len(got.Releases[0].Assets) != 2 {
		t.Errorf("v1.0.0 should have 2 assets, got %d", len(got.Releases[0].Assets))
	}
	// v0.9.0 added archive-only.
	if got.Releases[1].Tag != "v0.9.0" {
		t.Errorf("archive-only release not added")
	}

	// Nil listing -> archive-only.
	only := Merge(nil, archived)
	if len(only.Releases) != 2 {
		t.Errorf("nil listing should yield archive-only with 2 releases, got %d", len(only.Releases))
	}
}

func TestArchiveDownloads(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("PK-fake-zip"))
	}))
	defer srv.Close()

	const repoID = "github.com/owner/repo"
	asset := source.Asset{Name: "thing.zip", URL: srv.URL + "/thing.zip"}
	if err := Archive(repoID, "v2.0.0", asset); err != nil {
		t.Fatal(err)
	}

	root, _ := Dir()
	data, err := os.ReadFile(filepath.Join(root, "github.com_owner_repo", "v2.0.0", "thing.zip"))
	if err != nil {
		t.Fatalf("archived file not found: %v", err)
	}
	if string(data) != "PK-fake-zip" {
		t.Errorf("unexpected archived content: %q", data)
	}

	// A local-url asset is a no-op (nothing to fetch).
	if err := Archive(repoID, "v2.0.0", source.Asset{Name: "x.zip", URL: "/already/local.zip"}); err != nil {
		t.Errorf("local asset archive should be a no-op, got %v", err)
	}
}
