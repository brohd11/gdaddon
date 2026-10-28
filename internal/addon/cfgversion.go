package addon

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"gopkg.in/ini.v1"
)

// ConfigVersion identifies the config selected by the same precedence as inspection.
type ConfigVersion struct {
	Path    string
	Version string
}

// ReadConfigVersion reads an addon's effective version, including [addon] overrides.
// A missing version is empty; a missing config returns os.ErrNotExist.
func ReadConfigVersion(dir string) (ConfigVersion, error) {
	path := pluginCfgPath(dir)
	if path == "" {
		return ConfigVersion{}, fmt.Errorf("no addon config in %s: %w", dir, os.ErrNotExist)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ConfigVersion{}, err
	}
	_, version, _, err := configVersion(data)
	return ConfigVersion{Path: path, Version: version}, err
}

// configVersion also identifies where a missing version belongs: [addon] if present,
// otherwise [plugin]. Loading with the inspection parser keeps read semantics aligned.
func configVersion(data []byte) (*ini.File, string, string, error) {
	cfg, err := parseConfig(data)
	if err != nil {
		return nil, "", "", err
	}
	section := "plugin"
	if s, err := cfg.GetSection("addon"); err == nil {
		section = "addon"
		if s.HasKey("version") {
			return cfg, strings.Trim(strings.TrimSpace(s.Key("version").String()), `'"`), section, nil
		}
	}
	if s, err := cfg.GetSection("plugin"); err == nil && s.HasKey("version") {
		return cfg, strings.Trim(strings.TrimSpace(s.Key("version").String()), `'"`), "plugin", nil
	}
	return cfg, "", section, nil
}

// SetConfigVersion replaces only the effective version value, preserving the rest of
// the file. expectedVersion guards against another version edit while the form is open;
// unrelated edits are retained by reading the file again at save time.
func SetConfigVersion(path, expectedVersion, newVersion string) error {
	newVersion = strings.TrimSpace(newVersion)
	if newVersion == "" {
		return fmt.Errorf("a version is required")
	}
	if strings.ContainsAny(newVersion, "\"'`\\#;") || strings.ContainsFunc(newVersion, unicode.IsControl) {
		return fmt.Errorf("version must be one line without quotes, backslashes, or comment markers")
	}
	// Resolve symlinks so an atomic replacement updates their target, not the link.
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	before, current, section, err := configVersion(data)
	if err != nil {
		return err
	}
	if current != expectedVersion {
		return fmt.Errorf("config version changed from %q to %q; reopen Edit Version/Tag", expectedVersion, current)
	}
	if newVersion == current {
		return nil
	}
	updated, err := replaceConfigVersion(data, section, newVersion)
	if err != nil {
		return err
	}
	after, got, _, err := configVersion(updated)
	if err != nil || got != newVersion || !sameConfigExceptVersion(before, after, section) {
		return fmt.Errorf("cannot safely update this config's version")
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	// Write fully before replacing the original, so a failed write leaves it intact.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gdaddon-version-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(info.Mode().Perm()); err == nil {
		_, err = tmp.Write(updated)
	}
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), path)
}

// replaceConfigVersion locates a single scalar without reserializing the INI file.
// Ambiguous or multiline versions are refused; parsed-key comparison below also guards
// against mistaking section-like text inside another multiline value for a real key.
func replaceConfigVersion(data []byte, section, version string) ([]byte, error) {
	text := string(data)
	current, sections, matches := "", 0, 0
	start, end, insert := 0, 0, -1
	for offset := 0; offset < len(text); {
		next := strings.IndexByte(text[offset:], '\n')
		if next < 0 {
			next = len(text)
		} else {
			next += offset + 1
		}
		line := strings.TrimRight(text[offset:next], "\r\n")
		trimmed := strings.TrimSpace(strings.TrimPrefix(line, "\uFEFF"))
		if strings.HasPrefix(trimmed, "[") {
			if i := strings.IndexByte(trimmed, ']'); i >= 0 {
				current = strings.TrimSpace(trimmed[1:i])
				if current == section {
					sections++
					insert = next
				}
			}
		} else if current == section {
			if eq := strings.IndexByte(line, '='); eq >= 0 && strings.TrimSpace(line[:eq]) == "version" {
				matches++
				value := line[eq+1:]
				start = offset + eq + 1 + len(value) - len(strings.TrimLeft(value, " \t"))
				raw := text[start : offset+len(line)]
				// Default INI loading recognizes # and ; as inline comments.
				if i := strings.IndexAny(raw, "#;"); i >= 0 {
					raw = raw[:i]
				}
				raw = strings.TrimRight(raw, " \t")
				if strings.HasPrefix(raw, `"""`) || strings.HasPrefix(raw, "`") || strings.HasSuffix(raw, "\\") {
					return nil, fmt.Errorf("cannot edit a multiline config version")
				}
				end = start + len(raw)
			}
		}
		offset = next
	}
	if sections > 1 || matches > 1 {
		return nil, fmt.Errorf("cannot safely edit duplicate version keys or sections")
	}
	if matches == 1 {
		value := version
		old := text[start:end]
		if len(old) >= 2 && (old[0] == '"' || old[0] == '\'') && old[len(old)-1] == old[0] {
			value = old[:1] + version + old[:1]
		} else if old == "" {
			value = `"` + version + `"`
		}
		return []byte(text[:start] + value + text[end:]), nil
	}
	eol := "\n"
	if bytes.Contains(data, []byte("\r\n")) {
		eol = "\r\n"
	}
	line := `version="` + version + `"` + eol
	if insert < 0 {
		insert = len(text)
		line = "[" + section + "]" + eol + line
	}
	if insert > 0 && text[insert-1] != '\n' {
		line = eol + line
	}
	return []byte(text[:insert] + line + text[insert:]), nil
}

func sameConfigExceptVersion(before, after *ini.File, section string) bool {
	if after == nil {
		return false
	}
	// Check both directions, allowing only the target version key (and a new
	// target section containing that key) to differ.
	for _, pair := range [][2]*ini.File{{before, after}, {after, before}} {
		for _, s := range pair[0].Sections() {
			other, _ := pair[1].GetSection(s.Name())
			for _, k := range s.Keys() {
				if s.Name() == section && k.Name() == "version" {
					continue
				}
				if other == nil || !other.HasKey(k.Name()) || other.Key(k.Name()).Value() != k.Value() {
					return false
				}
			}
		}
	}
	return true
}
