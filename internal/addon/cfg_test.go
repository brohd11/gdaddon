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

	t.Run("addon.cfg only", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "addon.cfg")
		if !hasPluginCfg(dir) || filepath.Base(pluginCfgPath(dir)) != "addon.cfg" {
			t.Errorf("addon.cfg not detected")
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

	t.Run("plugin.cfg beats addon.cfg beats version.cfg", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "addon.cfg")
		write(dir, "version.cfg")
		if filepath.Base(pluginCfgPath(dir)) != "addon.cfg" {
			t.Errorf("addon.cfg should be preferred over version.cfg")
		}
		write(dir, "plugin.cfg")
		if filepath.Base(pluginCfgPath(dir)) != "plugin.cfg" {
			t.Errorf("plugin.cfg should be preferred over addon.cfg")
		}
	})
}

func TestCfgSection(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"addon section", "[addon]\nversion=\"1.0.0\"\n", "1.0.0"},
		{"plugin section", "[plugin]\nversion=\"2.0.0\"\n", "2.0.0"},
		{"addon wins over plugin", "[plugin]\nversion=\"2.0.0\"\n[addon]\nversion=\"1.0.0\"\n", "1.0.0"},
	}
	for _, file := range pluginCfgNames {
		for _, c := range cases {
			t.Run(file+"/"+c.name, func(t *testing.T) {
				dir := t.TempDir()
				if err := os.WriteFile(filepath.Join(dir, file), []byte(c.body), 0o644); err != nil {
					t.Fatal(err)
				}
				if got := getLocalPluginVersion(dir); got != c.want {
					t.Errorf("version = %q, want %q", got, c.want)
				}
			})
		}
	}
}

func TestCfgSectionMerge(t *testing.T) {
	for _, file := range pluginCfgNames {
		t.Run(file, func(t *testing.T) {
			dir := t.TempDir()
			body := "[plugin]\nname=\"base\"\nversion=\"2.0.0\"\n[addon]\nname=\"x\"\n"
			if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := getLocalPluginVersion(dir); got != "2.0.0" {
				t.Errorf("version = %q, want 2.0.0 from [plugin]", got)
			}
			if got := getLocalPluginName(dir); got != "x" {
				t.Errorf("name = %q, want x from [addon]", got)
			}
		})
	}
}

func TestDependenciesMergedSections(t *testing.T) {
	dir := t.TempDir()
	body := "[plugin]\nversion=\"1.0.0\"\ndeps=[\"a/old\"]\n[addon]\nrequire=[\"owner/repo\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "plugin.cfg"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, err := Dependencies(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].Repo != "repo" {
		t.Fatalf("deps = %+v, want only owner/repo from [addon] require", deps)
	}
}

func TestDependenciesFromAddonCfg(t *testing.T) {
	dir := t.TempDir()
	body := "[addon]\nrequire=[\"owner/repo\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "addon.cfg"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, err := Dependencies(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 {
		t.Fatalf("deps = %+v, want one", deps)
	}
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
