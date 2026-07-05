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
	if cfg.ArchiveDir != "" || cfg.CurrentTheme != "" {
		t.Fatalf("missing file should yield zero Config, got %+v", cfg)
	}
	srcs, err := LoadSources()
	if err != nil {
		t.Fatalf("LoadSources() error = %v", err)
	}
	if len(srcs) != 0 {
		t.Fatalf("missing file should yield no sources, got %d", len(srcs))
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
	dir := filepath.Join(home, ".gdaddon", "config")
	configPath := filepath.Join(dir, "config.yml")
	sourcesPath := filepath.Join(dir, "sources.yml")

	created, err := Ensure()
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(created) != 2 || created[0] != configPath || created[1] != sourcesPath {
		t.Fatalf("first Ensure should create both files, got %v", created)
	}

	// The dumped sources file must parse back into the defaults.
	srcs, err := LoadSources()
	if err != nil {
		t.Fatalf("LoadSources after Ensure: %v", err)
	}
	if len(srcs) != len(DefaultSources()) || srcs[0].Name != "GitHub" {
		t.Fatalf("dumped sources mismatch: %+v", srcs)
	}

	// Idempotent: a second call leaves the (possibly user-edited) files alone.
	if err := os.WriteFile(configPath, []byte("archive_dir: ~/custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	created, err = Ensure()
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("second Ensure should not recreate any file, got %v", created)
	}
	cfg, _ := Load()
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
	dir := filepath.Join(home, ".gdaddon", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yml := `
sources:
  - name: My Store
    type: json
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
	if err := os.WriteFile(filepath.Join(dir, "sources.yml"), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}

	srcs, err := LoadSources()
	if err != nil {
		t.Fatalf("LoadSources() error = %v", err)
	}
	if len(srcs) != 1 {
		t.Fatalf("got %d sources, want 1", len(srcs))
	}
	s := srcs[0]
	if s.Name != "My Store" || s.Type != "json" {
		t.Fatalf("source header mismatch: %+v", s)
	}
	if s.Search.PageBase != 1 || s.Search.ResultsPath != "items" || s.Search.Fields.ID != "full_name" {
		t.Fatalf("search rule mismatch: %+v", s.Search)
	}
	if s.Detail.URL != "https://ex.com/repo/{id}" || s.Detail.BrowseURLPath != "clone_url" {
		t.Fatalf("detail rule mismatch: %+v", s.Detail)
	}
}
