package addon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPluginCfgPath(t *testing.T) {
	write := func(dir, name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("[plugin]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("plugin.cfg", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "plugin.cfg")
		if !hasPluginCfg(dir) || filepath.Base(pluginCfgPath(dir)) != "plugin.cfg" {
			t.Errorf("plugin.cfg not detected")
		}
	})

	t.Run("version.cfg only", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "version.cfg")
		if !hasPluginCfg(dir) || filepath.Base(pluginCfgPath(dir)) != "version.cfg" {
			t.Errorf("version.cfg not detected")
		}
	})

	t.Run("neither", func(t *testing.T) {
		dir := t.TempDir()
		if hasPluginCfg(dir) || pluginCfgPath(dir) != "" {
			t.Errorf("expected no config detected")
		}
	})

	t.Run("plugin.cfg wins when both present", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "plugin.cfg")
		write(dir, "version.cfg")
		if filepath.Base(pluginCfgPath(dir)) != "plugin.cfg" {
			t.Errorf("plugin.cfg should be preferred over version.cfg")
		}
	})
}

func TestGetLocalPluginVersionFromVersionCfg(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "version.cfg"), []byte("[plugin]\nversion=\"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := getLocalPluginVersion(dir); got != "1.0.0" {
		t.Errorf("version from version.cfg = %q, want 1.0.0", got)
	}
}
