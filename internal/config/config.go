// Package config loads gdaddon's user config from ~/.gdaddon/config/, split
// across two files: config.yml (general knobs — archive_dir, current_theme,
// last_search_source) and sources.yml (the list of provider rules for search and
// vcs resolution). Each file is read per call — there is no process-wide cache,
// so callers always see the current on-disk state (and tests that swap $HOME keep
// working). A missing file is not an error: it yields the zero value, and Ensure
// dumps defaults on first run so each file becomes the editable source of truth.
package config

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the parsed ~/.gdaddon/config/config.yml — the general knobs. A
// missing file yields the zero value, so every field is optional. omitempty keeps
// the dumped default file (see Ensure) free of blank knobs. The provider rules
// live in a separate file (sources.yml); see LoadSources.
type Config struct {
	ArchiveDir       string `yaml:"archive_dir,omitempty"`
	CurrentTheme     string `yaml:"current_theme,omitempty"`      // last-selected TUI theme; loaded at startup, saved on change
	LastSearchSource string `yaml:"last_search_source,omitempty"` // last-selected Search tab source; loaded at startup, saved on search
}

// sourcesFile is the parsed ~/.gdaddon/config/sources.yml — the provider rules
// under a top-level sources: key. See LoadSources / DefaultSources.
type sourcesFile struct {
	Sources []SourceConfig `yaml:"sources"`
}

// BinSubdir is the ~/.gdaddon subdirectory the release installers copy the OS
// binary into (the permission-free, plugin-launched target). It is the single
// source of truth for the dir name shared by EnsureGitignore and the installers.
const BinSubdir = "bin"

// Dir is ~/.gdaddon, the home for the config dir, bin/, and the default archive.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".gdaddon"), nil
}

// ConfigDir is ~/.gdaddon/config, the home for config.yml and sources.yml.
func ConfigDir() (string, error) {
	base, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "config"), nil
}

// Ensure writes the default config.yml and sources.yml if they don't exist yet,
// creating ~/.gdaddon/config as needed. This makes each file the editable source
// of truth from first run; a user who breaks one can delete it and rerun to get a
// fresh default. It returns the paths it created (empty when both were already
// present). Existing files are left untouched.
func Ensure() (created []string, err error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if c, err := ensureFile(filepath.Join(dir, "config.yml"), DefaultConfig()); err != nil {
		return created, err
	} else if c != "" {
		created = append(created, c)
	}
	if c, err := ensureFile(filepath.Join(dir, "sources.yml"), sourcesFile{Sources: DefaultSources()}); err != nil {
		return created, err
	} else if c != "" {
		created = append(created, c)
	}
	return created, nil
}

// ensureFile writes v (marshalled as YAML) to path if it doesn't exist yet,
// returning the path when it created the file (or "" when already present).
func ensureFile(path string, v any) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return "", nil // already present — never overwrite the user's file
	} else if !os.IsNotExist(err) {
		return "", err
	}
	data, err := yaml.Marshal(v)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// EnsureGitignore writes ~/.gdaddon/.gitignore ignoring the bin/ dir if none
// exists yet, creating ~/.gdaddon as needed. ~/.gdaddon is meant to be
// committable (config.yml, sources, sets); the OS binary is not, so bin/ is
// ignored by default. An existing file is left untouched — a user who wants to
// commit the binary can remove the entry. It returns whether it created the file
// and the file's path.
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
	data, err := os.ReadFile(filepath.Join(dir, "config.yml"))
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

// LoadSources reads the provider rules from ~/.gdaddon/config/sources.yml. A
// missing file is not an error — it returns an empty slice, so callers fall back
// to DefaultSources. A malformed file returns the parse error.
func LoadSources() ([]SourceConfig, error) {
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, "sources.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f sourcesFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return f.Sources, nil
}

// Sources is the effective provider list: the user's sources.yml when present and
// non-empty, else the built-in DefaultSources. It centralizes the "start from
// defaults, override with the file when it has entries" precedence used by both
// search and vcs resolution (an unreadable/malformed file falls back to defaults).
func Sources() []SourceConfig {
	if srcs, err := LoadSources(); err == nil && len(srcs) > 0 {
		return srcs
	}
	return DefaultSources()
}

// saveConfigKey sets key=value in ~/.gdaddon/config/config.yml surgically — only that
// key's value is set (or the key appended) — so the user's other keys and comments
// survive untouched. A missing file is seeded from DefaultConfig (so the other
// defaults are still written), then the key is set, matching Ensure's first-run shape.
func saveConfigKey(key, value string) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "config.yml")

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if data, err = yaml.Marshal(DefaultConfig()); err != nil {
			return err
		}
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	setMappingScalar(&doc, key, value)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// SaveTheme persists name as current_theme in ~/.gdaddon/config/config.yml (surgical
// edit — see saveConfigKey).
func SaveTheme(name string) error { return saveConfigKey("current_theme", name) }

// SaveLastSource persists name as last_search_source in ~/.gdaddon/config/config.yml
// (surgical edit — see saveConfigKey).
func SaveLastSource(name string) error { return saveConfigKey("last_search_source", name) }

// setMappingScalar sets key=value on the top-level mapping of a parsed YAML
// document, overwriting an existing key's value or appending the pair when absent.
// An empty document is initialized to a mapping first.
func setMappingScalar(doc *yaml.Node, key, value string) {
	if len(doc.Content) == 0 {
		doc.Kind = yaml.DocumentNode
		doc.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1].Kind = yaml.ScalarNode
			m.Content[i+1].Tag = "!!str"
			m.Content[i+1].Value = value
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
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
