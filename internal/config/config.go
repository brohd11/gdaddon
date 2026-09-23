// Package config loads gdaddon's config from ~/.gdaddon/config/: config.yml (general
// settings) and sources.yml (provider rules). The theme lives in ~/.bubblestack. Files are
// read per call; a missing file yields the zero value, and Ensure writes defaults on first
// run.
package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/brohd11/goutil/configdir"
	"github.com/brohd11/goutil/strutil"
)

// Config is config.yml's general settings, all optional; omitempty keeps the written
// defaults free of blanks.
type Config struct {
	ArchiveDir       string `yaml:"archive_dir,omitempty"`
	LastSearchSource string `yaml:"last_search_source,omitempty"` // last-selected Search tab source; loaded at startup, saved on search
}

// sourcesFile is the parsed ~/.gdaddon/config/sources.yml — the provider rules
// under a top-level sources: key. See LoadSources / DefaultSources.
type sourcesFile struct {
	Sources []SourceConfig `yaml:"sources"`
}

// BinSubdir is the ~/.gdaddon subdirectory the installers put the binary in.
const BinSubdir = "bin"

// Dir is ~/.gdaddon, the home for the config dir, bin/, and the default archive. The
// ~/.<app> convention itself is goutil/configdir's; this pins gdaddon's own name.
func Dir() (string, error) {
	return configdir.Dir("gdaddon")
}

// ConfigDir is ~/.gdaddon/config, the home for config.yml and sources.yml.
func ConfigDir() (string, error) {
	base, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "config"), nil
}

// Ensure writes the default config.yml and sources.yml when missing (delete one to get
// the default back) and returns the paths it created. Existing files are left untouched.
func Ensure() (created []string, err error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	for name, v := range map[string]any{
		"config.yml":  DefaultConfig(),
		"sources.yml": sourcesFile{Sources: DefaultSources()},
	} {
		c, err := configdir.Ensure(dir, name, v)
		if err != nil {
			return created, err
		}
		if c {
			created = append(created, filepath.Join(dir, name))
		}
	}
	slices.Sort(created)
	return created, nil
}

// EnsureGitignore writes ~/.gdaddon/.gitignore ignoring bin/ when none exists (the folder
// is meant to be committable, the binary is not). An existing file is left alone.
func EnsureGitignore() (created bool, path string, err error) {
	base, err := Dir()
	if err != nil {
		return false, "", err
	}
	path = filepath.Join(base, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return false, path, nil // already present — never overwrite the user's file
	} else if !os.IsNotExist(err) {
		return false, path, err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return false, path, err
	}
	if err := os.WriteFile(path, []byte(BinSubdir+"/\n"), 0o644); err != nil {
		return false, path, err
	}
	return true, path, nil
}

// Load reads ~/.gdaddon/config/config.yml. A missing file is not an error — it
// returns the zero Config. A malformed file returns the parse error.
func Load() (*Config, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := configdir.Load(filepath.Join(dir, "config.yml"), &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// LoadSources reads sources.yml; missing yields an empty slice, malformed an error.
func LoadSources() ([]SourceConfig, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	var f sourcesFile
	if err := configdir.Load(filepath.Join(dir, "sources.yml"), &f); err != nil {
		return nil, err
	}
	return f.Sources, nil // nil when the file is absent — callers fall back to DefaultSources
}

// Sources is the effective provider list: sources.yml when it has entries, else
// DefaultSources (also on read errors).
func Sources() []SourceConfig {
	if srcs, err := LoadSources(); err == nil && len(srcs) > 0 {
		return srcs
	}
	return DefaultSources()
}

// saveConfigKey sets one key in config.yml, preserving the rest (configdir.SaveKey). A
// missing file is seeded from DefaultConfig.
func saveConfigKey(key, value string) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	return configdir.SaveKey(filepath.Join(dir, "config.yml"), key, value, DefaultConfig())
}

// SaveLastSource persists name as last_search_source in ~/.gdaddon/config/config.yml
// (surgical edit — see saveConfigKey).
func SaveLastSource(name string) error { return saveConfigKey("last_search_source", name) }

// ResolvedArchiveDir returns archive_dir (with a leading "~" expanded) if set,
// otherwise ~/.gdaddon/archive.
func (c *Config) ResolvedArchiveDir() (string, error) {
	base, err := Dir()
	if err != nil {
		return "", err
	}
	if dir := strings.TrimSpace(c.ArchiveDir); dir != "" {
		return strutil.ExpandHome(dir)
	}
	return filepath.Join(base, "archive"), nil
}
