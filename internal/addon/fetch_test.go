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
