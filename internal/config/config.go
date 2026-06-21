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
	ArchiveDir       string         `yaml:"archive_dir,omitempty"`
	CurrentTheme     string         `yaml:"current_theme,omitempty"`      // last-selected TUI theme; loaded at startup, saved on change
	LastSearchSource string         `yaml:"last_search_source,omitempty"` // last-selected Search tab source; loaded at startup, saved on search
	Sources          []SourceConfig `yaml:"sources,omitempty"`            // search sources; the source of truth once dumped
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

// SaveTheme persists name as current_theme in ~/.gdaddon/config.yml. It edits the
// file surgically — only the current_theme key's value is set (or the key appended)
// — so the user's archive_dir, sources block, and any comments survive untouched. A
// missing file is created from DefaultConfig (with the theme overridden), matching
// Ensure's first-run shape.
func SaveTheme(name string) error {
	base, err := Dir()
	if err != nil {
		return err
	}
	path := filepath.Join(base, "config.yml")

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(base, 0o755); err != nil {
			return err
		}
		def := DefaultConfig()
		def.CurrentTheme = name
		out, err := yaml.Marshal(def)
		if err != nil {
			return err
		}
		return os.WriteFile(path, out, 0o644)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	setMappingScalar(&doc, "current_theme", name)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

// SaveLastSource persists name as last_search_source in ~/.gdaddon/config.yml,
// editing the file surgically like SaveTheme so the user's other keys and comments
// survive untouched. A missing file is created from DefaultConfig with the field set.
func SaveLastSource(name string) error {
	base, err := Dir()
	if err != nil {
		return err
	}
	path := filepath.Join(base, "config.yml")

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if err := os.MkdirAll(base, 0o755); err != nil {
			return err
		}
		def := DefaultConfig()
		def.LastSearchSource = name
		out, err := yaml.Marshal(def)
		if err != nil {
			return err
		}
		return os.WriteFile(path, out, 0o644)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return err
	}
	setMappingScalar(&doc, "last_search_source", name)
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

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
