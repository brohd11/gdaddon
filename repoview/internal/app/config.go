package app

import (
	"os"
	"path/filepath"
	"strings"
)

// repoview keeps its own tiny state dir (~/.repoview), separate from gdaddon's ~/.gdaddon, so
// the two tools never touch each other's config. The only thing persisted today is the
// selected theme, stored as a one-line plain-text file — no config format, no yaml dependency.

// configDir is ~/.repoview.
func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".repoview"), nil
}

// loadTheme returns the persisted theme name, or "" when none is saved (which leaves
// bubblestack's default). Any read error degrades to "" rather than failing startup.
func loadTheme() string {
	dir, err := configDir()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(dir, "theme"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// saveTheme persists name as the startup theme. The error is returned but callers drop it —
// a live theme switch must never block on persistence.
func saveTheme(name string) error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "theme"), []byte(name+"\n"), 0o644)
}
