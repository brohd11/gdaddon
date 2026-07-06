package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gdaddon/internal/config"
)

// SetsDir is the directory holding saved "sets": manifest-shaped YAML files the
// user can populate with plugins and later import wholesale into a project. It
// lives under ~/.gdaddon/sets, alongside the global plugin list and the archive.
func SetsDir() (string, error) {
	dir, err := config.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sets"), nil
}

// SetPath returns the file path for the set named name (<SetsDir>/<name>.yml).
func SetPath(name string) (string, error) {
	dir, err := SetsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".yml"), nil
}

// ListSets returns the names (without the .yml suffix) of every set saved under
// SetsDir, sorted. A missing directory reads as an empty list.
func ListSets() ([]string, error) {
	dir, err := SetsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yml") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".yml"))
	}
	sort.Strings(names)
	return names, nil
}

// CreateSet creates a new empty set file named name and returns its path. Like
// CreateManifest it creates the parent dir and refuses to overwrite an existing
// set, so a duplicate name is reported rather than clobbering one.
func CreateSet(name string) (string, error) { return CreateSetFrom(name, "") }

// CreateSetFrom creates a new set named name seeded with the contents of the
// manifest at fromPath (a verbatim copy, preserving each entry's url/path/version),
// and returns its path. An empty fromPath creates an empty set. It refuses to
// overwrite an existing set.
func CreateSetFrom(name, fromPath string) (string, error) {
	path, err := SetPath(name)
	if err != nil {
		return "", err
	}
	if fromPath == "" {
		if err := CreateManifest(path); err != nil {
			return "", err
		}
		return path, nil
	}
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%s already exists", name+".yml")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	data, err := os.ReadFile(fromPath)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// DeleteSet removes the set file named name.
func DeleteSet(name string) error {
	path, err := SetPath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}
