// Package config loads ~/.gdaddon/config.yml, the single user-config file for
// gdaddon. It owns two things: the archive directory override (archive_dir) and
// the list of user-defined search sources (sources). The file is tiny and read
// per call — there is no process-wide cache, so callers always see the current
// on-disk state (and tests that swap $HOME keep working).
package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the parsed ~/.gdaddon/config.yml. A missing file yields the zero
// value, so every field is optional. omitempty keeps the dumped default file
// (see Ensure) free of blank knobs.
type Config struct {
	ArchiveDir string         `yaml:"archive_dir,omitempty"`
	Sources    []SourceConfig `yaml:"sources,omitempty"` // search sources; the source of truth once dumped
}

// SourceConfig is a declarative search source. A generic provider in
// internal/search interprets it to satisfy the search.Source interface, so a new
// store can be added in YAML without a Go backend.
type SourceConfig struct {
	Name   string     `yaml:"name"`           // display label in the source picker
	Type   string     `yaml:"type"`           // "json" (the only type supported today)
	Auth   string     `yaml:"auth,omitempty"` // "" | "github" → send Bearer $GITHUB_TOKEN
	Search SearchRule `yaml:"search"`
	Detail DetailRule `yaml:"detail"`
}

// SearchRule describes how to fetch and parse a page of results. URL is a
// template; {query}, {page} and {godot_version} are substituted (see
// internal/search/template.go). Extraction is by dotted JSON paths.
type SearchRule struct {
	URL         string     `yaml:"url"`
	PageBase    int        `yaml:"page_base,omitempty"`     // value of {page} for the first page (0 or 1)
	OmitIfEmpty []string   `yaml:"omit_if_empty,omitempty"` // drop these query params when their value is empty
	ResultsPath string     `yaml:"results_path"`            // dotted path to the result array
	Fields      FieldPaths `yaml:"fields"`                  // dotted paths within each array element
	PagePath    string     `yaml:"page_path,omitempty"`     // dotted path → current page number
	PagesPath   string     `yaml:"pages_path,omitempty"`    // dotted path → total pages
	TotalPath   string     `yaml:"total_path,omitempty"`    // dotted path → total item count
	PerPage     int        `yaml:"per_page,omitempty"`      // used to derive Pages from TotalPath when PagesPath is unset
}

// FieldPaths maps each Summary field to a dotted JSON path within a result
// element. Empty paths are skipped.
type FieldPaths struct {
	ID            string `yaml:"id,omitempty"`
	Title         string `yaml:"title,omitempty"`
	Author        string `yaml:"author,omitempty"`
	Category      string `yaml:"category,omitempty"`
	Cost          string `yaml:"cost,omitempty"`
	GodotVersion  string `yaml:"godot_version,omitempty"`
	VersionString string `yaml:"version_string,omitempty"`
}

// DetailRule describes the per-asset fetch that yields the repo URL. URL is a
// template with {id} (the Summary.ID from search). BrowseURLPath is the only
// load-bearing field — it must resolve to a URL the installer accepts.
type DetailRule struct {
	URL             string `yaml:"url"`
	BrowseURLPath   string `yaml:"browse_url_path"`
	DownloadURLPath string `yaml:"download_url_path,omitempty"`
	DescriptionPath string `yaml:"description_path,omitempty"`
	TitlePath       string `yaml:"title_path,omitempty"`
	AuthorPath      string `yaml:"author_path,omitempty"`
}

// Dir is ~/.gdaddon, the home for config.yml and the default archive.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gdaddon"), nil
}

// Ensure writes the default config.yml if none exists yet, creating ~/.gdaddon
// as needed. This makes the file the editable source of truth from first run; a
// user who breaks it can delete it and rerun to get a fresh default. It returns
// whether it created the file and the file's path. An existing file is left
// untouched.
func Ensure() (created bool, path string, err error) {
	base, err := Dir()
	if err != nil {
		return false, "", err
	}
	path = filepath.Join(base, "config.yml")
	if _, err := os.Stat(path); err == nil {
		return false, path, nil // already present — never overwrite the user's file
	} else if !os.IsNotExist(err) {
		return false, path, err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return false, path, err
	}
	data, err := yaml.Marshal(DefaultConfig())
	if err != nil {
		return false, path, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false, path, err
	}
	return true, path, nil
}

// DefaultConfig is the config dumped on first run: the default archive dir plus
// the built-in search sources. Add future defaults here so Ensure picks them up.
func DefaultConfig() *Config {
	return &Config{
		ArchiveDir: "~/.gdaddon/archive",
		Sources:    DefaultSources(),
	}
}

// DefaultSources are the built-in search source rules. They use the same schema
// as user sources, so the dumped file doubles as a worked example. The Asset
// Store stays a hard-coded Go backend (its HTML scrape can't be expressed here)
// and is appended by internal/search, not listed here.
func DefaultSources() []SourceConfig {
	return []SourceConfig{
		{
			Name: "GitHub",
			Type: "json",
			Auth: "github",
			Search: SearchRule{
				URL:         "https://api.github.com/search/repositories?q={query}&per_page=20&page={page}",
				PageBase:    1,
				ResultsPath: "items",
				PerPage:     20,
				TotalPath:   "total_count",
				Fields: FieldPaths{
					ID:            "full_name",
					Title:         "full_name",
					Author:        "owner.login",
					VersionString: "default_branch",
				},
			},
			Detail: DetailRule{
				URL:             "https://api.github.com/repos/{id}",
				BrowseURLPath:   "clone_url", // ends in .git → accepted by the installer
				DescriptionPath: "description",
				TitlePath:       "full_name",
				AuthorPath:      "owner.login",
			},
		},
		{
			Name: "Asset Library",
			Type: "json",
			Search: SearchRule{
				URL:         "https://godotengine.org/asset-library/api/asset?filter={query}&type=addon&max_results=20&page={page}&sort=updated&godot_version={godot_version}",
				OmitIfEmpty: []string{"godot_version"},
				ResultsPath: "result",
				PagePath:    "page",
				PagesPath:   "pages",
				TotalPath:   "total_items",
				Fields: FieldPaths{
					ID:            "asset_id",
					Title:         "title",
					Author:        "author",
					Category:      "category",
					Cost:          "cost",
					GodotVersion:  "godot_version",
					VersionString: "version_string",
				},
			},
			Detail: DetailRule{
				URL:             "https://godotengine.org/asset-library/api/asset/{id}",
				BrowseURLPath:   "browse_url",
				DownloadURLPath: "download_url",
				DescriptionPath: "description",
				TitlePath:       "title",
				AuthorPath:      "author",
			},
		},
	}
}

// Load reads ~/.gdaddon/config.yml. A missing file is not an error — it returns
// the zero Config. A malformed file returns the parse error.
func Load() (*Config, error) {
	base, err := Dir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(base, "config.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ResolvedArchiveDir returns archive_dir (with a leading "~" expanded) if set,
// otherwise ~/.gdaddon/archive.
func (c *Config) ResolvedArchiveDir() (string, error) {
	base, err := Dir()
	if err != nil {
		return "", err
	}
	if dir := strings.TrimSpace(c.ArchiveDir); dir != "" {
		return ExpandHome(dir)
	}
	return filepath.Join(base, "archive"), nil
}

// ExpandHome expands a leading "~" or "~/" to the user's home directory.
func ExpandHome(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/")), nil
	}
	return path, nil
}
