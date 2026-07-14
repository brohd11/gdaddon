package addon

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// mkCfg creates dir (relative to root) with a minimal config file (cfgName)
// inside it.
func mkCfg(t *testing.T, root, dir, cfgName string) {
	t.Helper()
	full := filepath.Join(root, dir)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, cfgName), []byte("[plugin]\nversion=\"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// mkPlugin creates dir (relative to root) with a minimal plugin.cfg inside it.
func mkPlugin(t *testing.T, root, dir string) { mkCfg(t, root, dir, "plugin.cfg") }

// mkCfgWith creates dir (relative to root) with a config file (cfgName) carrying an
// extra [plugin] line (e.g. `dir="addons/x"`) alongside the version.
func mkCfgWith(t *testing.T, root, dir, cfgName, extra string) {
	t.Helper()
	full := filepath.Join(root, dir)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "[plugin]\nversion=\"1.0.0\"\n" + extra + "\n"
	if err := os.WriteFile(filepath.Join(full, cfgName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func destSet(ps []placement) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.destRel
	}
	sort.Strings(out)
	return out
}

func TestResolveInstall(t *testing.T) {
	t.Run("addon under addons/", func(t *testing.T) {
		root := t.TempDir()
		mkPlugin(t, root, "addons/my_addon")
		ps := resolveInstall(root, "Whatever", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/my_addon" {
			t.Fatalf("got %+v", ps)
		}
		if filepath.Base(ps[0].src) != "my_addon" {
			t.Errorf("src should be the plugin folder, got %s", ps[0].src)
		}
	})

	t.Run("nested sub-addon pruned", func(t *testing.T) {
		root := t.TempDir()
		mkPlugin(t, root, "addons/my_addon")
		mkPlugin(t, root, "addons/my_addon/sub")
		ps := resolveInstall(root, "Whatever", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/my_addon" {
			t.Fatalf("nested sub-addon should be pruned; got %+v", ps)
		}
	})

	t.Run("version.cfg library detected", func(t *testing.T) {
		root := t.TempDir()
		mkCfg(t, root, "addons/my_lib", "version.cfg")
		ps := resolveInstall(root, "Whatever", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/my_lib" {
			t.Fatalf("version.cfg folder not detected; got %+v", ps)
		}
	})

	t.Run("version.cfg nested under plugin.cfg pruned", func(t *testing.T) {
		root := t.TempDir()
		mkPlugin(t, root, "addons/my_addon")
		mkCfg(t, root, "addons/my_addon/lib", "version.cfg")
		ps := resolveInstall(root, "Whatever", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/my_addon" {
			t.Fatalf("nested version.cfg should be pruned; got %+v", ps)
		}
	})

	t.Run("two top-level addons -> dump all", func(t *testing.T) {
		root := t.TempDir()
		mkPlugin(t, root, "addons/a")
		mkPlugin(t, root, "addons/b")
		ps := resolveInstall(root, "Whatever", "", "")
		got := destSet(ps)
		if len(got) != 2 || got[0] != "addons/a" || got[1] != "addons/b" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("plugin.cfg at staging root -> use name", func(t *testing.T) {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "plugin.cfg"), []byte("[plugin]\n"), 0o644)
		ps := resolveInstall(root, "CoolPlugin", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/CoolPlugin" || ps[0].src != root {
			t.Fatalf("got %+v", ps)
		}
	})

	t.Run("no plugin.cfg -> name fallback", func(t *testing.T) {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "README.md"), []byte("hi"), 0o644)
		ps := resolveInstall(root, "Mystery", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/Mystery" || ps[0].src != root {
			t.Fatalf("got %+v", ps)
		}
	})

	t.Run("plugin.cfg at staging root -> use pkgName", func(t *testing.T) {
		// An uploaded asset whose zip is the plugin folder (script_tabs/plugin.cfg):
		// the unwrapped folder name wins over the manifest entry name.
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "plugin.cfg"), []byte("[plugin]\n"), 0o644)
		ps := resolveInstall(root, "EntryName", "", "script_tabs")
		if len(ps) != 1 || ps[0].destRel != "addons/script_tabs" || ps[0].src != root {
			t.Fatalf("got %+v", ps)
		}
	})

	t.Run("nested addon ignores pkgName", func(t *testing.T) {
		// A real addons/<name>/ layout derives from the nested folder, not pkgName.
		root := t.TempDir()
		mkPlugin(t, root, "addons/my_addon")
		ps := resolveInstall(root, "EntryName", "", "ignored_wrapper")
		if len(ps) != 1 || ps[0].destRel != "addons/my_addon" {
			t.Fatalf("got %+v", ps)
		}
	})

	t.Run("empty pkgName at root -> name fallback (suppressed source archive)", func(t *testing.T) {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "plugin.cfg"), []byte("[plugin]\n"), 0o644)
		ps := resolveInstall(root, "CoolPlugin", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/CoolPlugin" {
			t.Fatalf("got %+v", ps)
		}
	})

	t.Run("namespace folder kept when there is no addons/", func(t *testing.T) {
		// addon_lib/my_addon/version.cfg: with no addons/ folder the package root *is*
		// the addons folder, so addon_lib is a namespace level and must survive.
		root := t.TempDir()
		mkCfg(t, root, "addon_lib/my_addon", "version.cfg")
		ps := resolveInstall(root, "Whatever", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/addon_lib/my_addon" {
			t.Fatalf("addon_lib should be kept; got %+v", ps)
		}
		if filepath.Base(ps[0].src) != "my_addon" {
			t.Errorf("src should be the plugin folder, got %s", ps[0].src)
		}
	})

	t.Run("namespace folder kept for a bundle", func(t *testing.T) {
		root := t.TempDir()
		mkPlugin(t, root, "addon_lib/a")
		mkPlugin(t, root, "addon_lib/b")
		ps := resolveInstall(root, "Whatever", "", "")
		got := destSet(ps)
		if len(got) != 2 || got[0] != "addons/addon_lib/a" || got[1] != "addons/addon_lib/b" {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("namespace folder under addons/ anchors on addons/", func(t *testing.T) {
		// The same layout shipped with its own addons/ folder: addons/ is the anchor,
		// so the namespace is not doubled up (addons/addons/addon_lib/…). The child is
		// mirrored whole — my_addon rides along inside src and still lands at
		// addons/addon_lib/my_addon on disk; addon_lib is what gets pinned.
		root := t.TempDir()
		mkCfg(t, root, "addons/addon_lib/my_addon", "version.cfg")
		ps := resolveInstall(root, "Whatever", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/addon_lib" {
			t.Fatalf("got %+v", ps)
		}
		if filepath.Base(ps[0].src) != "addon_lib" {
			t.Errorf("src should be the addons/ child folder, got %s", ps[0].src)
		}
	})

	t.Run("explicit path wins", func(t *testing.T) {
		root := t.TempDir()
		mkPlugin(t, root, "addons/my_addon")
		ps := resolveInstall(root, "Whatever", "addons/addon_lib/my_addon", "")
		if len(ps) != 1 || ps[0].destRel != "addons/addon_lib/my_addon" {
			t.Fatalf("got %+v", ps)
		}
		if filepath.Base(ps[0].src) != "my_addon" {
			t.Errorf("src should be the detected plugin folder, got %s", ps[0].src)
		}
	})

	t.Run("dir= in config overrides derivation", func(t *testing.T) {
		root := t.TempDir()
		mkCfgWith(t, root, "addons/my_addon", "plugin.cfg", `dir="addons/custom/place"`)
		ps := resolveInstall(root, "Whatever", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/custom/place" {
			t.Fatalf("dir= override not honored; got %+v", ps)
		}
		if filepath.Base(ps[0].src) != "my_addon" {
			t.Errorf("src should be the detected plugin folder, got %s", ps[0].src)
		}
	})

	t.Run("dir= in version.cfg overrides derivation", func(t *testing.T) {
		root := t.TempDir()
		mkCfgWith(t, root, "addons/my_lib", "version.cfg", `dir="addons/custom_lib"`)
		ps := resolveInstall(root, "Whatever", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/custom_lib" {
			t.Fatalf("dir= in version.cfg not honored; got %+v", ps)
		}
	})

	t.Run("explicit manifest path still wins over dir=", func(t *testing.T) {
		root := t.TempDir()
		mkCfgWith(t, root, "addons/my_addon", "plugin.cfg", `dir="addons/custom/place"`)
		ps := resolveInstall(root, "Whatever", "addons/manifest_pinned", "")
		if len(ps) != 1 || ps[0].destRel != "addons/manifest_pinned" {
			t.Fatalf("manifest path should win over dir=; got %+v", ps)
		}
	})

	t.Run("addons/ with no plugin.cfg (icon pack)", func(t *testing.T) {
		// at-icons case: a config-less package whose plugin folder lives under addons/
		// alongside repo junk. Derive from addons/, not the whole tree.
		root := t.TempDir()
		os.MkdirAll(filepath.Join(root, "addons/at_icons/node"), 0o755)
		os.WriteFile(filepath.Join(root, "addons/at_icons/LICENSE.txt"), []byte("x"), 0o644)
		os.MkdirAll(filepath.Join(root, "docs"), 0o755)
		os.MkdirAll(filepath.Join(root, ".github"), 0o755)
		os.WriteFile(filepath.Join(root, "README.md"), []byte("hi"), 0o644)
		ps := resolveInstall(root, "at-icons", "", "at-icons-main")
		if len(ps) != 1 || ps[0].destRel != "addons/at_icons" {
			t.Fatalf("got %+v", ps)
		}
		if filepath.Base(ps[0].src) != "at_icons" {
			t.Errorf("src should be the addons/ child folder, got %s", ps[0].src)
		}
	})

	t.Run("addons/ with a loose file beside the plugin folder", func(t *testing.T) {
		root := t.TempDir()
		mkPlugin(t, root, "addons/my_addon")
		os.WriteFile(filepath.Join(root, "addons", "README.md"), []byte("hi"), 0o644)
		ps := resolveInstall(root, "Whatever", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/my_addon" {
			t.Fatalf("loose file should be ignored; got %+v", ps)
		}
	})

	t.Run("addons/ chosen over a stray plugin.cfg elsewhere", func(t *testing.T) {
		root := t.TempDir()
		mkPlugin(t, root, "addons/real")
		mkPlugin(t, root, "tools/stray")
		ps := resolveInstall(root, "Whatever", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/real" {
			t.Fatalf("should resolve from addons/, not the stray; got %+v", ps)
		}
	})

	t.Run("submodule root plugin.cfg wins over a bundled addons/", func(t *testing.T) {
		root := t.TempDir()
		os.WriteFile(filepath.Join(root, "plugin.cfg"), []byte("[plugin]\n"), 0o644)
		mkPlugin(t, root, "addons/bundled")
		ps := resolveInstall(root, "CoolPlugin", "", "")
		if len(ps) != 1 || ps[0].destRel != "addons/CoolPlugin" || ps[0].src != root {
			t.Fatalf("root plugin.cfg should win; got %+v", ps)
		}
	})

	t.Run("multi-folder bundle with pinned path does not collapse", func(t *testing.T) {
		// A bundle (cogito shipping another addon) with the entry's path already
		// pinned must derive each folder, not copy the whole staging tree into the
		// pinned path (the old bug).
		root := t.TempDir()
		mkPlugin(t, root, "addons/cogito")
		mkPlugin(t, root, "addons/quest_system")
		ps := resolveInstall(root, "cogito", "addons/cogito", "")
		if got := destSet(ps); len(got) != 2 || got[0] != "addons/cogito" || got[1] != "addons/quest_system" {
			t.Fatalf("got %v", got)
		}
		for _, p := range ps {
			if p.destRel == "addons/cogito" && filepath.Base(p.src) != "cogito" {
				t.Errorf("cogito src should be the cogito folder, not the staging root: %s", p.src)
			}
		}
	})
}

func TestIsSourceArchiveURL(t *testing.T) {
	cases := []struct {
		url  string
		want bool
	}{
		{"https://github.com/o/r/archive/refs/tags/v1.0.0.zip", true},
		{"https://github.com/o/r/archive/refs/heads/main.zip", true},
		{"https://codeberg.org/o/r/archive/v1.0.0.zip", true},
		{"https://github.com/o/r/releases/download/v1.0.0/script-tabs-1.0.0.zip", false},
		// A local archived copy installs from a file path under .../archive/ — must
		// not be mistaken for a remote source archive.
		{"/home/u/.gdaddon/archive/github.com/o/r/v1.0.0/script-tabs.zip", false},
	}
	for _, c := range cases {
		if got := isSourceArchiveURL(c.url); got != c.want {
			t.Errorf("isSourceArchiveURL(%q) = %v, want %v", c.url, got, c.want)
		}
	}
}

func TestRelocate(t *testing.T) {
	root := t.TempDir()
	mkPlugin(t, root, "addons/my_addon")

	// Moves the dir (creating the new parent) and leaves the old path gone.
	if err := Relocate(root, "addons/my_addon", "addons/lib/renamed"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "addons/lib/renamed/plugin.cfg")); err != nil {
		t.Fatalf("file not moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "addons/my_addon")); !os.IsNotExist(err) {
		t.Errorf("source dir should be gone")
	}

	// Same from/to is a no-op (not an error).
	if err := Relocate(root, "addons/lib/renamed", "addons/lib/renamed"); err != nil {
		t.Errorf("same-path relocate should be a no-op, got %v", err)
	}

	// Refuses to overwrite an existing destination.
	mkPlugin(t, root, "addons/occupied")
	if err := Relocate(root, "addons/lib/renamed", "addons/occupied"); err == nil {
		t.Errorf("expected error relocating onto an existing dir")
	}
}

func TestInspectURLOnlyIsMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addon_manifest.yml")
	os.WriteFile(path, []byte("Libby:\n    url: https://github.com/u/Libby.git\n"), 0o644)

	statuses, err := Inspect(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].State != StateMissing {
		t.Errorf("url-only entry should be StateMissing, got %v", statuses[0].State)
	}
}
