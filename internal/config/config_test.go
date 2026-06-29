package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ArchiveDir != "" || len(cfg.Sources) != 0 {
		t.Fatalf("missing file should yield zero Config, got %+v", cfg)
	}
}

func TestResolvedArchiveDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Default: ~/.gdaddon/archive.
	cfg := &Config{}
	got, err := cfg.ResolvedArchiveDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".gdaddon", "archive"); got != want {
		t.Fatalf("default archive dir = %q, want %q", got, want)
	}

	// Override with ~ expansion.
	cfg = &Config{ArchiveDir: "~/pkgs"}
	got, err = cfg.ResolvedArchiveDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, "pkgs"); got != want {
		t.Fatalf("override archive dir = %q, want %q", got, want)
	}
}

func TestEnsureWritesDefaultsOnce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".gdaddon", "config.yml")

	created, got, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if !created {
		t.Fatal("first Ensure should create the file")
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}

	// The dumped file must parse back into the defaults.
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load after Ensure: %v", err)
	}
	if len(cfg.Sources) != len(DefaultSources()) || cfg.Sources[0].Name != "GitHub" {
		t.Fatalf("dumped sources mismatch: %+v", cfg.Sources)
	}

	// Idempotent: a second call leaves the (possibly user-edited) file alone.
	if err := os.WriteFile(path, []byte("archive_dir: ~/custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, _, err = Ensure()
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if created {
		t.Fatal("second Ensure should not recreate the file")
	}
	cfg, _ = Load()
	if cfg.ArchiveDir != "~/custom" {
		t.Fatalf("Ensure overwrote an existing file: %+v", cfg)
	}
}

func TestEnsureGitignore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, ".gdaddon", ".gitignore")

	created, got, err := EnsureGitignore()
	if err != nil {
		t.Fatalf("EnsureGitignore: %v", err)
	}
	if !created {
		t.Fatal("first EnsureGitignore should create the file")
	}
	if got != path {
		t.Fatalf("path = %q, want %q", got, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if want := BinSubdir + "/\n"; string(data) != want {
		t.Fatalf("contents = %q, want %q", data, want)
	}

	// Idempotent: a second call leaves a user-edited file alone.
	if err := os.WriteFile(path, []byte("bin/\nfoo/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, _, err = EnsureGitignore()
	if err != nil {
		t.Fatalf("second EnsureGitignore: %v", err)
	}
	if created {
		t.Fatal("second EnsureGitignore should not recreate the file")
	}
	data, _ = os.ReadFile(path)
	if string(data) != "bin/\nfoo/\n" {
		t.Fatalf("EnsureGitignore overwrote an existing file: %q", data)
	}
}

func TestLoadSources(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	base := filepath.Join(home, ".gdaddon")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	yml := `
archive_dir: ~/pkgs
sources:
  - name: My Store
    type: json
    auth: github
    search:
      url: "https://ex.com/search?q={query}&page={page}"
      page_base: 1
      results_path: items
      fields:
        id: full_name
        title: name
    detail:
      url: "https://ex.com/repo/{id}"
      browse_url_path: clone_url
`
	if err := os.WriteFile(filepath.Join(base, "config.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(cfg.Sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(cfg.Sources))
	}
	s := cfg.Sources[0]
	if s.Name != "My Store" || s.Type != "json" || s.Auth != "github" {
		t.Fatalf("source header mismatch: %+v", s)
	}
	if s.Search.PageBase != 1 || s.Search.ResultsPath != "items" || s.Search.Fields.ID != "full_name" {
		t.Fatalf("search rule mismatch: %+v", s.Search)
	}
	if s.Detail.URL != "https://ex.com/repo/{id}" || s.Detail.BrowseURLPath != "clone_url" {
		t.Fatalf("detail rule mismatch: %+v", s.Detail)
	}
}
