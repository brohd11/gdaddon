package addon

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A config-less package (no plugin.cfg/version.cfg, e.g. an icon pack) is stamped
// with a synthetic version.cfg from the install tag, so its version reads back and
// Inspect reports it as installed rather than a perpetual mismatch.
func TestInstallStampsConfiglessVersion(t *testing.T) {
	data := buildZip(t, map[string]string{
		"MyRepo-1.1.1/README.md":                "docs",
		"MyRepo-1.1.1/addons/at_icons/icon.svg": "<svg/>",
	})
	zipPath := filepath.Join(t.TempDir(), "pkg.zip")
	if err := os.WriteFile(zipPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	project := t.TempDir()
	res, err := Install(context.Background(), Addon{Name: "at_icons", URL: zipPath, Tag: "v1.1.1"}, project, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != "addons/at_icons" {
		t.Fatalf("Path = %q, want addons/at_icons", res.Path)
	}
	// The stamped version.cfg makes the version read back as the normalized tag.
	if res.Version != "1.1.1" {
		t.Errorf("Version = %q, want 1.1.1 (stamped)", res.Version)
	}
	dest := filepath.Join(project, "addons/at_icons")
	if got := getLocalPluginVersion(dest); got != "1.1.1" {
		t.Errorf("getLocalPluginVersion = %q, want 1.1.1", got)
	}
	if _, err := os.Stat(filepath.Join(dest, "version.cfg")); err != nil {
		t.Errorf("version.cfg not stamped: %v", err)
	}

	// End-to-end: a manifest pinning that version now reads as installed, not mismatch.
	manifest := filepath.Join(project, "addon_manifest.yml")
	os.WriteFile(manifest, []byte("at_icons:\n  url: "+zipPath+"\n  path: addons/at_icons\n  version: \"1.1.1\"\n"), 0o644)
	statuses, err := Inspect(manifest, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || statuses[0].State != StateInstalled {
		t.Fatalf("state = %v, want StateInstalled", statuses[0].State)
	}
}

func TestStampVersion(t *testing.T) {
	t.Run("writes version.cfg for a config-less dir", func(t *testing.T) {
		dir := t.TempDir()
		stampVersion(dir, "v2.3.4")
		if got := getLocalPluginVersion(dir); got != "2.3.4" {
			t.Errorf("got %q, want 2.3.4", got)
		}
	})

	t.Run("never clobbers an existing plugin.cfg", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "plugin.cfg"), []byte("[plugin]\nversion=\"9.9.9\"\n"), 0o644)
		stampVersion(dir, "1.0.0")
		if got := getLocalPluginVersion(dir); got != "9.9.9" {
			t.Errorf("authored config overwritten; got %q, want 9.9.9", got)
		}
		if _, err := os.Stat(filepath.Join(dir, "version.cfg")); !os.IsNotExist(err) {
			t.Errorf("version.cfg should not be written when a config exists")
		}
	})

	t.Run("empty version writes nothing", func(t *testing.T) {
		dir := t.TempDir()
		stampVersion(dir, "")
		if _, err := os.Stat(filepath.Join(dir, "version.cfg")); !os.IsNotExist(err) {
			t.Errorf("empty version should write no version.cfg")
		}
	})
}

func TestIntendedVersion(t *testing.T) {
	if got := intendedVersion(Addon{Tag: "v1.2.3", Version: "9.9.9"}); got != "v1.2.3" {
		t.Errorf("tag should win; got %q", got)
	}
	if got := intendedVersion(Addon{Version: "9.9.9"}); got != "9.9.9" {
		t.Errorf("version fallback; got %q", got)
	}
	if got := intendedVersion(Addon{}); got != "" {
		t.Errorf("no intent; got %q", got)
	}
}
